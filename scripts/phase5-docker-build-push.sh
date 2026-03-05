#!/bin/bash
# Phase 5: Docker Build and Push Script
# This script builds the demo-job Docker image and pushes it to Artifact Registry

set -e

echo "=== Phase 5: Docker Build & Push ==="

# Configuration
read -p "Enter GCP Project ID: " GCP_PROJECT
read -p "Enter Artifact Registry Region (default: us-central1): " REGISTRY_REGION
REGISTRY_REGION=${REGISTRY_REGION:-us-central1}

# Variables
REGISTRY_HOST="${REGISTRY_REGION}-docker.pkg.dev"
REPOSITORY_NAME="demo-job-repo"
IMAGE_NAME="demo-job"
IMAGE_TAG="latest"
IMAGE_URI="${REGISTRY_HOST}/${GCP_PROJECT}/${REPOSITORY_NAME}/${IMAGE_NAME}:${IMAGE_TAG}"

echo ""
echo "Configuration:"
echo "  Project ID: $GCP_PROJECT"
echo "  Registry: $REGISTRY_HOST"
echo "  Repository: $REPOSITORY_NAME"
echo "  Image URI: $IMAGE_URI"
echo ""

# Step 1: Verify gcloud setup and authenticate via impersonation
echo "Step 1: Verifying gcloud setup..."
gcloud config get-value project > /dev/null 2>&1 || {
    echo "ERROR: gcloud not configured. Run: gcloud auth login"
    exit 1
}
gcloud config set project $GCP_PROJECT

# Authenticate using service account impersonation
echo "Authenticating via service account impersonation..."
gcloud auth application-default login \
  --impersonate-service-account \
  gcp-sa-dev-interns@${GCP_PROJECT}.iam.gserviceaccount.com
echo "✓ gcloud configured for $GCP_PROJECT (impersonated service account)"

# Step 2: Enable required APIs
echo ""
echo "Step 2: Enabling required Google APIs..."
gcloud services enable artifactregistry.googleapis.com --quiet
gcloud services enable batch.googleapis.com --quiet
echo "✓ APIs enabled"

# Step 3: Create Artifact Registry repository if needed
echo ""
echo "Step 3: Setting up Artifact Registry repository..."
REPO_EXISTS=$(gcloud artifacts repositories list \
  --location=$REGISTRY_REGION \
  --filter="name=$REPOSITORY_NAME" \
  --format="value(name)" 2>/dev/null || echo "")

if [ -z "$REPO_EXISTS" ]; then
    echo "Creating repository: $REPOSITORY_NAME"
    gcloud artifacts repositories create $REPOSITORY_NAME \
      --repository-format=docker \
      --location=$REGISTRY_REGION \
      --quiet
    echo "✓ Repository created"
else
    echo "✓ Repository already exists"
fi

# Step 4: Configure Docker authentication
echo ""
echo "Step 4: Configuring Docker authentication..."
gcloud auth configure-docker ${REGISTRY_HOST} --quiet
echo "✓ Docker authentication configured"

# Step 5: Build Docker image
echo ""
echo "Step 5: Building Docker image..."
docker build -t $IMAGE_URI -f cmd/demo-job/Dockerfile . 

if [ $? -eq 0 ]; then
    echo "✓ Docker image built successfully"
else
    echo "ERROR: Docker build failed"
    exit 1
fi

# Step 6: Verify image
echo ""
echo "Step 6: Verifying Docker image..."
docker inspect $IMAGE_URI > /dev/null 2>&1 || {
    echo "ERROR: Docker image verification failed"
    exit 1
}
IMAGE_SIZE=$(docker images --format "table {{.Repository}}\t{{.Size}}" | grep $IMAGE_NAME | awk '{print $NF}')
echo "✓ Image verified (Size: $IMAGE_SIZE)"

# Step 7: Test image locally
echo ""
echo "Step 7: Testing Docker image locally..."
docker run --rm $IMAGE_URI --help > /dev/null 2>&1 || {
    echo "ERROR: Docker image test failed"
    exit 1
}
echo "✓ Image test passed"

# Step 8: Push to Artifact Registry
echo ""
echo "Step 8: Pushing image to Artifact Registry..."
docker push $IMAGE_URI

if [ $? -eq 0 ]; then
    echo "✓ Image pushed successfully"
else
    echo "ERROR: Docker push failed"
    exit 1
fi

# Step 9: Verify push
echo ""
echo "Step 9: Verifying image in Artifact Registry..."
PUSHED_IMAGE=$(gcloud artifacts docker images list \
  ${REGISTRY_HOST}/${GCP_PROJECT}/${REPOSITORY_NAME} \
  --include-tags \
  --format="value(image)" | grep $IMAGE_NAME | head -1)

if [ -z "$PUSHED_IMAGE" ]; then
    echo "ERROR: Image not found in registry"
    exit 1
fi

echo "✓ Image verified in Artifact Registry"
echo ""
echo "=== DEPLOYMENT COMPLETE ==="
echo ""
echo "Docker Image URI:"
echo "  $IMAGE_URI"
echo ""
echo "Next steps:"
echo "  1. Update batch-job-config.json with:"
echo "     \"imageUri\": \"$IMAGE_URI\""
echo "  2. Run: bash scripts/phase5-submit-job.sh"
echo ""
