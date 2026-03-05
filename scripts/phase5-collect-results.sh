#!/bin/bash
# Phase 5: Results Collection & Analysis Script
# Retrieves metrics from GCP Batch job and runs aggregation analysis

set -e

echo "=== Phase 5: Results Collection & Analysis ==="
echo ""

# Get configuration
read -p "Enter GCP Project ID: " GCP_PROJECT
read -p "Enter Cloud Storage bucket name: " BUCKET_NAME
read -p "Enter Job Name (or leave blank to find latest): " JOB_NAME

# If no job name provided, find latest
if [ -z "$JOB_NAME" ]; then
    JOB_NAME=$(gcloud batch jobs list \
      --location=us-central1 \
      --format="value(name)" \
      --sort-by="~CREATE_TIME" \
      --limit=1)
    echo "Using latest job: $JOB_NAME"
fi

REGION="us-central1"
OUTPUT_DIR="phase5-results-${JOB_NAME}"

echo ""
echo "Configuration:"
echo "  Project: $GCP_PROJECT"
echo "  Job: $JOB_NAME"
echo "  Bucket: gs://${BUCKET_NAME}"
echo "  Output Directory: $OUTPUT_DIR"
echo ""

# Step 1: Verify job exists and check status
echo "Step 1: Checking job status..."
JOB_STATE=$(gcloud batch jobs describe $JOB_NAME \
  --location=$REGION \
  --format="value(status.state)")

echo "  Job state: $JOB_STATE"

if [ "$JOB_STATE" != "SUCCEEDED" ]; then
    echo "WARNING: Job has not completed successfully"
    echo "  Current state: $JOB_STATE"
    read -p "Continue anyway? (y/n): " CONTINUE
    if [ "$CONTINUE" != "y" ]; then
        exit 0
    fi
fi

# Step 2: Create output directory
echo ""
echo "Step 2: Creating output directory..."
mkdir -p $OUTPUT_DIR
echo "✓ Directory created: $OUTPUT_DIR"

# Step 3: Download metrics from GCS
echo ""
echo "Step 3: Downloading metrics from GCS..."

# List available files
echo "  Files in GCS:"
gsutil ls gs://${BUCKET_NAME}/output/ | head -10

# Download metrics
gsutil -m cp gs://${BUCKET_NAME}/output/instance-*.json $OUTPUT_DIR/ 2>/dev/null || {
    echo "ERROR: Failed to download metrics"
    echo "Verify bucket is correct: gs://${BUCKET_NAME}/output/"
    exit 1
}

# Count files
METRIC_FILES=$(ls $OUTPUT_DIR/instance-*.json 2>/dev/null | wc -l)
echo "✓ Downloaded $METRIC_FILES metrics files"

# Step 4: Display individual metrics
echo ""
echo "Step 4: Individual metrics:"
echo ""

for file in $OUTPUT_DIR/instance-*.json; do
    if [ -f "$file" ]; then
        echo "--- $(basename $file) ---"
        py << 'PYTHON_EOF'
import json
import sys
data = json.load(open(sys.argv[1]))
print(f"  Instance ID: {data['instance_id']}")
print(f"  Lines: {data['lines_count']:,}")
print(f"  Bytes: {data['bytes_processed']:,}")
print(f"  Time: {data['processing_time_seconds']:.2f}s")
print(f"  Throughput: {data['throughput_mb_per_second']:.2f} MB/s")
PYTHON_EOF
        "$file"
        echo ""
    fi
done

# Step 5: Run aggregator
echo ""
echo "Step 5: Running aggregator on metrics..."
echo ""

# Check if aggregator exists
if [ ! -f "cmd/aggregator/aggregator" ]; then
    echo "ERROR: Aggregator not found. Build it first:"
    echo "  go build -o cmd/aggregator/aggregator ./cmd/aggregator"
    exit 1
fi

# Run with local path
./cmd/aggregator/aggregator \
  --metrics-path $OUTPUT_DIR \
  --baseline-seconds 52.0 \
  --format detailed

# Step 6: Generate summary report
echo ""
echo "Step 6: Generating summary report..."

REPORT_FILE="${OUTPUT_DIR}/RESULTS_SUMMARY.txt"

{
    echo "=========================================="
    echo "PHASE 5 DISTRIBUTED BATCH PROCESSING RESULTS"
    echo "=========================================="
    echo ""
    echo "Job Details:"
    echo "  Job Name: $JOB_NAME"
    echo "  Job State: $JOB_STATE"
    echo "  Project: $GCP_PROJECT"
    echo "  Bucket: gs://${BUCKET_NAME}"
    echo ""
    
    echo "Metrics Files:"
    ls -lh $OUTPUT_DIR/instance-*.json | awk '{print "  " $9 " (" $5 ")"}'
    echo ""
    
    echo "Quick Stats:"
    py << 'PYTHON_EOF'
import json
import glob
import os

metrics = []
for f in sorted(glob.glob(os.getcwd() + '/*/instance-*.json')):
    try:
        with open(f) as fp:
            metrics.append(json.load(fp))
    except:
        pass

if metrics:
    metrics.sort(key=lambda m: m['instance_id'])
    
    total_lines = sum(m['lines_count'] for m in metrics)
    total_bytes = sum(m['bytes_processed'] for m in metrics)
    total_time = sum(m['processing_time_seconds'] for m in metrics)
    max_time = max(m['processing_time_seconds'] for m in metrics)
    min_time = min(m['processing_time_seconds'] for m in metrics)
    avg_time = total_time / len(metrics)
    
    baseline = 52.0
    speedup = baseline / max_time
    efficiency = speedup / len(metrics)
    
    print(f"  Instances: {len(metrics)}")
    print(f"  Total Lines: {total_lines:,}")
    print(f"  Total Bytes: {total_bytes:,}")
    print(f"  Processing Time (min/avg/max): {min_time:.2f}s / {avg_time:.2f}s / {max_time:.2f}s")
    print(f"  Speedup: {speedup:.2f}x")
    print(f"  Efficiency: {efficiency:.1%}")
PYTHON_EOF
    
    echo ""
    echo "Analysis:"
    echo "  Expected speedup for 4 instances: 3.5-3.8x"
    echo "  Expected efficiency: 87-95%"
    echo "  GCS overhead typically adds 5-10% latency"
    
} | tee $REPORT_FILE

# Step 7: Additional analysis
echo ""
echo "Step 7: Additional analysis files..."

# Create detailed CSV
CSV_FILE="${OUTPUT_DIR}/metrics-detailed.csv"
{
    echo "instance_id,lines_count,bytes_processed,processing_time_seconds,throughput_mb_per_second,start_byte,end_byte"
    
    for file in $OUTPUT_DIR/instance-*.json; do
        py << 'PYTHON_END'
import json, sys
d = json.load(open(sys.argv[1]))
print(f"{d['instance_id']},{d['lines_count']},{d['bytes_processed']},{d['processing_time_seconds']:.2f},{d['throughput_mb_per_second']:.2f},{d['start_byte']},{d['end_byte']}")
PYTHON_END
        "$file"
    done
} > $CSV_FILE

echo "✓ Created: $CSV_FILE"

# Summary
echo ""
echo "=== COLLECTION COMPLETE ==="
echo ""
echo "Results saved to: $OUTPUT_DIR"
echo "Files:"
echo "  - instance-*.json (raw metrics)"
echo "  - RESULTS_SUMMARY.txt (report)"
echo "  - metrics-detailed.csv (analysis)"
echo ""
echo "Next steps:"
echo "  1. Review results: cat $REPORT_FILE"
echo "  2. Compare with local test (Phase 4)"
echo "  3. Document findings"
echo "  4. Create final report: docs/r9ight/PHASE_5_RESULTS.md"
echo ""
