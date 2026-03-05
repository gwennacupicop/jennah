# Distributed Workload Processing Configuration

## Overview

**Project:** labs-169405  
**Region:** asia-northeast1  
**Service:** GCP Batch  
**Container Registry:** us-central1-docker.pkg.dev/labs-169405/demo-job-repo  

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     GCP Batch Job                           │
├─────────────────────────────────────────────────────────────┤
│  Instance 0          Instance 1          Instance 2         │   Instance 3
│  bytes 0-25%         bytes 25-50%        bytes 50-75%       │   bytes 75-100%
│      ↓                   ↓                   ↓              │       ↓
│  gs://bucket/input/test-data.txt (shared read)              │
│      ↓                   ↓                   ↓              │       ↓
│  gs://bucket/output/instance-{0,1,2,3}.json (each writes)   │
└─────────────────────────────────────────────────────────────┘
```

## Image Path

```
us-central1-docker.pkg.dev/labs-169405/demo-job-repo/demo-job:latest
```

## GCS Bucket

```
gs://bisu-202602/jennah-demo/
├── input/test-data.txt    # Shared input file
└── output/                # Metrics output (instance-*.json)
```

## Build & Push

```bash
# Authenticate
gcloud auth configure-docker us-central1-docker.pkg.dev

# Build
docker build -t us-central1-docker.pkg.dev/labs-169405/demo-job-repo/demo-job:latest \
  -f cmd/demo-job/Dockerfile .

# Push
docker push us-central1-docker.pkg.dev/labs-169405/demo-job-repo/demo-job:latest
```

## Submit Job

```bash
gcloud batch jobs submit JOB_NAME \
  --config=batch-job-config-v2.json \
  --location=asia-northeast1 \
  --impersonate-service-account=gcp-sa-dev-interns@labs-169405.iam.gserviceaccount.com
```

## Job Configuration (batch-job-config-v2.json)

```json
{
  "taskGroups": [{
    "taskCount": "4",
    "parallelism": "4",
    "taskSpec": {
      "runnables": [{
        "container": {
          "imageUri": "us-central1-docker.pkg.dev/labs-169405/demo-job-repo/demo-job:latest"
        },
        "environment": {
          "variables": {
            "INPUT_DATA_PATH": "gs://bisu-202602/jennah-demo/input/test-data.txt",
            "INPUT_DATA_SIZE": "86888890",
            "OUTPUT_BASE_PATH": "gs://bisu-202602/jennah-demo/output",
            "JOB_ID": "demo-job-001",
            "DISTRIBUTION_MODE": "BYTE_RANGE",
            "ENABLE_DISTRIBUTED_MODE": "true"
          }
        }
      }],
      "computeResource": { "cpuMilli": "1000", "memoryMib": "512" },
      "maxRunDuration": "3600s"
    }
  }],
  "allocationPolicy": {
    "location": { "allowedLocations": ["regions/asia-northeast1"] }
  },
  "logsPolicy": { "destination": "CLOUD_LOGGING" }
}
```

## Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `BATCH_TASK_INDEX` | Instance ID (0-based, auto-injected) | `0`, `1`, `2`, `3` |
| `BATCH_TASK_COUNT` | Total instances (auto-injected) | `4` |
| `INPUT_DATA_PATH` | GCS path to input file | `gs://bucket/input/file.txt` |
| `INPUT_DATA_SIZE` | File size in bytes | `86888890` |
| `OUTPUT_BASE_PATH` | GCS path for output | `gs://bucket/output` |

## Monitor & Collect Results

```bash
# Check status
gcloud batch jobs describe JOB_NAME --location=asia-northeast1 --format="value(status.state)"

# Download results
gsutil -m cp "gs://bisu-202602/jennah-demo/output/instance-*.json" ./results/

# Aggregate metrics
go run ./cmd/aggregator --metrics-path ./results --format detailed
```

## Performance (Validated)

| Metric | Value |
|--------|-------|
| Instances | 4 |
| Input Size | 86.9 MB (1M lines) |
| Processing Time | 0.6 seconds |
| Throughput | 34.7 MB/s per instance |
| **Efficiency** | **97.9%** |

## Key Files

| File | Purpose |
|------|---------|
| `cmd/demo-job/main.go` | Main application |
| `cmd/demo-job/Dockerfile` | Container build |
| `internal/demo/chunker.go` | Byte-range distribution |
| `internal/demo/gcs.go` | GCS read/write |
| `internal/demo/processor.go` | Processing logic |
| `cmd/aggregator/main.go` | Results aggregation |

## Notes

- Do NOT specify custom `serviceAccount` in job config — use default Compute Engine SA
- Image registry (us-central1) and job region (asia-northeast1) can differ
- GCS bucket must be accessible by the Compute Engine default SA
- Each instance writes `instance-{INDEX}.json` to output path
