# Integration Tests

This directory contains integration tests for Jennah services.

## Prerequisites

1. **Worker must be running:**
   ```bash
   # From project root
   BATCH_PROVIDER=gcp \
   BATCH_PROJECT_ID=labs-169405 \
   BATCH_REGION=asia-northeast1 \
   DB_PROVIDER=spanner \
   DB_PROJECT_ID=labs-169405 \
   DB_INSTANCE=alphaus-dev \
   DB_DATABASE=main \
   go run ./cmd/worker/
   ```

2. **Database must be accessible** (Cloud Spanner)

3. **GCP authentication:**
   ```bash
   gcloud auth application-default login
   ```

## Running Tests

### Test SubmitJob Endpoint

Submits a test job to the worker:

```bash
./test/test-submit-job.sh
```

Expected output:
- HTTP 200 OK
- Response with `job_id` and `status: "PENDING"` or `status: "RUNNING"`

### Test ListJobs Endpoint

Lists all jobs for a tenant:

```bash
./test/test-list-jobs.sh
```

Expected output:
- HTTP 200 OK
- Response with array of jobs

## Customization

Edit the scripts to change:
- `WORKER_URL` - Worker endpoint (default: `http://localhost:8081`)
- `TENANT_ID` - Test tenant ID (default: `test-tenant-123`)
- `image_uri` - Container image to run
- `env_vars` - Environment variables for the job

## What Gets Committed to Git?

✅ **YES - Commit these:**
- Test scripts (`*.sh`)
- Test documentation (`README.md`)
- Example request payloads

❌ **NO - Don't commit (add to .gitignore):**
- Test outputs/logs
- Temporary test data
- `.env` files with real credentials

## Notes

- These tests use **curl** to call ConnectRPC endpoints
- Tests require a real database connection (not mocked)
- Jobs submitted will actually be created in GCP Batch
- Use `test-tenant-123` to avoid polluting production data
