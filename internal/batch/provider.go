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

// TaskGroupConfig contains configuration for a group of tasks.
type TaskGroupConfig struct {
	// TaskCount is the total number of tasks in this group (default: 1).
	TaskCount int64

	// Parallelism is the maximum number of tasks to run concurrently.
	// 0 means unlimited (default behavior).
	Parallelism int64

	// SchedulingPolicy determines task execution order.
	// Options: "AS_SOON_AS_POSSIBLE" (default) or "IN_ORDER".
	SchedulingPolicy string

	// TaskCountPerNode limits the number of tasks per VM (default: 1).
	TaskCountPerNode int64

	// RequireHostsFile populates /etc/hosts with all VMs in the task group.
	// Useful for distributed computing and multi-VM coordination.
	RequireHostsFile bool

	// PermissiveSsh enables passwordless SSH between task VMs.
	// Required for distributed frameworks (e.g., Spark, MPI).
	PermissiveSsh bool

	// RunAsNonRoot enforces non-root execution of tasks (optional).
	RunAsNonRoot bool
}

// AcceleratorConfig specifies GPU/TPU requirements.
type AcceleratorConfig struct {
	// Type is the accelerator type (e.g., "nvidia-tesla-t4", "nvidia-tesla-v100", "tpu-v4").
	Type string

	// Count is the number of accelerators to attach.
	Count int64

	// DriverVersion is an optional specific GPU driver version.
	DriverVersion string
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

	// TaskGroup specifies task group configuration (optional, defaults to 1 task).
	TaskGroup *TaskGroupConfig

	// MachineType is the Compute Engine machine type (e.g., "e2-standard-4", "n1-standard-16").
	// If empty, GCP Batch will use a default machine type.
	MachineType string

	// BootDiskSizeGb is the boot disk size in gigabytes (default: 50, range: [10, 65536]).
	BootDiskSizeGb int64

	// UseSpotVMs enables Spot VMs for cost savings (trades availability for price).
	UseSpotVMs bool

	// ServiceAccount is the custom service account email to run tasks under.
	// If empty, the default Compute Engine service account is used.
	ServiceAccount string

	// Commands are the commands to execute in the container (appended to/overrides CMD).
	Commands []string

	// ContainerEntrypoint optionally overrides the container's ENTRYPOINT.
	ContainerEntrypoint string

	// Accelerators specifies GPU/TPU requirements (optional).
	Accelerators *AcceleratorConfig

	// RequestID is an idempotency key (UUID format) for deduplicating job submissions.
	// If not provided, one will be generated.
	RequestID string

	// JobLabels are custom labels applied to the job resource for billing/organization.
	JobLabels map[string]string

	// NetworkName is the VPC network resource path (optional).
	// Example: "projects/my-project/global/networks/my-network"
	NetworkName string

	// SubnetworkName is the subnetwork resource path (optional).
	// Example: "projects/my-project/regions/us-central1/subnetworks/my-subnet"
	SubnetworkName string

	// BlockExternalIP disables external IP assignment for task VMs (private networking).
	BlockExternalIP bool

	// MinCpuPlatform specifies the minimum CPU platform processors (optional).
	// Example: "Intel Cascade Lake", "AMD EPYC Rome"
	MinCpuPlatform string

	// Priority sets job scheduling priority (0-100, where 100 is highest, default: 0).
	Priority int64

	// AllowedLocations restricts where VMs can be created (optional).
	// Example: ["us-central1", "us-west1"]
	AllowedLocations []string

	// InstallGpuDrivers enables automatic GPU driver installation from third-party sources.
	InstallGpuDrivers bool

	// InstallOpsAgent enables automatic installation of Google Cloud Operations Agent.
	InstallOpsAgent bool

	// BlockProjectSshKeys disables project-level SSH keys from accessing VMs.
	// Enhances security by restricting SSH access.
	BlockProjectSshKeys bool

	// MaxRetryCount is the maximum number of task retries on failure (range: [0, 10]).
	// Different from job-level retries; applies at the task granularity.
	MaxRetryCount int32
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
