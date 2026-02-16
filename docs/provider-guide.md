# Multi-Cloud Provider Guide

This guide explains how Jennah's multi-cloud provider architecture works and how to implement support for new cloud platforms.

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Configuration](#configuration)
- [Migration Guide](#migration-guide)
- [Implementing a New Provider](#implementing-a-new-provider)
- [Provider Reference](#provider-reference)

---

## Architecture Overview

Jennah uses a **provider interface pattern** to abstract cloud-specific batch orchestration. This enables the worker to submit jobs to different cloud platforms (GCP Batch, AWS Batch, Azure Batch) without changing core business logic.

### Key Components

```
┌─────────────┐
│   Gateway   │  (cloud-agnostic)
└──────┬──────┘
       │
       ▼
┌─────────────┐
│   Worker    │  (uses batch.Provider interface)
└──────┬──────┘
       │
       ▼
┌─────────────────────────────┐
│  batch.Provider Interface   │
└──────┬──────────────────────┘
       │
       ├──► GCP Provider (internal/batch/gcp/)
       ├──► AWS Provider (internal/batch/aws/) [stub]
       └──► Azure Provider (internal/batch/azure/) [future]
```

### Provider Interface

All cloud implementations must satisfy the `batch.Provider` interface:

```go
type Provider interface {
    SubmitJob(ctx context.Context, config JobConfig) (*JobResult, error)
    GetJobStatus(ctx context.Context, cloudResourcePath string) (JobStatus, error)
    CancelJob(ctx context.Context, cloudResourcePath string) error
    ListJobs(ctx context.Context) ([]string, error)
}
```

**Key Concepts**:

- **JobConfig**: Cloud-agnostic job specification (image URI, env vars, resources)
- **JobResult**: Contains `CloudResourcePath` (provider-specific resource identifier)
- **JobStatus**: Enum mapping cloud states to Jennah statuses (PENDING, RUNNING, COMPLETED, etc.)

---

## Configuration

### Environment Variables

Jennah uses environment variables for configuration (12-factor app):

#### Worker Configuration

| Variable                | Description              | Required     | Example                                    |
| ----------------------- | ------------------------ | ------------ | ------------------------------------------ |
| `BATCH_PROVIDER`        | Cloud provider name      | Yes          | `gcp`, `aws`, `azure`                      |
| `BATCH_PROJECT_ID`      | GCP project ID           | GCP only     | `labs-169405`                              |
| `BATCH_REGION`          | Cloud region             | Yes          | `asia-northeast1` (GCP), `us-east-1` (AWS) |
| `AWS_ACCOUNT_ID`        | AWS account ID           | AWS only     | `123456789012`                             |
| `AWS_JOB_QUEUE`         | AWS Batch job queue name | AWS only     | `jennah-job-queue`                         |
| `AZURE_SUBSCRIPTION_ID` | Azure subscription ID    | Azure only   | `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`     |
| `AZURE_RESOURCE_GROUP`  | Azure resource group     | Azure only   | `jennah-resources`                         |
| `DB_PROVIDER`           | Database provider        | Yes          | `spanner`, `dynamodb`, `postgres`          |
| `DB_PROJECT_ID`         | Database project ID      | Spanner only | `labs-169405`                              |
| `DB_INSTANCE`           | Database instance name   | Spanner only | `alphaus-dev`                              |
| `DB_DATABASE`           | Database name            | Yes          | `main`                                     |
| `WORKER_PORT`           | Worker HTTP port         | No           | `8081` (default)                           |

#### Gateway Configuration

Gateway configuration is provider-agnostic:

```bash
./gateway serve \
  --port 8080 \
  --worker-ips "10.146.0.26,10.146.0.27" \
  --db-project-id "labs-169405" \
  --db-instance "alphaus-dev" \
  --db-database "main"
```

### Provider-Specific Examples

#### GCP (Current Default)

```bash
export BATCH_PROVIDER=gcp
export BATCH_PROJECT_ID=labs-169405
export BATCH_REGION=asia-northeast1
export DB_PROVIDER=spanner
export DB_PROJECT_ID=labs-169405
export DB_INSTANCE=alphaus-dev
export DB_DATABASE=main
export WORKER_PORT=8081

./worker
```

#### AWS (Stub Implementation)

```bash
export BATCH_PROVIDER=aws
export BATCH_REGION=us-east-1
export AWS_ACCOUNT_ID=123456789012
export AWS_JOB_QUEUE=jennah-job-queue
export DB_PROVIDER=dynamodb
export DB_REGION=us-east-1
export WORKER_PORT=8081

./worker
```

#### Azure (Future)

```bash
export BATCH_PROVIDER=azure
export BATCH_REGION=eastus
export AZURE_SUBSCRIPTION_ID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
export AZURE_RESOURCE_GROUP=jennah-resources
export DB_PROVIDER=cosmosdb
export DB_ENDPOINT=https://jennah.documents.azure.com:443/
export WORKER_PORT=8081

./worker
```

---

## Migration Guide

### From Hardcoded Config to Environment Variables

**Old** (hardcoded in `cmd/worker/main.go`):

```go
const (
    projectId       = "labs-169405"
    region          = "asia-northeast1"
    spannerInstance = "alphaus-dev"
    spannerDb       = "main"
    workerPort      = "8081"
)
```

**New** (environment variables):

```bash
export BATCH_PROVIDER=gcp
export BATCH_PROJECT_ID=labs-169405
export BATCH_REGION=asia-northeast1
export DB_PROVIDER=spanner
export DB_PROJECT_ID=labs-169405
export DB_INSTANCE=alphaus-dev
export DB_DATABASE=main
export WORKER_PORT=8081
```

### Database Schema Migration

The database schema was updated to be cloud-agnostic:

**Migration Script**: `database/migrate-cloud-resource-path.sql`

```sql
ALTER TABLE Jobs RENAME COLUMN GcpBatchJobName TO CloudJobResourcePath;
```

**Run Migration**:

```bash
# Using gcloud spanner
gcloud spanner databases ddl update main \
  --instance=alphaus-dev \
  --project=labs-169405 \
  --ddl-file=database/migrate-cloud-resource-path.sql
```

**No Data Migration Required**: The column contents remain the same, only the name changes.

### Deployment Changes

#### Docker Environment Variables

Update your `Dockerfile` or container orchestration configs:

```dockerfile
# Old (hardcoded)
# No environment variables

# New (configurable)
ENV BATCH_PROVIDER=gcp
ENV BATCH_PROJECT_ID=labs-169405
ENV BATCH_REGION=asia-northeast1
ENV DB_PROVIDER=spanner
ENV DB_PROJECT_ID=labs-169405
ENV DB_INSTANCE=alphaus-dev
ENV DB_DATABASE=main
```

#### Cloud Run Deployment

```bash
# Deploy worker with environment variables
gcloud run deploy jennah-worker \
  --image=asia-docker.pkg.dev/labs-169405/jennah/worker:latest \
  --region=asia-northeast1 \
  --set-env-vars="BATCH_PROVIDER=gcp,BATCH_PROJECT_ID=labs-169405,BATCH_REGION=asia-northeast1,DB_PROVIDER=spanner,DB_PROJECT_ID=labs-169405,DB_INSTANCE=alphaus-dev,DB_DATABASE=main"
```

---

## Implementing a New Provider

Follow these steps to add support for a new cloud platform:

### 1. Create Provider Package

Create a new directory: `internal/batch/{provider}/`

```
internal/batch/
├── provider.go           # Interface definition
├── gcp/
│   └── client.go        # GCP implementation
├── aws/
│   └── client.go        # AWS implementation
└── azure/               # Your new provider
    └── client.go
```

### 2. Implement Provider Interface

```go
package azure

import (
    "context"
    "fmt"

    batchpkg "github.com/alphauslabs/jennah/internal/batch"
)

func init() {
    // Register provider constructor
    batchpkg.RegisterAzureProvider(NewAzureBatchProvider)
}

type AzureBatchProvider struct {
    // Azure Batch client
    subscriptionID string
    resourceGroup  string
    region         string
}

func NewAzureBatchProvider(ctx context.Context, config batchpkg.ProviderConfig) (batchpkg.Provider, error) {
    // Validate required config
    subscriptionID := config.ProviderOptions["subscription_id"]
    if subscriptionID == "" {
        return nil, fmt.Errorf("subscription_id is required")
    }

    // Initialize Azure SDK client
    // ...

    return &AzureBatchProvider{
        subscriptionID: subscriptionID,
        resourceGroup:  config.ProviderOptions["resource_group"],
        region:         config.Region,
    }, nil
}

func (p *AzureBatchProvider) SubmitJob(ctx context.Context, config batchpkg.JobConfig) (*batchpkg.JobResult, error) {
    // 1. Create Azure Batch pool/job definition
    // 2. Submit job to Azure Batch
    // 3. Return CloudResourcePath (Azure resource ID format)

    return &batchpkg.JobResult{
        CloudResourcePath: "/subscriptions/.../resourceGroups/.../providers/Microsoft.Batch/...",
        InitialStatus:     batchpkg.JobStatusPending,
    }, nil
}

func (p *AzureBatchProvider) GetJobStatus(ctx context.Context, cloudResourcePath string) (batchpkg.JobStatus, error) {
    // Query Azure Batch API for job status
    // Map Azure status to Jennah JobStatus enum
    return batchpkg.JobStatusRunning, nil
}

func (p *AzureBatchProvider) CancelJob(ctx context.Context, cloudResourcePath string) error {
    // Call Azure Batch terminate/delete API
    return nil
}

func (p *AzureBatchProvider) ListJobs(ctx context.Context) ([]string, error) {
    // List jobs from Azure Batch
    return []string{}, nil
}
```

### 3. Map Cloud States to Jennah Status

Each provider must map its native job states to Jennah's status enum:

```go
func mapAzureStatusToJennah(azureStatus string) batchpkg.JobStatus {
    switch azureStatus {
    case "active":
        return batchpkg.JobStatusScheduled
    case "running":
        return batchpkg.JobStatusRunning
    case "completed":
        return batchpkg.JobStatusCompleted
    case "failed":
        return batchpkg.JobStatusFailed
    default:
        return batchpkg.JobStatusUnknown
    }
}
```

### 4. Import Provider in Worker

Update `cmd/worker/main.go`:

```go
import (
    _ "github.com/alphauslabs/jennah/internal/batch/gcp"   // Register GCP
    _ "github.com/alphauslabs/jennah/internal/batch/aws"   // Register AWS
    _ "github.com/alphauslabs/jennah/internal/batch/azure" // Register Azure
)
```

The `init()` function in each provider package automatically registers the constructor.

### 5. Update Configuration Validation

Update `internal/config/config.go` validation:

```go
func (c *Config) Validate() error {
    switch c.BatchProvider.Provider {
    case "gcp":
        // GCP validation...
    case "aws":
        // AWS validation...
    case "azure":
        if c.BatchProvider.ProviderOptions["subscription_id"] == "" {
            return fmt.Errorf("AZURE_SUBSCRIPTION_ID is required")
        }
        // More Azure validation...
    }
}
```

### 6. Test Provider

Create unit tests:

```go
func TestAzureBatchProvider_SubmitJob(t *testing.T) {
    ctx := context.Background()

    provider, err := NewAzureBatchProvider(ctx, batchpkg.ProviderConfig{
        Provider: "azure",
        Region:   "eastus",
        ProviderOptions: map[string]string{
            "subscription_id":  "test-sub-id",
            "resource_group":   "test-rg",
        },
    })

    require.NoError(t, err)

    result, err := provider.SubmitJob(ctx, batchpkg.JobConfig{
        JobID:    "test-job-123",
        ImageURI: "test.azurecr.io/app:latest",
    })

    require.NoError(t, err)
    assert.NotEmpty(t, result.CloudResourcePath)
}
```

---

## Provider Reference

### GCP Batch Provider

**Status**: ✅ Fully Implemented

**Location**: `internal/batch/gcp/client.go`

**Configuration**:

```bash
BATCH_PROVIDER=gcp
BATCH_PROJECT_ID=<project-id>
BATCH_REGION=<region>
```

**Resource Path Format**: `projects/{project}/locations/{region}/jobs/{job-id}`

**State Mapping**:

- `QUEUED` → `PENDING`
- `SCHEDULED` → `SCHEDULED`
- `RUNNING` → `RUNNING`
- `SUCCEEDED` → `COMPLETED`
- `FAILED` → `FAILED`
- `DELETION_IN_PROGRESS` → `CANCELLED`

**Requirements**:

- GCP Batch API enabled
- Service account with `roles/batch.jobsEditor`
- Container images in GCR/Artifact Registry

**Documentation**: See [gcp-batch-sdk-guide.md](gcp-batch-sdk-guide.md)

---

### AWS Batch Provider

**Status**: 🚧 Stub Implementation

**Location**: `internal/batch/aws/client.go`

**Configuration**:

```bash
BATCH_PROVIDER=aws
BATCH_REGION=<region>
AWS_ACCOUNT_ID=<account-id>
AWS_JOB_QUEUE=<job-queue-name>
```

**Resource Path Format**: `arn:aws:batch:{region}:{account}:job/{job-id}`

**State Mapping**:

- `SUBMITTED`, `PENDING` → `PENDING`
- `RUNNABLE`, `STARTING` → `SCHEDULED`
- `RUNNING` → `RUNNING`
- `SUCCEEDED` → `COMPLETED`
- `FAILED` → `FAILED`

**Requirements** (for full implementation):

- AWS Batch job queue created
- Compute environment configured
- IAM permissions for `batch:*` operations
- Container images in ECR

**TODO**:

- [ ] Implement `SubmitJob` with AWS SDK v2
- [ ] Implement `GetJobStatus` with DescribeJobs API
- [ ] Implement `CancelJob` with TerminateJob API
- [ ] Implement `ListJobs` with pagination
- [ ] Add integration tests

---

### Azure Batch Provider

**Status**: ⏳ Not Yet Implemented

**Planned Configuration**:

```bash
BATCH_PROVIDER=azure
BATCH_REGION=<region>
AZURE_SUBSCRIPTION_ID=<subscription-id>
AZURE_RESOURCE_GROUP=<resource-group>
```

**Resource Path Format**: `/subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.Batch/batchAccounts/{account}/jobs/{job-id}`

**State Mapping** (planned):

- `active` → `SCHEDULED`
- `running` → `RUNNING`
- `completed` (success) → `COMPLETED`
- `completed` (failure) → `FAILED`

---

## Best Practices

### Provider Implementation

1. **Error Handling**: Wrap cloud SDK errors with context

   ```go
   if err != nil {
       return nil, fmt.Errorf("failed to submit AWS Batch job: %w", err)
   }
   ```

2. **Timeouts**: Use context deadlines for long operations

   ```go
   ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
   defer cancel()
   ```

3. **Idempotency**: Handle duplicate job submissions gracefully

   ```go
   if err == AlreadyExistsError {
       // Return existing job instead of error
   }
   ```

4. **Resource Cleanup**: Close clients in provider Close() method

5. **Logging**: Log important events for debugging
   ```go
   log.Printf("Submitted job to AWS Batch: %s", jobArn)
   ```

### Configuration Management

1. **Validation**: Fail fast with clear error messages
2. **Defaults**: Provide sensible defaults where possible
3. **Documentation**: Document required vs optional config
4. **Secrets**: Use secret managers for sensitive config (API keys, etc.)

### Testing

1. **Unit Tests**: Mock cloud SDK clients
2. **Integration Tests**: Test against real cloud APIs (dev environment)
3. **Error Cases**: Test network failures, quota errors, invalid config

---

## Troubleshooting

### Provider Not Found

**Error**: `unsupported batch provider: xyz`

**Solution**:

1. Check `BATCH_PROVIDER` environment variable
2. Ensure provider package is imported in worker main
3. Verify provider's `init()` function calls `Register*Provider()`

### Configuration Validation Failed

**Error**: `BATCH_PROJECT_ID is required for GCP batch provider`

**Solution**: Set all required environment variables for your provider

### Job Submission Failed

**Error**: `failed to submit batch job: permission denied`

**Solution**:

1. Check IAM permissions for service account
2. Verify cloud API is enabled (GCP Batch API, etc.)
3. Check resource quotas

### Database Connection Failed

**Error**: `failed to initialize database client`

**Solution**:

1. Verify database credentials
2. Check network connectivity
3. Ensure database exists and schema is up-to-date

---

## Additional Resources

- [GCP Batch SDK Guide](gcp-batch-sdk-guide.md) - Comprehensive GCP Batch reference
- [Architecture Documentation](../.github/copilot-instructions.md) - Overall system architecture
- [Database Schema](../database/schema.sql) - Database structure reference
- [Proto Definitions](../proto/jennah.proto) - API contract

---

## Contributing

When implementing a new provider:

1. Follow the provider interface contract exactly
2. Add comprehensive documentation to this guide
3. Include configuration examples
4. Add integration tests
5. Update configuration validation
6. Submit PR with implementation and docs

Questions? Open an issue or contact the maintainers.
