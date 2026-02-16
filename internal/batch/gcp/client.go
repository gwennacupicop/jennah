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

	// Create runnable with container configuration
	runnable := &batchpb.Runnable{
		Executable: &batchpb.Runnable_Container_{
			Container: &batchpb.Runnable_Container{
				ImageUri: config.ImageURI,
			},
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

	// Add resource requirements if specified
	if config.Resources != nil {
		taskSpec.ComputeResource = &batchpb.ComputeResource{
			CpuMilli:  config.Resources.CPUMillis,
			MemoryMib: config.Resources.MemoryMiB,
		}
		if config.Resources.MaxRunDurationSeconds > 0 {
			taskSpec.MaxRunDuration = durationpb.New(
				time.Duration(config.Resources.MaxRunDurationSeconds) * time.Second,
			)
		}
	}

	// Create job with task group
	job := &batchpb.Job{
		TaskGroups: []*batchpb.TaskGroup{
			{
				TaskSpec:  taskSpec,
				TaskCount: 1,
			},
		},
		LogsPolicy: &batchpb.LogsPolicy{
			Destination: batchpb.LogsPolicy_CLOUD_LOGGING,
		},
	}

	// Submit job to GCP Batch
	req := &batchpb.CreateJobRequest{
		Parent: parent,
		JobId:  config.JobID,
		Job:    job,
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
	default:
		return batchpkg.JobStatusUnknown
	}
}
