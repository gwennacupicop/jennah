# Worker Service

The Worker service orchestrates cloud batch jobs and manages job lifecycle in the database. It serves as the execution layer between the Gateway and cloud batch APIs (GCP Batch, AWS Batch, Azure Batch).

## Overview

The Worker receives job submission requests from the Gateway via ConnectRPC, creates corresponding batch jobs on the configured cloud provider, and persists job metadata to the database. Workers listen on port 8081 (configurable) and handle tenant-specific workloads based on consistent hashing routing from the Gateway.

## Configuration

The Worker is now provider-agnostic and configured entirely via environment variables.

### Required Environment Variables

#### Batch Provider Configuration

| Variable         | Description         | Example                                    |
| ---------------- | ------------------- | ------------------------------------------ |
| `BATCH_PROVIDER` | Cloud provider name | `gcp`, `aws`, `azure`                      |
| `BATCH_REGION`   | Cloud region        | `asia-northeast1` (GCP), `us-east-1` (AWS) |

#### Provider-Specific Variables

**GCP:**

- `BATCH_PROJECT_ID`: GCP project ID (e.g., `labs-169405`)

**AWS:**

- `AWS_ACCOUNT_ID`: AWS account ID
- `AWS_JOB_QUEUE`: AWS Batch job queue name

**Azure:**

- `AZURE_SUBSCRIPTION_ID`: Azure subscription ID
- `AZURE_RESOURCE_GROUP`: Azure resource group name

#### Database Configuration

| Variable        | Description                      | Example                           |
| --------------- | -------------------------------- | --------------------------------- |
| `DB_PROVIDER`   | Database provider                | `spanner`, `dynamodb`, `postgres` |
| `DB_PROJECT_ID` | Database project ID (Spanner)    | `labs-169405`                     |
| `DB_INSTANCE`   | Database instance name (Spanner) | `alphaus-dev`                     |
| `DB_DATABASE`   | Database name                    | `main`                            |

#### Server Configuration

| Variable      | Description      | Default |
| ------------- | ---------------- | ------- |
| `WORKER_PORT` | HTTP server port | `8081`  |

### Optional Failover Configuration (PoC)

| Variable                        | Description                                              | Default |
| ------------------------------- | -------------------------------------------------------- | ------- |
| `WORKER_ID`                     | Stable worker identity (set unique value per VM)         | Hostname |
| `WORKER_LEASE_TTL_SECONDS`      | Lease expiration for active job ownership                | `30`    |
| `WORKER_CLAIM_INTERVAL_SECONDS` | Interval for scanning/claiming orphaned active jobs      | `5`     |

For multi-VM failover, set a unique `WORKER_ID` on each VM.

## Running the Worker

### Option 1: Direct Execution (Development)

1. **Set environment variables:**

   ```bash
   export BATCH_PROVIDER=gcp
   export BATCH_PROJECT_ID=labs-169405
   export BATCH_REGION=asia-northeast1
   export DB_PROVIDER=spanner
   export DB_PROJECT_ID=labs-169405
   export DB_INSTANCE=alphaus-dev
   export DB_DATABASE=main
   ```

2. **Run the worker:**
   ```bash
   go run ./cmd/worker/
   ```

### Option 2: Inline Environment Variables

```bash
BATCH_PROVIDER=gcp \
BATCH_PROJECT_ID=labs-169405 \
BATCH_REGION=asia-northeast1 \
DB_PROVIDER=spanner \
DB_PROJECT_ID=labs-169405 \
DB_INSTANCE=alphaus-dev \
DB_DATABASE=main \
go run ./cmd/worker/
```

### Option 3: Docker (Production)

1. **Build the Docker image:**

   ```bash
   docker build -f Dockerfile.worker -t jennah-worker:latest .
   ```

2. **Run with environment variables:**

   ```bash
   docker run -p 8081:8081 \
     -e BATCH_PROVIDER=gcp \
     -e BATCH_PROJECT_ID=labs-169405 \
     -e BATCH_REGION=asia-northeast1 \
     -e DB_PROVIDER=spanner \
     -e DB_PROJECT_ID=labs-169405 \
     -e DB_INSTANCE=alphaus-dev \
     -e DB_DATABASE=main \
     jennah-worker:latest
   ```

3. **Or use env-file:**
   ```bash
   docker run -p 8081:8081 --env-file .env jennah-worker:latest
   ```

### Option 4: Cloud Run Deployment

```bash
# Build and push to Artifact Registry
docker build -f Dockerfile.worker -t asia-docker.pkg.dev/labs-169405/jennah/worker:latest .
docker push asia-docker.pkg.dev/labs-169405/jennah/worker:latest

# Deploy to Cloud Run
gcloud run deploy jennah-worker \
  --image=asia-docker.pkg.dev/labs-169405/jennah/worker:latest \
  --region=asia-northeast1 \
  --set-env-vars="BATCH_PROVIDER=gcp,BATCH_PROJECT_ID=labs-169405,BATCH_REGION=asia-northeast1,DB_PROVIDER=spanner,DB_PROJECT_ID=labs-169405,DB_INSTANCE=alphaus-dev,DB_DATABASE=main"
```

## Prerequisites

1. **Cloud Authentication**

   **GCP:**

   ```bash
   gcloud auth application-default login
   ```

   **AWS:**

   ```bash
   aws configure
   ```

   **Azure:**

   ```bash
   az login
   ```

2. **Required Cloud APIs Enabled**
   - **GCP**: Cloud Spanner API, Batch API
   - **AWS**: AWS Batch, DynamoDB (if using)
   - **Azure**: Azure Batch, Cosmos DB (if using)

3. **IAM Permissions**

   **GCP:**
   - `spanner.databaseUser` on the Spanner database
   - `batch.jobs.create` on the project
   - `batch.jobs.get` on the project

   **AWS:**
   - `batch:SubmitJob`, `batch:DescribeJobs`, etc.
   - DynamoDB table access

4. **Database**
   - Database schema must be deployed (see [/database/schema.sql](/database/schema.sql))
   - Run migration: [/database/migrate-cloud-resource-path.sql](/database/migrate-cloud-resource-path.sql)
  - Run migration: [/database/migrate-worker-lease-columns.sql](/database/migrate-worker-lease-columns.sql)
   - Tenants are automatically created on first job submission if they don't exist

## Building

```bash
# From project root
go build -o worker ./cmd/worker

# Or use go run for development
go run ./cmd/worker/main.go
```

## Running

### Local Development

```bash
# From project root
./worker

# Or using go run
go run ./cmd/worker/main.go
```

### Expected Output

```
Starting worker...
Connected to Spanner: labs-169405/alphaus-dev/main
Connected to GCP Batch API in region: asia-northeast1
ConnectRPC handler registered at path: /jennah.v1.DeploymentService/
Health check endpoint: /health
Worker listening on 0.0.0.0:8081
Available endpoints:
  • POST /jennah.v1.DeploymentService/SubmitJob
  • POST /jennah.v1.DeploymentService/ListJobs
  • GET  /health
Worker configured for project: labs-169405, region: asia-northeast1
```

## API Endpoints

### Health Check

```bash
curl http://localhost:8081/health
# Response: OK (200)
```

### Submit Job (Direct - for testing)

```bash
curl -X POST http://localhost:8081/jennah.v1.DeploymentService/SubmitJob \
  -H "Content-Type: application/json" \
  -H "X-Tenant-Id: test-tenant" \
  -d '{
    "image_uri": "gcr.io/labs-169405/my-app:latest",
    "env_vars": {
      "DATABASE_URL": "postgres://...",
      "API_KEY": "secret123"
    }
  }'
```

**Response:**

```json
{
  "job_id": "f05e8617-e8a9-4c8a-bcbb-dd00a8333c04",
  "status": "RUNNING"
}
```

### List Jobs (Direct - for testing)

```bash
curl -X POST http://localhost:8081/jennah.v1.DeploymentService/ListJobs \
  -H "Content-Type: application/json" \
  -H "X-Tenant-Id: test-tenant" \
  -d '{}'
```

**Response:**

```json
{
  "jobs": [
    {
      "job_id": "f05e8617-e8a9-4c8a-bcbb-dd00a8333c04",
      "tenant_id": "test-tenant",
      "image_uri": "gcr.io/labs-169405/my-app:latest",
      "status": "RUNNING",
      "created_at": "2026-02-11T10:30:00Z"
    }
  ]
}
```

## Job Lifecycle

1. **PENDING**: Job record created in Spanner
2. **RUNNING**: GCP Batch job successfully created
3. **COMPLETED**: Job finished successfully (future: status polling)
4. **FAILED**: Job creation or execution failed

## Architecture

### Request Flow

```
Gateway (8080) → Worker (8081) → GCP Batch API → Compute Engine
                      ↓
                  Cloud Spanner
```

### SubmitJob Handler Flow

1. Validate `tenant_id` and `image_uri`
2. Ensure tenant exists (auto-create if missing due to INTERLEAVE IN PARENT constraint)
3. Generate UUID for job ID
4. Insert job record in Spanner with `PENDING` status
5. Create GCP Batch job with container image and environment variables
6. Update job status to `RUNNING` on success
7. Return job ID and status to Gateway

### ListJobs Handler Flow

1. Validate `tenant_id`
2. Query all jobs for tenant from Spanner
3. Transform database records to proto format
4. Convert timestamps to ISO8601 strings
5. Return job list

## Integration with Gateway

Workers are discovered by the Gateway through hardcoded IP addresses (see [/cmd/gateway/main.go](/cmd/gateway/main.go)). The Gateway uses consistent hashing to route tenant requests to specific workers.

**Gateway Worker Configuration (example):**

```go
workerIPs := []string{
    "10.128.0.1",
    "10.128.0.2",
    "10.128.0.3",
}
```

For local testing with Gateway+Worker, update Gateway's worker IPs to include `localhost` or your local IP:

```go
workerIPs := []string{
    "127.0.0.1",  // Local worker
}
```

## GCP Batch Job Structure

Workers create GCP Batch jobs with the following structure:

```json
{
  "taskGroups": [
    {
      "taskSpec": {
        "runnables": [
          {
            "container": {
              "imageUri": "gcr.io/project/image:tag"
            },
            "environment": {
              "variables": {
                "KEY": "value"
              }
            }
          }
        ]
      },
      "taskCount": 1
    }
  ]
}
```

Jobs are created with:

- **Parent**: `projects/labs-169405/locations/asia-northeast1`
- **Job ID**: UUID from job record
- **Container**: User-specified image URI
- **Environment**: User-specified environment variables

## Troubleshooting

### Worker Won't Start

**Error:** `Failed to create database client`

- Ensure `gcloud auth application-default login` is completed
- Verify Spanner instance and database exist
- Check IAM permissions

**Error:** `Failed to create GCP Batch client`

- Ensure Batch API is enabled: `gcloud services enable batch.googleapis.com`
- Verify authentication credentials have batch API access

### Job Creation Fails

**Check Spanner:**

```bash
# Verify job was created with PENDING status
gcloud spanner databases execute-sql main \
  --instance=alphaus-dev \
  --sql="SELECT * FROM Jobs WHERE JobId='<job-id>'"
```

**Check GCP Batch Console:**

- Navigate to: https://console.cloud.google.com/batch/jobs?project=labs-169405
- Filter by region: asia-northeast1
- Look for job by UUID

**Common Issues:**

- Parent row missing error: Tenant is auto-created on first job submission (fixed by service)
- Image URI not accessible (check Container Registry permissions)
- Region quota exceeded (check asia-northeast1 quota)
- Invalid environment variable format

### Gateway Can't Reach Worker

**Error:** Gateway logs show "worker failed to process job"

- Verify worker is listening on port 8081: `netstat -tlnp | grep 8081`
- Check firewall rules allow traffic on port 8081
- Confirm Gateway's `workerIPs` list includes this worker's IP
- Test connectivity: `curl http://<worker-ip>:8081/health`

## Graceful Shutdown

Worker handles `SIGINT` (Ctrl+C) and `SIGTERM` gracefully:

- Stops accepting new connections
- Completes in-flight requests (30s timeout)
- Closes database and Batch API clients
- Exits cleanly

## Future Enhancements

- **Background Status Polling**: Monitor GCP Batch job status and update Spanner
- **Job Cancellation**: Implement job deletion/cancellation endpoint
- **Metrics and Observability**: Add OpenTelemetry instrumentation
- **Configuration via Environment**: Support all config via env vars
- **Retry Logic**: Implement exponential backoff for transient failures
- **Job Validation**: Pre-flight checks for image URI accessibility

## Related Documentation

- [Gateway Service](/cmd/gateway/README.md)
- [Database Schema](/database/schema.sql)
- [GCP Batch Requirements](/docs/jennah-dp-gcp-batch-requirements.md)
- [Project Overview](/README.md)
