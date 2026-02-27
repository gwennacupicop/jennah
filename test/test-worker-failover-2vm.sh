#!/usr/bin/env bash
set -euo pipefail

GW_URL="${GW_URL:-}"
OAUTH_EMAIL="${OAUTH_EMAIL:-failover-test@alphauslabs.com}"
OAUTH_USER_ID="${OAUTH_USER_ID:-failover-test-user}"
OAUTH_PROVIDER="${OAUTH_PROVIDER:-google}"

VM_PRIMARY="${VM_PRIMARY:-}"
VM_SECONDARY="${VM_SECONDARY:-}"
VM_ZONE="${VM_ZONE:-asia-northeast1-a}"
PROJECT_ID="${PROJECT_ID:-labs-169405}"

if [[ -z "${GW_URL}" ]]; then
	echo "GW_URL is required"
	exit 1
fi

echo "Submitting long-running job through gateway: ${GW_URL}"
RESP="$(curl -sS -X POST "${GW_URL}/jennah.v1.DeploymentService/SubmitJob" \
	-H "Content-Type: application/json" \
	-H "Connect-Protocol-Version: 1" \
	-H "X-OAuth-Email: ${OAUTH_EMAIL}" \
	-H "X-OAuth-UserId: ${OAUTH_USER_ID}" \
	-H "X-OAuth-Provider: ${OAUTH_PROVIDER}" \
	-d '{"image_uri":"us-docker.pkg.dev/google-containers/busybox","commands":["sh","-c","sleep 900"]}')"

echo "Submit response: ${RESP}"

JOB_ID="$(echo "${RESP}" | sed -n 's/.*"jobId":"\([^"]*\)".*/\1/p')"
if [[ -z "${JOB_ID}" ]]; then
	JOB_ID="$(echo "${RESP}" | sed -n 's/.*"job_id":"\([^"]*\)".*/\1/p')"
fi

if [[ -z "${JOB_ID}" ]]; then
	echo "Failed to parse job ID from response"
	exit 1
fi

echo "Job ID: ${JOB_ID}"
echo "Initial routing worker: $(echo "${RESP}" | sed -n 's/.*"workerAssigned":"\([^"]*\)".*/\1/p')"

if [[ -n "${VM_PRIMARY}" ]]; then
	echo "Stopping primary VM ${VM_PRIMARY} to trigger failover..."
	gcloud compute instances stop "${VM_PRIMARY}" --zone "${VM_ZONE}" --project "${PROJECT_ID}" --quiet
fi

echo "Waiting for ownership failover (lease expiry + claim interval)..."
sleep 45

if [[ -n "${VM_PRIMARY}" ]]; then
	echo "Starting primary VM ${VM_PRIMARY} to observe handback..."
	gcloud compute instances start "${VM_PRIMARY}" --zone "${VM_ZONE}" --project "${PROJECT_ID}" --quiet
	echo "Allowing time for worker restart + reconcile..."
	sleep 45
fi

echo "Checking job status via gateway list..."
curl -sS -X POST "${GW_URL}/jennah.v1.DeploymentService/ListJobs" \
	-H "Content-Type: application/json" \
	-H "Connect-Protocol-Version: 1" \
	-H "X-OAuth-Email: ${OAUTH_EMAIL}" \
	-H "X-OAuth-UserId: ${OAUTH_USER_ID}" \
	-H "X-OAuth-Provider: ${OAUTH_PROVIDER}" \
	-d '{}' | grep -E "${JOB_ID}|status|workerAssigned" || true

echo "Failover test flow complete for job ${JOB_ID}"

