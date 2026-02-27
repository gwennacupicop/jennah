#!/bin/bash

# Test ListJobs endpoint
# Usage: ./test-list-jobs.sh

WORKER_URL="http://localhost:8081"
TENANT_ID="73edf6d2-b1a8-4f5e-b099-d6a6a8fdd0fa"

echo "Testing ListJobs endpoint..."
echo "================================"

curl -v -X POST "${WORKER_URL}/jennah.v1.DeploymentService/ListJobs" \
  -H "Content-Type: application/json" \
  -H "X-Tenant-Id: ${TENANT_ID}" \
  -d '{}'

echo -e "\n\nDone!"
