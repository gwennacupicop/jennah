package batch

import (
	"context"
	"fmt"
)

// Provider defines the interface for cloud batch service implementations.
// This abstraction enables Jennah to work with different cloud providers
// (GCP Batch, AWS Batch, Azure Batch) and different GCP services
// (Cloud Tasks, Cloud Run Jobs, Cloud Batch) without changing core business logic.
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

	// ServiceType returns the GCP service type this provider implements.
	// Examples: "CLOUD_TASKS", "CLOUD_RUN_JOB", "CLOUD_BATCH".
	ServiceType() string
}

// JobConfig contains the configuration for submitting a batch job.
// This structure is cloud-agnostic and maps to provider-specific formats.
// Fields mirror the frontend SubmitJobRequest proto plus backend-only knobs.
type JobConfig struct {
	// ── Identity ──────────────────────────────────────────────────────────────

	// JobID is the provider-compatible job identifier (e.g., "jennah-abc123").
	// Generated as "jennah-{name}" or "jennah-{uuid[:8]}" if Name is empty.
	JobID string

	// RequestID is used as an idempotency key at the GCP Batch CreateJob API.
	// Defaults to the internal job UUID.
	RequestID string

	// Name is the optional user-facing job label from SubmitJobRequest.name.
	Name string

	// ── Container Execution ───────────────────────────────────────────────────

	// ImageURI is the container image to run (e.g., gcr.io/project/image:tag).
	ImageURI string

	// Commands overrides/appends to the container CMD.
	Commands []string

	// ContainerEntrypoint overrides the container ENTRYPOINT (backend-only).
	ContainerEntrypoint string

	// EnvVars are environment variables passed to the container.
	EnvVars map[string]string

	// ── Compute Resources ─────────────────────────────────────────────────────

	// Resources specifies CPU, memory, and max-run-duration requirements.
	// Resolved from resource_profile + resource_override.
	Resources *ResourceRequirements

	// MachineType requests a specific GCP machine type (e.g., "e2-standard-4").
	// Empty means GCP auto-selects based on CPU/memory.
	MachineType string

	// MinCpuPlatform enforces a specific processor generation (backend-only).
	// Examples: "Intel Cascade Lake", "AMD EPYC Rome".
	MinCpuPlatform string

	// BootDiskSizeGb is boot disk size in GB (default 50, min 10).
	BootDiskSizeGb int64

	// UseSpotVMs selects SPOT provisioning (cheaper, preemptible) when true.
	UseSpotVMs bool

	// Accelerators requests GPU/TPU resources (backend-only).
	Accelerators *AcceleratorConfig

	// ── Networking & Security ─────────────────────────────────────────────────

	// ServiceAccount is the GCP SA email for the job VMs.
	// Empty uses the default Compute Engine service account.
	ServiceAccount string

	// NetworkName is the full VPC network resource name (backend-only).
	NetworkName string

	// SubnetworkName is the full subnetwork resource name (backend-only).
	SubnetworkName string

	// BlockExternalIP disables external IPs on VMs (backend-only).
	BlockExternalIP bool

	// AllowedLocations restricts VM creation to these regions (backend-only).
	AllowedLocations []string

	// ── VM Instance Options ───────────────────────────────────────────────────

	// InstallGpuDrivers auto-installs GPU drivers when true (backend-only).
	InstallGpuDrivers bool

	// InstallOpsAgent auto-installs the GCP Ops Agent when true (backend-only).
	InstallOpsAgent bool

	// BlockProjectSshKeys prevents project-level SSH keys from accessing VMs.
	BlockProjectSshKeys bool

	// ── Task Group ────────────────────────────────────────────────────────────

	// TaskGroup controls parallelism and scheduling within a job.
	TaskGroup *TaskGroupConfig

	// MaxRetryCount is the number of GCP-level task retries (0–10, backend-only).
	MaxRetryCount int32

	// ── Job Metadata ──────────────────────────────────────────────────────────

	// Priority is the job scheduling priority (0–100, backend-only).
	Priority int64

	// JobLabels are key-value labels attached to the GCP Batch job (backend-only).
	JobLabels map[string]string
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

// TaskGroupConfig controls how GCP Batch manages the task group within a job.
type TaskGroupConfig struct {
	// TaskCount is the total number of tasks (default 1).
	TaskCount int64

	// Parallelism is the max concurrent tasks (0 = unlimited).
	Parallelism int64

	// SchedulingPolicy is "AS_SOON_AS_POSSIBLE" (default) or "IN_ORDER".
	SchedulingPolicy string

	// TaskCountPerNode limits tasks per VM (0 = no limit).
	TaskCountPerNode int64

	// RequireHostsFile populates /etc/hosts with all task IPs (for multi-VM jobs).
	RequireHostsFile bool

	// PermissiveSsh enables passwordless SSH between task VMs.
	PermissiveSsh bool

	// RunAsNonRoot enforces non-root container execution.
	RunAsNonRoot bool
}

// AcceleratorConfig specifies GPU/TPU hardware accelerators for a job.
type AcceleratorConfig struct {
	// Type is the accelerator type (e.g., "nvidia-tesla-t4", "nvidia-tesla-v100").
	Type string

	// Count is the number of accelerator units per VM.
	Count int64

	// DriverVersion is an optional specific driver version.
	DriverVersion string
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

// Service type constants used by Provider.ServiceType().
const (
	ServiceTypeCloudTasks  = "CLOUD_TASKS"
	ServiceTypeCloudRunJob = "CLOUD_RUN_JOB"
	ServiceTypeCloudBatch  = "CLOUD_BATCH"
)

// NewProvider creates a new batch provider based on the configuration.
func NewProvider(ctx context.Context, config ProviderConfig) (Provider, error) {
	switch config.Provider {
	case "gcp":
		return newGCPProvider(ctx, config)
	case "gcp-cloudtasks":
		return newGCPCloudTasksProvider(ctx, config)
	case "gcp-cloudrun":
		return newGCPCloudRunProvider(ctx, config)
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
	newGCPProvider           func(context.Context, ProviderConfig) (Provider, error)
	newGCPCloudTasksProvider func(context.Context, ProviderConfig) (Provider, error)
	newGCPCloudRunProvider   func(context.Context, ProviderConfig) (Provider, error)
	newAWSProvider           func(context.Context, ProviderConfig) (Provider, error)
	newAzureProvider         func(context.Context, ProviderConfig) (Provider, error)
)

// RegisterGCPProvider registers the GCP batch provider constructor.
func RegisterGCPProvider(fn func(context.Context, ProviderConfig) (Provider, error)) {
	newGCPProvider = fn
}

// RegisterGCPCloudTasksProvider registers the GCP Cloud Tasks provider constructor.
func RegisterGCPCloudTasksProvider(fn func(context.Context, ProviderConfig) (Provider, error)) {
	newGCPCloudTasksProvider = fn
}

// RegisterGCPCloudRunProvider registers the GCP Cloud Run Jobs provider constructor.
func RegisterGCPCloudRunProvider(fn func(context.Context, ProviderConfig) (Provider, error)) {
	newGCPCloudRunProvider = fn
}

// RegisterAWSProvider registers the AWS batch provider constructor.
func RegisterAWSProvider(fn func(context.Context, ProviderConfig) (Provider, error)) {
	newAWSProvider = fn
}

// RegisterAzureProvider registers the Azure batch provider constructor.
func RegisterAzureProvider(fn func(context.Context, ProviderConfig) (Provider, error)) {
	newAzureProvider = fn
}
