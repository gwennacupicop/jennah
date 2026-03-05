#!/bin/bash
# Phase 5: GCP Batch Job Submission Script
# This script submits a distributed job to Google Batch

set -e

echo "=== Phase 5: Submit Distributed Job to GCP Batch ==="
echo ""

# Get configuration
read -p "Enter GCP Project ID: " GCP_PROJECT
read -p "Enter Artifact Registry Region (default: us-central1): " REGISTRY_REGION
REGISTRY_REGION=${REGISTRY_REGION:-us-central1}

read -p "Enter number of instances (default: 4): " TASK_COUNT
TASK_COUNT=${TASK_COUNT:-4}

read -p "Enter Cloud Storage bucket name (without gs://): " BUCKET_NAME

# Variables
REGION="us-central1"
JOB_NAME="demo-job-$(date +%Y%m%d-%H%M%S)"
CONFIG_FILE="batch-job-config-${JOB_NAME}.json"
INPUT_FILE_SIZE="524288000"

echo ""
echo "Configuration:"
echo "  Project: $GCP_PROJECT"
echo "  Job Name: $JOB_NAME"
echo "  Region: $REGION"
echo "  Instances: $TASK_COUNT"
echo "  Bucket: gs://${BUCKET_NAME}"
echo ""

# Step 1: Verify GCS bucket exists
echo "Step 1: Verifying Cloud Storage bucket..."
if ! gsutil ls gs://${BUCKET_NAME} > /dev/null 2>&1; then
    echo "ERROR: Bucket gs://${BUCKET_NAME} not found"
    echo "Create it with: gsutil mb gs://${BUCKET_NAME}"
    exit 1
fi
echo "✓ Bucket verified"

# Step 2: Verify input data exists
echo ""
echo "Step 2: Verifying input data..."
if ! gsutil ls gs://${BUCKET_NAME}/input/test-data.txt > /dev/null 2>&1; then
    echo "ERROR: Input file not found: gs://${BUCKET_NAME}/input/test-data.txt"
    echo ""
    echo "Upload test data:"
    echo "  py scripts/phase5-generate-test-data.py"
    echo "  gsutil cp test-data.txt gs://${BUCKET_NAME}/input/"
    exit 1
fi

# Get actual file size
ACTUAL_SIZE=$(gsutil stat gs://${BUCKET_NAME}/input/test-data.txt | grep "Content-Length" | awk '{print $NF}')
INPUT_FILE_SIZE=$ACTUAL_SIZE
echo "✓ Input file found (${ACTUAL_SIZE} bytes)"

# Step 3: Create job config from template
echo ""
echo "Step 3: Creating job configuration..."

# Get image URI
IMAGE_URI="${REGISTRY_REGION}-docker.pkg.dev/${GCP_PROJECT}/demo-job-repo/demo-job:latest"

# Create config file
cat scripts/batch-job-config-template.json | \
  sed "s|REPLACE_WITH_YOUR_IMAGE_URI|${IMAGE_URI}|g" | \
  sed "s|REPLACE_PROJECT_BUCKET|${BUCKET_NAME}|g" | \
  sed "s|REPLACE_PROJECT_ID|${GCP_PROJECT}|g" | \
  sed "s|batch-job-runner|gcp-sa-dev-interns|g" | \
  sed "s|\"taskCount\": 4|\"taskCount\": ${TASK_COUNT}|g" | \
  sed "s|\"parallelism\": 4|\"parallelism\": ${TASK_COUNT}|g" | \
  sed "s|\"INPUT_DATA_SIZE\": \"524288000\"|\"INPUT_DATA_SIZE\": \"${INPUT_FILE_SIZE}\"|g" \
  > $CONFIG_FILE

echo "✓ Job configuration created: $CONFIG_FILE"

# Step 4: Submit job to Batch
echo ""
echo "Step 4: Submitting job to Google Batch..."
echo "  Command: gcloud batch jobs create --config=$CONFIG_FILE"

gcloud batch jobs create $JOB_NAME \
  --config=$CONFIG_FILE \
  --location=$REGION

JOB_ID=$(gcloud batch jobs list \
  --location=$REGION \
  --filter="name:*${JOB_NAME}*" \
  --format="value(name)" | head -1)

echo "✓ Job submitted successfully"
echo ""
echo "Job Details:"
echo "  Job Name: $JOB_NAME"
echo "  Job UID: $JOB_ID"
echo "  Region: $REGION"
echo "  Instances: $TASK_COUNT"
echo ""

# Step 5: Monitor job
echo "Step 5: Monitoring job execution..."
echo ""
echo "Commands to monitor:"
echo "  # Check job status"
echo "  gcloud batch jobs describe $JOB_NAME --location=$REGION"
echo ""
echo "  # Watch status (every 10 seconds)"
echo "  watch -n 10 'gcloud batch jobs describe $JOB_NAME --location=$REGION --format=value(status.state)'"
echo ""
echo "  # View task details"
echo "  gcloud batch tasks list --job=$JOB_NAME --location=$REGION"
echo ""

# Step 6: Wait for job or ask user
read -p "Do you want to wait for job completion? (y/n, default: n): " WAIT_FOR_COMPLETE
WAIT_FOR_COMPLETE=${WAIT_FOR_COMPLETE:-n}

if [ "$WAIT_FOR_COMPLETE" = "y" ]; then
    echo ""
    echo "Waiting for job to complete..."
    
    TIMEOUT=1800  # 30 minutes
    ELAPSED=0
    POLL_INTERVAL=30
    
    while [ $ELAPSED -lt $TIMEOUT ]; do
        STATE=$(gcloud batch jobs describe $JOB_NAME \
          --location=$REGION \
          --format="value(status.state)")
        
        echo "[$(date '+%H:%M:%S')] Job state: $STATE"
        
        if [ "$STATE" = "SUCCEEDED" ]; then
            echo "✓ Job completed successfully"
            break
        elif [ "$STATE" = "FAILED" ]; then
            echo "✗ Job failed"
            echo ""
            echo "View logs with:"
            echo "  gcloud logging read 'resource.type=gce_instance AND labels.job_id=$JOB_NAME' --limit=50"
            exit 1
        fi
        
        sleep $POLL_INTERVAL
        ELAPSED=$((ELAPSED + POLL_INTERVAL))
    done
    
    if [ $ELAPSED -ge $TIMEOUT ]; then
        echo "⏱ Timeout waiting for job (${TIMEOUT}s). Check status manually."
    fi
fi

echo ""
echo "=== SUBMISSION COMPLETE ==="
echo ""
echo "Next steps:"
echo "  1. Monitor job: gcloud batch jobs describe $JOB_NAME --location=$REGION"
echo "  2. Once complete, retrieve results:"
echo "     mkdir -p output-${JOB_NAME}"
echo "     gsutil -m cp gs://${BUCKET_NAME}/output/instance-*.json output-${JOB_NAME}/"
echo "  3. Run aggregator:"
echo "     ./cmd/aggregator/aggregator --metrics-path gs://${BUCKET_NAME}/output --baseline-seconds 52.0"
echo ""
