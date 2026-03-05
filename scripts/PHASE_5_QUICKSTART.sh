#!/bin/bash
# Phase 5 Quick Start Guide - All Commands in Sequence

# ============================================
# PHASE 5: GCP BATCH DEPLOYMENT - QUICK START
# ============================================
#
# This guide runs through all Phase 5 steps.
# Run commands one section at a time.

echo "=== PHASE 5: GCP BATCH DEPLOYMENT - QUICK START ==="
echo ""
echo "This guide has 7 sections. Run each section sequentially."
echo ""

# ============================================
# SECTION 1: GCP SETUP
# ============================================
echo ""
echo "SECTION 1: GCP ENVIRONMENT SETUP"
echo "=================================="
echo ""
echo "Run these commands to set up your GCP environment:"
echo ""

cat << 'EOF'
# Set your project ID
export GCP_PROJECT="your-project-id"
export REGION="us-central1"
export REGISTRY_REGION="us-central1"

# Authenticate with GCP via service account impersonation
gcloud auth login
gcloud config set project $GCP_PROJECT

# Impersonate the dev-interns service account
gcloud auth application-default login \
  --impersonate-service-account \
  gcp-sa-dev-interns@${GCP_PROJECT}.iam.gserviceaccount.com

# Enable required APIs
gcloud services enable batch.googleapis.com
gcloud services enable artifactregistry.googleapis.com
gcloud services enable storage.googleapis.com

# Verify APIs are enabled
gcloud services list --enabled | grep -E "(Batch|Artifact|Storage)"

echo "✓ GCP setup complete (using impersonated service account)"
EOF

echo ""
read -p "Press Enter when GCP setup is complete..."
echo ""

# ============================================
# SECTION 2: CREATE STORAGE BUCKETS
# ============================================
echo "SECTION 2: CREATE CLOUD STORAGE BUCKETS"
echo "======================================="
echo ""
echo "Run these commands to create GCS buckets:"
echo ""

cat << 'EOF'
# Set bucket name
export BUCKET_NAME="${GCP_PROJECT}-distributed-demo"

# Create bucket
gsutil mb -p $GCP_PROJECT gs://${BUCKET_NAME}/

# Create folder structure
gsutil -m mkdir \
  gs://${BUCKET_NAME}/input/ \
  gs://${BUCKET_NAME}/output/ \
  gs://${BUCKET_NAME}/logs/

# Verify structure
gsutil ls -R gs://${BUCKET_NAME}/

echo "✓ Buckets created"
EOF

echo ""
read -p "Press Enter after creating buckets..."
echo ""

# ============================================
# SECTION 3: GENERATE TEST DATA
# ============================================
echo "SECTION 3: GENERATE TEST DATA"
echo "=============================="
echo ""
echo "Run these commands to generate and upload test data:"
echo ""

cat << 'EOF'
# Generate 100MB test file (5 million lines)
# NOTE: On Windows use 'py' instead of 'python3'
py scripts/phase5-generate-test-data.py -o test-data.txt -l 5000000

# Or use preset sizes:
# py scripts/phase5-generate-test-data.py --size small   (1M lines)
# py scripts/phase5-generate-test-data.py --size medium  (5M lines)
# py scripts/phase5-generate-test-data.py --size large   (10M lines)

# Upload to GCS
gsutil cp test-data.txt gs://${BUCKET_NAME}/input/

# Verify upload
gsutil ls -lh gs://${BUCKET_NAME}/input/

echo "✓ Test data uploaded"
EOF

echo ""
read -p "Press Enter after uploading test data..."
echo ""

# ============================================
# SECTION 4: BUILD & PUSH DOCKER IMAGE
# ============================================
echo "SECTION 4: BUILD & PUSH DOCKER IMAGE"
echo "====================================="
echo ""
echo "Run this script to build and push the Docker image:"
echo ""

cat << 'EOF'
bash scripts/phase5-docker-build-push.sh

# This script will:
# 1. Ask for your GCP Project ID
# 2. Enable APIs
# 3. Create Artifact Registry repository
# 4. Build Docker image locally
# 5. Test image
# 6. Push to Artifact Registry
# 7. Verify push

# Save the Image URI from the output - you'll need it next!

echo "✓ Docker image pushed"
EOF

echo ""
read -p "Press Enter after Docker build & push..."
echo ""

# ============================================
# SECTION 5: SUBMIT BATCH JOB
# ============================================
echo "SECTION 5: SUBMIT BATCH JOB"
echo "==========================="
echo ""
echo "Run this script to submit the distributed job:"
echo ""

cat << 'EOF'
bash scripts/phase5-submit-job.sh

# This script will:
# 1. Ask for configuration (project, bucket, instance count)
# 2. Verify GCS bucket and input data
# 3. Create job configuration
# 4. Submit job to Batch
# 5. Show how to monitor

# NOTE: Set TASK_COUNT=4 when asked
#       Use gs://PROJECT-bucket/output for metrics

echo "✓ Job submitted to GCP Batch"
EOF

echo ""
read -p "Press Enter after job submission..."
echo ""

# ============================================
# SECTION 6: MONITOR JOB EXECUTION
# ============================================
echo "SECTION 6: MONITOR JOB EXECUTION"
echo "================================="
echo ""
echo "Watch your job run with these commands:"
echo ""

cat << 'EOF'
# Set job name from submission output
export JOB_NAME="demo-job-XXXXXXXX-XXXXXX"

# Check job status once
gcloud batch jobs describe $JOB_NAME --location=us-central1

# Watch status continuously (updates every 10s)
watch -n 10 "gcloud batch jobs describe $JOB_NAME --location=us-central1 --format=value(status.state)"

# View task details
gcloud batch tasks list --job=$JOB_NAME --location=us-central1

# Expected states: QUEUED → RUNNING → SUCCEEDED
# Job typically takes 15-30 minutes to complete

echo "✓ Monitor job until it shows SUCCEEDED"
EOF

echo ""
read -p "Press Enter when job completes (state = SUCCEEDED)..."
echo ""

# ============================================
# SECTION 7: COLLECT & ANALYZE RESULTS
# ============================================
echo "SECTION 7: COLLECT & ANALYZE RESULTS"
echo "===================================="
echo ""
echo "Retrieve and analyze results with this script:"
echo ""

cat << 'EOF'
bash scripts/phase5-collect-results.sh

# This script will:
# 1. Ask for project and bucket
# 2. Download metrics files from GCS
# 3. Show per-instance details
# 4. Run aggregator analysis
# 5. Generate summary report
# 6. Create CSV for analysis

# Results saved to: phase5-results-*/ directory

# Final Statistics:
# - Speedup should be 3.5-3.8x (4 instances)
# - Efficiency should be 87-95%
# - Variance from local test (~5%): normal due to cloud overhead

echo "✓ Phase 5 complete!"
EOF

echo ""
echo "=== PHASE 5 QUICK START COMPLETE ==="
echo ""
echo "Summary:"
echo "  ✓ GCP environment configured"
echo "  ✓ Cloud Storage buckets created"
echo "  ✓ Test data generated and uploaded"
echo "  ✓ Docker image built and pushed"
echo "  ✓ Distributed job submitted to Batch"
echo "  ✓ Job executed and metrics collected"
echo "  ✓ Results analyzed and reported"
echo ""
echo "Next: Create phase5-results document with findings"
echo ""
