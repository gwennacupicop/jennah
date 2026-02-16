package batch

import (
	"context"
	"fmt"
)

// Provider defines the interface for cloud batch service implementations.
// This abstraction enables Jennah to work with different cloud providers
// (GCP Batch, AWS Batch, Azure Batch) without changing core business logic.
type Provider interface {
	// SubmitJob submits a new batch job to the cloud provider.
	// Returns the internal job ID and cloud resource path (e.g., GCP: projects/.../jobs/..., AWS: ARN).
	SubmitJob(ctx context.Context, config JobConfig) (*JobResult, error)

	// GetJobStatus retrieves the current status of a job.
	GetJobStatus(ctx context.Context, cloudResourcePath string) (JobStatus, error)

	// CancelJob cancels a running job.
	CancelJob(ctx context.Context, cloudResourcePath string) error

	// ListJobs lists all jobs for the configured project/account.
	// Returns cloud resource paths.
	ListJobs(ctx context.Context) ([]string, error)
}

// JobConfig contains the configuration for submitting a batch job.
// This structure is cloud-agnostic and maps to provider-specific formats.
type JobConfig struct {
	// JobID is the provider-compatible job identifier (e.g., "jennah-abc123").
	JobID string

	// ImageURI is the container image to run (e.g., gcr.io/project/image:tag).
	ImageURI string

	// EnvVars are environment variables to pass to the container.
	EnvVars map[string]string

	// Resources specifies compute resource requirements (optional).
	Resources *ResourceRequirements
}

// ResourceRequirements specifies compute resource requirements for a job.
type ResourceRequirements struct {
	// CPUMillis is CPU in milli-cores (1000 = 1 CPU).
	CPUMillis int64

	// MemoryMiB is memory in mebibytes.
	MemoryMiB int64

	// MaxRunDurationSeconds is maximum runtime before timeout (optional).
	MaxRunDurationSeconds int64
}

// JobResult contains the result of submitting a batch job.
type JobResult struct {
	// CloudResourcePath is the full cloud-specific resource identifier.
	// Examples:
	//   - GCP: "projects/my-project/locations/us-central1/jobs/jennah-abc123"
	//   - AWS: "arn:aws:batch:us-east-1:123456789:job/jennah-abc123"
	//   - Azure: "/subscriptions/.../resourceGroups/.../providers/Microsoft.Batch/..."
	CloudResourcePath string

	// InitialStatus is the job status immediately after submission.
	InitialStatus JobStatus
}

// JobStatus represents the status of a batch job.
// This enum maps various cloud provider states to a common set.
type JobStatus string

const (
	// JobStatusPending indicates the job has been accepted but not yet scheduled.
	// Maps to: GCP QUEUED, AWS SUBMITTED/PENDING.
	JobStatusPending JobStatus = "PENDING"

	// JobStatusScheduled indicates the job is scheduled and resources are being allocated.
	// Maps to: GCP SCHEDULED, AWS RUNNABLE/STARTING.
	JobStatusScheduled JobStatus = "SCHEDULED"

	// JobStatusRunning indicates the job is actively executing.
	// Maps to: GCP RUNNING, AWS RUNNING, Azure active.
	JobStatusRunning JobStatus = "RUNNING"

	// JobStatusCompleted indicates the job finished successfully.
	// Maps to: GCP SUCCEEDED, AWS SUCCEEDED, Azure completed (success).
	JobStatusCompleted JobStatus = "COMPLETED"

	// JobStatusFailed indicates the job failed.
	// Maps to: GCP FAILED, AWS FAILED, Azure completed (failure).
	JobStatusFailed JobStatus = "FAILED"

	// JobStatusCancelled indicates the job was cancelled.
	// Maps to: GCP DELETION_IN_PROGRESS, AWS cancelled.
	JobStatusCancelled JobStatus = "CANCELLED"

	// JobStatusUnknown indicates the status could not be determined.
	JobStatusUnknown JobStatus = "UNKNOWN"
)

// ProviderConfig contains configuration for initializing a batch provider.
type ProviderConfig struct {
	// Provider is the cloud provider name ("gcp", "aws", "azure").
	Provider string

	// Region is the cloud region for batch operations.
	Region string

	// ProjectID is used by GCP (project ID) and optionally by other providers.
	ProjectID string

	// ProviderOptions contains provider-specific configuration.
	// Examples:
	//   - GCP: empty (uses projectID and region)
	//   - AWS: {"account_id": "123456789", "job_queue": "my-queue"}
	//   - Azure: {"subscription_id": "...", "resource_group": "..."}
	ProviderOptions map[string]string
}

// NewProvider creates a new batch provider based on the configuration.
func NewProvider(ctx context.Context, config ProviderConfig) (Provider, error) {
	switch config.Provider {
	case "gcp":
		return newGCPProvider(ctx, config)
	case "aws":
		return newAWSProvider(ctx, config)
	case "azure":
		return newAzureProvider(ctx, config)
	default:
		return nil, fmt.Errorf("unsupported batch provider: %s", config.Provider)
	}
}

// Provider-specific constructors (implemented in separate files)
var (
	newGCPProvider   func(context.Context, ProviderConfig) (Provider, error)
	newAWSProvider   func(context.Context, ProviderConfig) (Provider, error)
	newAzureProvider func(context.Context, ProviderConfig) (Provider, error)
)

// RegisterGCPProvider registers the GCP batch provider constructor.
func RegisterGCPProvider(fn func(context.Context, ProviderConfig) (Provider, error)) {
	newGCPProvider = fn
}

// RegisterAWSProvider registers the AWS batch provider constructor.
func RegisterAWSProvider(fn func(context.Context, ProviderConfig) (Provider, error)) {
	newAWSProvider = fn
}

// RegisterAzureProvider registers the Azure batch provider constructor.
func RegisterAzureProvider(fn func(context.Context, ProviderConfig) (Provider, error)) {
	newAzureProvider = fn
}
