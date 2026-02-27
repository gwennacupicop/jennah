package gcp

import (
	"context"
	"fmt"
	"time"

	batch "cloud.google.com/go/batch/apiv1"
	"cloud.google.com/go/batch/apiv1/batchpb"
	"google.golang.org/protobuf/types/known/durationpb"

	batchpkg "github.com/alphauslabs/jennah/internal/batch"
)

func init() {
	// Register GCP provider constructor
	batchpkg.RegisterGCPProvider(NewGCPBatchProvider)
}

// GCPBatchProvider implements the batch.Provider interface for Google Cloud Batch.
type GCPBatchProvider struct {
	client    *batch.Client
	projectID string
	region    string
}

// NewGCPBatchProvider creates a new GCP Batch provider.
func NewGCPBatchProvider(ctx context.Context, config batchpkg.ProviderConfig) (batchpkg.Provider, error) {
	if config.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required for GCP batch provider")
	}
	if config.Region == "" {
		return nil, fmt.Errorf("region is required for GCP batch provider")
	}

	client, err := batch.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCP Batch client: %w", err)
	}

	return &GCPBatchProvider{
		client:    client,
		projectID: config.ProjectID,
		region:    config.Region,
	}, nil
}

// SubmitJob submits a new batch job to GCP Batch.
func (p *GCPBatchProvider) SubmitJob(ctx context.Context, config batchpkg.JobConfig) (*batchpkg.JobResult, error) {
	parent := fmt.Sprintf("projects/%s/locations/%s", p.projectID, p.region)

	// Create container runnable with image and optional overrides
	container := &batchpb.Runnable_Container{
		ImageUri: config.ImageURI,
	}

	// Add commands if provided
	if len(config.Commands) > 0 {
		container.Commands = config.Commands
	}

	// Add entrypoint if provided
	if config.ContainerEntrypoint != "" {
		container.Entrypoint = config.ContainerEntrypoint
	}

	runnable := &batchpb.Runnable{
		Executable: &batchpb.Runnable_Container_{
			Container: container,
		},
	}

	// Add environment variables if provided
	if len(config.EnvVars) > 0 {
		runnable.Environment = &batchpb.Environment{
			Variables: config.EnvVars,
		}
	}

	// Create task specification
	taskSpec := &batchpb.TaskSpec{
		Runnables: []*batchpb.Runnable{runnable},
	}

	// Configure compute resources
	if config.Resources != nil || config.BootDiskSizeGb > 0 {
		computeResource := &batchpb.ComputeResource{}

		if config.Resources != nil {
			computeResource.CpuMilli = config.Resources.CPUMillis
			computeResource.MemoryMib = config.Resources.MemoryMiB
		}

		// Convert boot disk size from GB to MiB (1 GB = 1024 MiB)
		if config.BootDiskSizeGb > 0 {
			computeResource.BootDiskMib = config.BootDiskSizeGb * 1024
		}

		taskSpec.ComputeResource = computeResource
	}

	// Set max run duration if specified
	if config.Resources != nil && config.Resources.MaxRunDurationSeconds > 0 {
		taskSpec.MaxRunDuration = durationpb.New(
			time.Duration(config.Resources.MaxRunDurationSeconds) * time.Second,
		)
	}

	// Set task retry count if specified
	if config.MaxRetryCount > 0 {
		taskSpec.MaxRetryCount = config.MaxRetryCount
	}

	// Determine task count from TaskGroup or default to 1
	taskCount := int64(1)
	if config.TaskGroup != nil && config.TaskGroup.TaskCount > 0 {
		taskCount = config.TaskGroup.TaskCount
	}

	// Create task group with configuration
	taskGroup := &batchpb.TaskGroup{
		TaskSpec:  taskSpec,
		TaskCount: taskCount,
	}

	// Configure task group options if provided
	if config.TaskGroup != nil {
		if config.TaskGroup.Parallelism > 0 {
			taskGroup.Parallelism = config.TaskGroup.Parallelism
		}

		if config.TaskGroup.SchedulingPolicy != "" {
			switch config.TaskGroup.SchedulingPolicy {
			case "IN_ORDER":
				taskGroup.SchedulingPolicy = batchpb.TaskGroup_IN_ORDER
			default:
				taskGroup.SchedulingPolicy = batchpb.TaskGroup_AS_SOON_AS_POSSIBLE
			}
		}

		if config.TaskGroup.TaskCountPerNode > 0 {
			taskGroup.TaskCountPerNode = config.TaskGroup.TaskCountPerNode
		}

		if config.TaskGroup.RequireHostsFile {
			taskGroup.RequireHostsFile = true
		}

		if config.TaskGroup.PermissiveSsh {
			taskGroup.PermissiveSsh = true
		}

		if config.TaskGroup.RunAsNonRoot {
			taskGroup.RunAsNonRoot = true
		}
	}

	// Build instance policy
	instancePolicy := &batchpb.AllocationPolicy_InstancePolicy{}

	// Set machine type if provided
	if config.MachineType != "" {
		instancePolicy.MachineType = config.MachineType
	}

	// Set provisioning model based on UseSpotVMs
	if config.UseSpotVMs {
		instancePolicy.ProvisioningModel = batchpb.AllocationPolicy_SPOT
	} else {
		instancePolicy.ProvisioningModel = batchpb.AllocationPolicy_STANDARD
	}

	// Set minimum CPU platform if provided
	if config.MinCpuPlatform != "" {
		instancePolicy.MinCpuPlatform = config.MinCpuPlatform
	}

	// Configure boot disk if size is specified
	if config.BootDiskSizeGb > 0 {
		instancePolicy.BootDisk = &batchpb.AllocationPolicy_Disk{
			Type:   "pd-standard",
			SizeGb: config.BootDiskSizeGb,
		}
	}

	// Add accelerators if specified
	if config.Accelerators != nil && config.Accelerators.Type != "" {
		instancePolicy.Accelerators = []*batchpb.AllocationPolicy_Accelerator{
			{
				Type:  config.Accelerators.Type,
				Count: config.Accelerators.Count,
			},
		}
	}

	// Create instance policy or template for allocation policy
	instancePolicyOrTemplate := &batchpb.AllocationPolicy_InstancePolicyOrTemplate{
		PolicyTemplate: &batchpb.AllocationPolicy_InstancePolicyOrTemplate_Policy{
			Policy: instancePolicy,
		},
		InstallGpuDrivers:   config.InstallGpuDrivers,
		InstallOpsAgent:     config.InstallOpsAgent,
		BlockProjectSshKeys: config.BlockProjectSshKeys,
	}

	// Build allocation policy
	allocationPolicy := &batchpb.AllocationPolicy{
		Instances: []*batchpb.AllocationPolicy_InstancePolicyOrTemplate{
			instancePolicyOrTemplate,
		},
	}

	// Configure service account if provided
	if config.ServiceAccount != "" {
		allocationPolicy.ServiceAccount = &batchpb.ServiceAccount{
			Email: config.ServiceAccount,
			Scopes: []string{
				"https://www.googleapis.com/auth/cloud-platform",
			},
		}
	}

	// Configure network if provided
	if config.NetworkName != "" || config.SubnetworkName != "" {
		networkInterface := &batchpb.AllocationPolicy_NetworkInterface{
			Network:        config.NetworkName,
			Subnetwork:     config.SubnetworkName,
			NoExternalIpAddress: config.BlockExternalIP,
		}

		allocationPolicy.Network = &batchpb.AllocationPolicy_NetworkPolicy{
			NetworkInterfaces: []*batchpb.AllocationPolicy_NetworkInterface{
				networkInterface,
			},
		}
	}

	// Set allowed locations if provided
	if len(config.AllowedLocations) > 0 {
		allocationPolicy.Location = &batchpb.AllocationPolicy_LocationPolicy{
			AllowedLocations: config.AllowedLocations,
		}
	}

	// Create job with all configuration
	job := &batchpb.Job{
		TaskGroups:       []*batchpb.TaskGroup{taskGroup},
		AllocationPolicy: allocationPolicy,
		Priority:         config.Priority,
		LogsPolicy: &batchpb.LogsPolicy{
			Destination: batchpb.LogsPolicy_CLOUD_LOGGING,
		},
	}

	// Set job labels if provided
	if len(config.JobLabels) > 0 {
		job.Labels = config.JobLabels
	}

	// Create job submission request
	req := &batchpb.CreateJobRequest{
		Parent:    parent,
		JobId:     config.JobID,
		Job:       job,
		RequestId: config.RequestID,
	}

	batchJob, err := p.client.CreateJob(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCP Batch job: %w", err)
	}

	// Map initial GCP state to Jennah status
	initialStatus := mapGCPStatusToJennah(batchJob.Status.State)

	return &batchpkg.JobResult{
		CloudResourcePath: batchJob.Name,
		InitialStatus:     initialStatus,
	}, nil
}

// GetJobStatus retrieves the current status of a GCP Batch job.
func (p *GCPBatchProvider) GetJobStatus(ctx context.Context, cloudResourcePath string) (batchpkg.JobStatus, error) {
	req := &batchpb.GetJobRequest{
		Name: cloudResourcePath,
	}

	job, err := p.client.GetJob(ctx, req)
	if err != nil {
		return batchpkg.JobStatusUnknown, fmt.Errorf("failed to get GCP Batch job: %w", err)
	}

	return mapGCPStatusToJennah(job.Status.State), nil
}

// CancelJob cancels a running GCP Batch job.
func (p *GCPBatchProvider) CancelJob(ctx context.Context, cloudResourcePath string) error {
	req := &batchpb.DeleteJobRequest{
		Name: cloudResourcePath,
	}

	op, err := p.client.DeleteJob(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to start delete operation: %w", err)
	}

	// Wait for deletion to complete
	if err := op.Wait(ctx); err != nil {
		return fmt.Errorf("delete operation failed: %w", err)
	}

	return nil
}

// ListJobs lists all jobs in the GCP project/region.
func (p *GCPBatchProvider) ListJobs(ctx context.Context) ([]string, error) {
	parent := fmt.Sprintf("projects/%s/locations/%s", p.projectID, p.region)

	req := &batchpb.ListJobsRequest{
		Parent: parent,
	}

	it := p.client.ListJobs(ctx, req)
	var jobPaths []string

	for {
		job, err := it.Next()
		if err != nil {
			// Iterator exhausted
			break
		}
		jobPaths = append(jobPaths, job.Name)
	}

	return jobPaths, nil
}

// Close closes the GCP Batch client.
func (p *GCPBatchProvider) Close() error {
	return p.client.Close()
}

// mapGCPStatusToJennah maps GCP Batch job states to Jennah status constants.
func mapGCPStatusToJennah(state batchpb.JobStatus_State) batchpkg.JobStatus {
	switch state {
	case batchpb.JobStatus_QUEUED:
		return batchpkg.JobStatusPending
	case batchpb.JobStatus_SCHEDULED:
		return batchpkg.JobStatusScheduled
	case batchpb.JobStatus_RUNNING:
		return batchpkg.JobStatusRunning
	case batchpb.JobStatus_SUCCEEDED:
		return batchpkg.JobStatusCompleted
	case batchpb.JobStatus_FAILED:
		return batchpkg.JobStatusFailed
	case batchpb.JobStatus_DELETION_IN_PROGRESS:
		return batchpkg.JobStatusCancelled
	case batchpb.JobStatus_CANCELLATION_IN_PROGRESS:
		return batchpkg.JobStatusCancelled
	case batchpb.JobStatus_CANCELLED:
		return batchpkg.JobStatusCancelled
	default:
		return batchpkg.JobStatusUnknown
	}
}
