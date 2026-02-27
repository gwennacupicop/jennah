#!/bin/bash

# Test worker health endpoint
# Usage: ./test-health.sh

WORKER_URL="http://localhost:8081"

echo "Testing Health endpoint..."
echo "================================"

curl -v "${WORKER_URL}/health"

echo -e "\n\nExpected: HTTP 200 OK"
