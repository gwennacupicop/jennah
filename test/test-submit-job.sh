#!/bin/bash

# Test SubmitJob endpoint
# Usage: ./test-submit-job.sh [resource-profile]
# Examples:
#   ./test-submit-job.sh          # Uses default profile
#   ./test-submit-job.sh small    # Uses small profile
#   ./test-submit-job.sh large    # Uses large profile

WORKER_URL="http://localhost:8081"
TENANT_ID="73edf6d2-b1a8-4f5e-b099-d6a6a8fdd0fa"
RESOURCE_PROFILE="${1:-}"  # Optional: small, medium, large, xlarge

echo "Testing SubmitJob endpoint..."
echo "================================"
echo "Worker URL: ${WORKER_URL}"
echo "Tenant ID: ${TENANT_ID}"
if [ -n "$RESOURCE_PROFILE" ]; then
  echo "Resource Profile: ${RESOURCE_PROFILE}"
else
  echo "Resource Profile: default"
fi
echo ""

# Build JSON payload
JSON_PAYLOAD='{
  "image_uri": "gcr.io/google-samples/hello-app:1.0",
  "env_vars": {
    "TEST_ENV": "production",
    "APP_NAME": "hello-world",
    "TIMESTAMP": "'$(date +%s)'"
  }'

# Add resource_profile if specified
if [ -n "$RESOURCE_PROFILE" ]; then
  JSON_PAYLOAD="${JSON_PAYLOAD}"',
  "resource_profile": "'"${RESOURCE_PROFILE}"'"'
fi

JSON_PAYLOAD="${JSON_PAYLOAD}"'
}'

echo "Request Payload:"
echo "$JSON_PAYLOAD" | jq '.' 2>/dev/null || echo "$JSON_PAYLOAD"
echo ""

# Submit the job
echo "Submitting job..."
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "${WORKER_URL}/jennah.v1.DeploymentService/SubmitJob" \
  -H "Content-Type: application/json" \
  -H "X-Tenant-Id: ${TENANT_ID}" \
  -d "$JSON_PAYLOAD")

# Parse response
HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | sed '$d')

echo "HTTP Status: ${HTTP_CODE}"
echo ""
echo "Response:"
echo "$BODY" | jq '.' 2>/dev/null || echo "$BODY"
echo ""

# Check if successful
if [ "$HTTP_CODE" = "200" ]; then
  echo "✅ Job submitted successfully!"
  JOB_ID=$(echo "$BODY" | jq -r '.jobId' 2>/dev/null)
  if [ -n "$JOB_ID" ] && [ "$JOB_ID" != "null" ]; then
    echo "Job ID: ${JOB_ID}"
  fi
else
  echo "❌ Job submission failed!"
  exit 1
fi

echo ""
echo "Done!"
