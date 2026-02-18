# GCP Batch Go SDK Client Guide

## Overview

The GCP Batch Go SDK provides a Go client library for interacting with Google Cloud Batch API. This guide covers usage patterns specific to Jennah's worker implementation.

**Package**: `cloud.google.com/go/batch/apiv1`  
**Import**: `cloud.google.com/go/batch/apiv1/batchpb`  
**Official Docs**: https://pkg.go.dev/cloud.google.com/go/batch/apiv1

## Installation

```bash
go get cloud.google.com/go/batch/apiv1
```

## Client Initialization

### Basic Setup

```go
import (
    "context"
    batch "cloud.google.com/go/batch/apiv1"
    "cloud.google.com/go/batch/apiv1/batchpb"
)

func initBatchClient(ctx context.Context) (*batch.Client, error) {
    // Uses Application Default Credentials (ADC)
    client, err := batch.NewClient(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to create batch client: %w", err)
    }
    return client, nil
}

// Always close client when done
defer client.Close()
```

### Authentication

The client uses [Application Default Credentials (ADC)](https://cloud.google.com/docs/authentication/application-default-credentials):

1. **Local Development**: `gcloud auth application-default login`
2. **GCE/Cloud Run**: Automatic via service account
3. **Manual Credentials**: Set `GOOGLE_APPLICATION_CREDENTIALS` env var

```bash
# For local development
gcloud auth application-default login

# Or set service account key
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/key.json"
```

## Core Operations

### 1. CreateJob - Submit a Batch Job

Creates a new batch job that runs a containerized workload.

#### Method Signature

```go
func (c *Client) CreateJob(
    ctx context.Context,
    req *batchpb.CreateJobRequest,
    opts ...gax.CallOption,
) (*batchpb.Job, error)
```

#### Request Structure

```go
type CreateJobRequest struct {
    // Parent: projects/{project}/locations/{location}
    Parent string

    // JobId: DNS-compliant ID (lowercase, alphanumeric, hyphens)
    // Pattern: ^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$
    JobId string

    // Job: The job configuration
    Job *Job
}
```

#### Complete Example

```go
func createGCPBatchJob(
    ctx context.Context,
    client *batch.Client,
    projectID string,
    region string,
    jobID string,
    imageURI string,
    envVars map[string]string,
) (*batchpb.Job, error) {
    // 1. Define the container runnable
    runnable := &batchpb.Runnable{
        Executable: &batchpb.Runnable_Container_{
            Container: &batchpb.Runnable_Container{
                ImageUri: imageURI,
                // Optional: Override container command
                // Commands: []string{"/bin/bash", "-c", "echo Hello"},
            },
        },
    }

    // 2. Add environment variables (optional)
    if len(envVars) > 0 {
        runnable.Environment = &batchpb.Environment{
            Variables: envVars,
        }
    }

    // 3. Create task specification (single task)
    task := &batchpb.TaskSpec{
        Runnables: []*batchpb.Runnable{runnable},
        ComputeResource: &batchpb.ComputeResource{
            CpuMilli:  2000, // 2 vCPUs
            MemoryMib: 4096, // 4 GB RAM
        },
        MaxRunDuration: &durationpb.Duration{
            Seconds: 3600, // 1 hour timeout
        },
    }

    // 4. Create task group (collection of tasks)
    taskGroup := &batchpb.TaskGroup{
        TaskCount: 1, // Number of tasks to run
        TaskSpec:  task,
    }

    // 5. Create job with allocation policy
    job := &batchpb.Job{
        TaskGroups: []*batchpb.TaskGroup{taskGroup},
        AllocationPolicy: &batchpb.AllocationPolicy{
            Instances: []*batchpb.AllocationPolicy_InstancePolicyOrTemplate{
                {
                    PolicyTemplate: &batchpb.AllocationPolicy_InstancePolicyOrTemplate_Policy{
                        Policy: &batchpb.AllocationPolicy_InstancePolicy{
                            MachineType: "e2-standard-2", // VM machine type
                        },
                    },
                },
            },
        },
        LogsPolicy: &batchpb.LogsPolicy{
            Destination: batchpb.LogsPolicy_CLOUD_LOGGING,
        },
    }

    // 6. Submit the job
    parent := fmt.Sprintf("projects/%s/locations/%s", projectID, region)
    req := &batchpb.CreateJobRequest{
        Parent: parent,
        JobId:  jobID, // Must match DNS naming: ^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$
        Job:    job,
    }

    batchJob, err := client.CreateJob(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("failed to create batch job: %w", err)
    }

    log.Printf("Created batch job: %s", batchJob.Name)
    return batchJob, nil
}
```

#### Job States

| State                  | Description                                        |
| ---------------------- | -------------------------------------------------- |
| `STATE_UNSPECIFIED`    | Unknown state                                      |
| `QUEUED`               | Job accepted, awaiting VM allocation               |
| `SCHEDULED`            | Job scheduled, VM is starting                      |
| `RUNNING`              | Job is actively running                            |
| `SUCCEEDED`            | Job completed successfully (all tasks exit code 0) |
| `FAILED`               | Job failed (task exit code != 0 or system error)   |
| `DELETION_IN_PROGRESS` | Job being deleted                                  |

### 2. GetJob - Query Job Status

Retrieves current state and metadata of a job.

```go
func getJobStatus(
    ctx context.Context,
    client *batch.Client,
    jobName string, // Full resource name: projects/.../locations/.../jobs/...
) (*batchpb.Job, error) {
    req := &batchpb.GetJobRequest{
        Name: jobName,
    }

    job, err := client.GetJob(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("failed to get job: %w", err)
    }

    log.Printf("Job state: %s", job.Status.State)
    return job, nil
}

// Access job status
// job.Status.State - Current state (RUNNING, SUCCEEDED, etc.)
// job.Status.StatusEvents - Timeline of state changes
// job.Status.RunDuration - How long job has been running
```

### 3. ListJobs - Query Multiple Jobs

Retrieves jobs with pagination support.

```go
func listJobs(
    ctx context.Context,
    client *batch.Client,
    projectID string,
    region string,
) ([]*batchpb.Job, error) {
    parent := fmt.Sprintf("projects/%s/locations/%s", projectID, region)

    req := &batchpb.ListJobsRequest{
        Parent: parent,
        // Optional filters
        Filter: "state=RUNNING", // Filter by state
    }

    it := client.ListJobs(ctx, req)

    var jobs []*batchpb.Job
    for {
        job, err := it.Next()
        if err == iterator.Done {
            break
        }
        if err != nil {
            return nil, fmt.Errorf("failed to iterate jobs: %w", err)
        }
        jobs = append(jobs, job)
    }

    return jobs, nil
}

// Using iterator with Go 1.23+ range over func
for job, err := range client.ListJobs(ctx, req).All() {
    if err != nil {
        return err
    }
    fmt.Printf("Job: %s, State: %s\n", job.Name, job.Status.State)
}
```

### 4. DeleteJob - Remove a Job

Deletes a job (long-running operation).

```go
func deleteJob(
    ctx context.Context,
    client *batch.Client,
    jobName string,
) error {
    req := &batchpb.DeleteJobRequest{
        Name: jobName,
    }

    op, err := client.DeleteJob(ctx, req)
    if err != nil {
        return fmt.Errorf("failed to start delete operation: %w", err)
    }

    // Wait for deletion to complete
    if err := op.Wait(ctx); err != nil {
        return fmt.Errorf("delete operation failed: %w", err)
    }

    log.Printf("Job deleted: %s", jobName)
    return nil
}
```

### 5. CancelJob - Stop a Running Job

Cancels a running job (long-running operation).

```go
func cancelJob(
    ctx context.Context,
    client *batch.Client,
    jobName string,
) error {
    req := &batchpb.CancelJobRequest{
        Name: jobName,
    }

    op, err := client.CancelJob(ctx, req)
    if err != nil {
        return fmt.Errorf("failed to start cancel operation: %w", err)
    }

    // Wait for cancellation to complete
    resp, err := op.Wait(ctx)
    if err != nil {
        return fmt.Errorf("cancel operation failed: %w", err)
    }

    log.Printf("Job cancelled: %s", resp)
    return nil
}
```

## Advanced Configuration

### Compute Resources

Specify CPU and memory requirements:

```go
task := &batchpb.TaskSpec{
    Runnables: []*batchpb.Runnable{runnable},
    ComputeResource: &batchpb.ComputeResource{
        CpuMilli:      4000,  // 4 vCPUs (1 CPU = 1000 milli)
        MemoryMib:     8192,  // 8 GB RAM
        BootDiskMib:   50000, // 50 GB boot disk (optional)
    },
    MaxRetryCount: 3, // Retry failed tasks up to 3 times
}
```

### Machine Type Selection

Control VM specifications:

```go
job := &batchpb.Job{
    AllocationPolicy: &batchpb.AllocationPolicy{
        Instances: []*batchpb.AllocationPolicy_InstancePolicyOrTemplate{
            {
                PolicyTemplate: &batchpb.AllocationPolicy_InstancePolicyOrTemplate_Policy{
                    Policy: &batchpb.AllocationPolicy_InstancePolicy{
                        MachineType: "n1-standard-4",     // 4 vCPU, 15 GB RAM
                        // Or use custom machine type
                        // MachineType: "custom-4-16384", // 4 vCPU, 16 GB RAM

                        // Spot VMs for cost savings
                        ProvisioningModel: batchpb.AllocationPolicy_SPOT,
                    },
                },
            },
        },
        // Placement policy (optional)
        Location: &batchpb.AllocationPolicy_LocationPolicy{
            AllowedLocations: []string{"zones/us-central1-a"},
        },
    },
}
```

### Timeouts and Deadlines

```go
import "google.golang.org/protobuf/types/known/durationpb"

task := &batchpb.TaskSpec{
    Runnables: []*batchpb.Runnable{runnable},
    MaxRunDuration: &durationpb.Duration{
        Seconds: 7200, // Max 2 hours per task
    },
    MaxRetryCount: 2, // Retry up to 2 times on failure
}
```

### Environment Variables

Multiple ways to set environment variables:

```go
// Method 1: On the runnable
runnable := &batchpb.Runnable{
    Executable: &batchpb.Runnable_Container_{
        Container: &batchpb.Runnable_Container{
            ImageUri: "gcr.io/my-project/app:v1",
        },
    },
    Environment: &batchpb.Environment{
        Variables: map[string]string{
            "PROJECT_ID": "my-project",
            "REGION":     "us-central1",
            "DEBUG":      "true",
        },
    },
}

// Method 2: Encrypted secrets (for sensitive data)
runnable.Environment = &batchpb.Environment{
    SecretVariables: map[string]string{
        "API_KEY": "projects/123/secrets/api-key/versions/latest",
    },
}
```

### Multiple Tasks (Parallel Execution)

Run multiple identical tasks in parallel:

```go
taskGroup := &batchpb.TaskGroup{
    TaskCount: 10, // Run 10 parallel tasks
    TaskSpec:  task,
    Parallelism: 5, // Max 5 tasks running concurrently
}

// Each task gets unique environment variables
// BATCH_TASK_INDEX: 0, 1, 2, ..., 9
// BATCH_TASK_COUNT: 10
```

### Custom Service Account

Specify service account for job execution:

```go
job := &batchpb.Job{
    AllocationPolicy: &batchpb.AllocationPolicy{
        ServiceAccount: &batchpb.ServiceAccount{
            Email: "batch-sa@my-project.iam.gserviceaccount.com",
            // Scopes: Default scopes are usually sufficient
        },
    },
}
```

## Error Handling

### Common Error Patterns

```go
import (
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

func handleBatchError(err error) {
    if err == nil {
        return
    }

    // Extract gRPC status
    st, ok := status.FromError(err)
    if !ok {
        log.Printf("Non-gRPC error: %v", err)
        return
    }

    switch st.Code() {
    case codes.InvalidArgument:
        // Job ID doesn't match pattern, missing required fields
        log.Printf("Invalid request: %s", st.Message())

    case codes.PermissionDenied:
        // Insufficient permissions
        log.Printf("Permission denied. Check IAM roles")

    case codes.ResourceExhausted:
        // Quota exceeded
        log.Printf("Quota exceeded. Request increase or retry later")

    case codes.AlreadyExists:
        // Job ID already exists
        log.Printf("Job already exists. Use a different JobId")

    case codes.NotFound:
        // Job doesn't exist
        log.Printf("Job not found: %s", st.Message())

    case codes.DeadlineExceeded:
        // Request timeout
        log.Printf("Request timed out. Retry with longer deadline")

    default:
        log.Printf("Batch API error [%s]: %s", st.Code(), st.Message())
    }
}
```

### Retry with Context Timeout

```go
import "time"

func createJobWithRetry(ctx context.Context, client *batch.Client, req *batchpb.CreateJobRequest) (*batchpb.Job, error) {
    // Set timeout
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    // Retry logic
    maxRetries := 3
    for i := 0; i < maxRetries; i++ {
        job, err := client.CreateJob(ctx, req)
        if err == nil {
            return job, nil
        }

        st, ok := status.FromError(err)
        if ok && st.Code() == codes.ResourceExhausted {
            // Exponential backoff
            backoff := time.Duration(i+1) * 2 * time.Second
            log.Printf("Quota exceeded, retrying in %v", backoff)
            time.Sleep(backoff)
            continue
        }

        return nil, err // Non-retryable error
    }

    return nil, fmt.Errorf("max retries exceeded")
}
```

## Jennah-Specific Integration

### Worker Implementation Pattern

```go
package main

import (
    "context"
    "fmt"
    "log"

    batch "cloud.google.com/go/batch/apiv1"
    "cloud.google.com/go/batch/apiv1/batchpb"
    "github.com/google/uuid"
    "strings"
)

type WorkerServer struct {
    batchClient *batch.Client
    projectId   string
    region      string
}

func (s *WorkerServer) SubmitJob(
    ctx context.Context,
    tenantId string,
    imageUri string,
    envVars map[string]string,
) (string, string, error) {
    // 1. Generate IDs
    internalJobID := uuid.New().String()
    batchJobID := "jennah-" + internalJobID[:8] // GCP-compliant ID

    // 2. Create GCP Batch job
    batchJob, err := s.createGCPBatchJob(ctx, batchJobID, imageUri, envVars)
    if err != nil {
        return "", "", fmt.Errorf("failed to create GCP Batch job: %w", err)
    }

    // 3. Extract GCP resource name
    gcpBatchJobName := batchJob.Name // projects/.../locations/.../jobs/jennah-xxx

    // 4. Return both IDs for database storage
    return internalJobID, gcpBatchJobName, nil
}

func (s *WorkerServer) createGCPBatchJob(
    ctx context.Context,
    jobID string,
    imageURI string,
    envVars map[string]string,
) (*batchpb.Job, error) {
    runnable := &batchpb.Runnable{
        Executable: &batchpb.Runnable_Container_{
            Container: &batchpb.Runnable_Container{
                ImageUri: imageURI,
            },
        },
    }

    if len(envVars) > 0 {
        runnable.Environment = &batchpb.Environment{
            Variables: envVars,
        }
    }

    task := &batchpb.TaskSpec{
        Runnables: []*batchpb.Runnable{runnable},
        ComputeResource: &batchpb.ComputeResource{
            CpuMilli:  2000,
            MemoryMib: 4096,
        },
    }

    job := &batchpb.Job{
        TaskGroups: []*batchpb.TaskGroup{{
            TaskCount: 1,
            TaskSpec:  task,
        }},
        LogsPolicy: &batchpb.LogsPolicy{
            Destination: batchpb.LogsPolicy_CLOUD_LOGGING,
        },
    }

    parent := fmt.Sprintf("projects/%s/locations/%s", s.projectId, s.region)
    req := &batchpb.CreateJobRequest{
        Parent: parent,
        JobId:  jobID,
        Job:    job,
    }

    return s.batchClient.CreateJob(ctx, req)
}
```

### Polling Job Status

```go
func (s *WorkerServer) PollJobStatus(ctx context.Context, gcpBatchJobName string) (string, error) {
    job, err := s.batchClient.GetJob(ctx, &batchpb.GetJobRequest{
        Name: gcpBatchJobName,
    })
    if err != nil {
        return "", err
    }

    // Map GCP Batch state to Jennah status
    switch job.Status.State {
    case batchpb.JobStatus_QUEUED, batchpb.JobStatus_SCHEDULED:
        return "SCHEDULED", nil
    case batchpb.JobStatus_RUNNING:
        return "RUNNING", nil
    case batchpb.JobStatus_SUCCEEDED:
        return "COMPLETED", nil
    case batchpb.JobStatus_FAILED:
        return "FAILED", nil
    default:
        return "UNKNOWN", nil
    }
}
```

## Resource Names Format

All GCP Batch resources use this naming format:

```
projects/{project}/locations/{location}/jobs/{job}
```

**Examples**:

```
projects/labs-169405/locations/us-central1/jobs/jennah-306b2e4f
```

## IAM Permissions Required

The service account running the worker needs these IAM roles:

```yaml
# Minimum permissions
roles/batch.jobsEditor           # Create, update, delete jobs
roles/batch.jobsViewer           # List and get jobs

# For Cloud Logging access
roles/logging.viewer             # View job logs

# If using custom service accounts
roles/iam.serviceAccountUser     # Use custom service account for jobs
```

Grant permissions:

```bash
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:SERVICE_ACCOUNT_EMAIL" \
  --role="roles/batch.jobsEditor"
```

## Monitoring and Logging

### Access Job Logs

```bash
# View logs for a specific job
gcloud logging read "resource.type=generic_task AND resource.labels.job_id=jennah-306b2e4f" \
  --project=labs-169405 \
  --limit=100
```

### View Job in Console

```
https://console.cloud.google.com/batch/jobs?project=labs-169405
```

### Programmatic Log Access

```go
import (
    logging "cloud.google.com/go/logging"
)

func getJobLogs(ctx context.Context, projectID, jobID string) ([]string, error) {
    client, err := logging.NewClient(ctx, projectID)
    if err != nil {
        return nil, err
    }
    defer client.Close()

    filter := fmt.Sprintf(`resource.type="generic_task" AND resource.labels.job_id="%s"`, jobID)
    iter := client.Logger("batch").Entries(ctx, logging.Filter(filter))

    var logs []string
    for {
        entry, err := iter.Next()
        if err == iterator.Done {
            break
        }
        if err != nil {
            return nil, err
        }
        logs = append(logs, entry.Payload.(string))
    }

    return logs, nil
}
```

## Best Practices

### 1. Job ID Generation

```go
// ✅ CORRECT: DNS-compliant
jobID := "jennah-" + uuid.New().String()[:8]  // jennah-306b2e4f

// ❌ WRONG: Uppercase, underscores
jobID := uuid.New().String()  // 306b2e4f-7fdd-4449-bb72-1565acc7edef
```

### 2. Error Handling

```go
// Always check and log errors with context
job, err := client.CreateJob(ctx, req)
if err != nil {
    log.Printf("CreateJob failed for tenant %s: %v", tenantID, err)
    return nil, fmt.Errorf("failed to create batch job: %w", err)
}
```

### 3. Resource Cleanup

```go
// Always close client
defer client.Close()

// Clean up failed jobs (optional)
if err := client.DeleteJob(ctx, &batchpb.DeleteJobRequest{Name: jobName}); err != nil {
    log.Printf("Failed to delete job: %v", err)
}
```

### 4. Use Context Timeouts

```go
// Set reasonable timeouts
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

job, err := client.CreateJob(ctx, req)
```

### 5. Implement Exponential Backoff

```go
import "github.com/googleapis/gax-go/v2"

// Retry with exponential backoff
opts := []gax.CallOption{
    gax.WithRetry(func() gax.Retryer {
        return gax.OnCodes([]codes.Code{
            codes.Unavailable,
            codes.ResourceExhausted,
        }, gax.Backoff{
            Initial:    1 * time.Second,
            Max:        10 * time.Second,
            Multiplier: 2,
        })
    }),
}

job, err := client.CreateJob(ctx, req, opts...)
```

## Troubleshooting

### Issue: "Job Id field is invalid"

**Solution**: Use DNS-compliant job IDs (lowercase, start with letter, no underscores)

```go
jobID := "jennah-" + strings.ToLower(uuid.New().String()[:8])
```

### Issue: "Permission denied"

**Solution**: Grant required IAM roles

```bash
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:SA_EMAIL" \
  --role="roles/batch.jobsEditor"
```

### Issue: "Quota exceeded"

**Solution**: Request quota increase or use exponential backoff retry

### Issue: Job stuck in QUEUED

**Possible causes**:

- No available VMs in the region
- Quota limits reached
- Invalid machine type specification

**Check**: `gcloud batch jobs describe JOB_NAME --location=REGION`

## References

- **Go SDK Docs**: https://pkg.go.dev/cloud.google.com/go/batch/apiv1
- **Batch API Reference**: https://cloud.google.com/batch/docs/reference/rest
- **Code Samples**: https://github.com/GoogleCloudPlatform/golang-samples/tree/main/batch
- **Authentication**: https://cloud.google.com/docs/authentication/application-default-credentials
- **Quotas**: https://cloud.google.com/batch/quotas

## Quick Reference

### Essential Methods

| Method                | Purpose        | Returns                        |
| --------------------- | -------------- | ------------------------------ |
| `CreateJob(ctx, req)` | Submit new job | `*Job`, `error`                |
| `GetJob(ctx, req)`    | Get job status | `*Job`, `error`                |
| `ListJobs(ctx, req)`  | List jobs      | `*JobIterator`                 |
| `DeleteJob(ctx, req)` | Delete job     | `*DeleteJobOperation`, `error` |
| `CancelJob(ctx, req)` | Cancel job     | `*CancelJobOperation`, `error` |

### Job Lifecycle

```
QUEUED → SCHEDULED → RUNNING → SUCCEEDED
                               ↘ FAILED
```

### Machine Types Reference

| Type             | vCPUs | Memory | Use Case          |
| ---------------- | ----- | ------ | ----------------- |
| `e2-micro`       | 2     | 1 GB   | Minimal workloads |
| `e2-standard-2`  | 2     | 8 GB   | Light processing  |
| `e2-standard-4`  | 4     | 16 GB  | Medium workloads  |
| `n1-standard-4`  | 4     | 15 GB  | General purpose   |
| `c2-standard-8`  | 8     | 32 GB  | CPU-intensive     |
| `custom-4-16384` | 4     | 16 GB  | Custom config     |
