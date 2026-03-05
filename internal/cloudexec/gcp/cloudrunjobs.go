package gcp

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	run "cloud.google.com/go/run/apiv2"
	runpb "cloud.google.com/go/run/apiv2/runpb"
	api "google.golang.org/genproto/googleapis/api"
	"google.golang.org/protobuf/types/known/durationpb"

	batchpkg "github.com/alphauslabs/jennah/internal/cloudexec"
)

func init() {
	batchpkg.RegisterGCPCloudRunProvider(NewGCPCloudRunProvider)
}

// GCPCloudRunProvider implements the batch.Provider interface for GCP Cloud Run Jobs.
// Cloud Run Jobs is used for MEDIUM jobs: ≤4000 mCPU, ≤8192 MiB, ≤3600 s.
//
// Each job is created as a Cloud Run Job resource and immediately executed.
// The job runs the configured container image with the specified resources.
type GCPCloudRunProvider struct {
	jobsClient      *run.JobsClient
	executionClient *run.ExecutionsClient
	projectID       string
	region          string
}

// ServiceType returns the service type identifier for Cloud Run Jobs.
func (p *GCPCloudRunProvider) ServiceType() string {
	return batchpkg.ServiceTypeCloudRunJob
}

// NewGCPCloudRunProvider creates a new GCP Cloud Run Jobs provider.
func NewGCPCloudRunProvider(ctx context.Context, config batchpkg.ProviderConfig) (batchpkg.Provider, error) {
	if config.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required for GCP Cloud Run Jobs provider")
	}
	if config.Region == "" {
		return nil, fmt.Errorf("region is required for GCP Cloud Run Jobs provider")
	}

	jobsClient, err := run.NewJobsClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloud Run Jobs client: %w", err)
	}

	executionClient, err := run.NewExecutionsClient(ctx)
	if err != nil {
		jobsClient.Close()
		return nil, fmt.Errorf("failed to create Cloud Run Executions client: %w", err)
	}

	return &GCPCloudRunProvider{
		jobsClient:      jobsClient,
		executionClient: executionClient,
		projectID:       config.ProjectID,
		region:          config.Region,
	}, nil
}

// SubmitJob creates a Cloud Run Job and runs it immediately.
//
// Cloud Run Jobs v2 API flow:
//  1. CreateJob — defines the job (container, resources, env)
//  2. RunJob   — creates an execution of the job
func (p *GCPCloudRunProvider) SubmitJob(ctx context.Context, config batchpkg.JobConfig) (*batchpkg.JobResult, error) {
	parent := fmt.Sprintf("projects/%s/locations/%s", p.projectID, p.region)

	// Build container definition.
	container := &runpb.Container{
		Image: config.ImageURI,
	}

	if len(config.Commands) > 0 {
		container.Command = config.Commands
	}

	if config.ContainerEntrypoint != "" {
		container.Command = []string{config.ContainerEntrypoint}
		if len(config.Commands) > 0 {
			container.Args = config.Commands
		}
	}

	// Set environment variables.
	for k, v := range config.EnvVars {
		container.Env = append(container.Env, &runpb.EnvVar{
			Name:   k,
			Values: &runpb.EnvVar_Value{Value: v},
		})
	}

	// Set resource limits.
	container.Resources = &runpb.ResourceRequirements{
		Limits: make(map[string]string),
	}
	if config.Resources != nil {
		if config.Resources.CPUMillis > 0 {
			// Cloud Run expects CPU as a string like "1" or "2" (whole cores)
			// or "1000m" for millicores.
			container.Resources.Limits["cpu"] = fmt.Sprintf("%dm", config.Resources.CPUMillis)
		}
		if config.Resources.MemoryMiB > 0 {
			container.Resources.Limits["memory"] = fmt.Sprintf("%dMi", config.Resources.MemoryMiB)
		}
	}

	// Build task template with timeout.
	taskTemplate := &runpb.TaskTemplate{
		Containers: []*runpb.Container{container},
	}

	if config.Resources != nil && config.Resources.MaxRunDurationSeconds > 0 {
		taskTemplate.Timeout = durationpb.New(
			time.Duration(config.Resources.MaxRunDurationSeconds) * time.Second,
		)
	}

	if config.MaxRetryCount > 0 {
		taskTemplate.Retries = &runpb.TaskTemplate_MaxRetries{
			MaxRetries: config.MaxRetryCount,
		}
	}

	// Set service account if provided.
	if config.ServiceAccount != "" {
		taskTemplate.ServiceAccount = config.ServiceAccount
	}

	// Configure execution template.
	executionTemplate := &runpb.ExecutionTemplate{
		Template: taskTemplate,
	}

	// Set task count and parallelism from task group config.
	if config.TaskGroup != nil {
		if config.TaskGroup.TaskCount > 0 {
			executionTemplate.TaskCount = int32(config.TaskGroup.TaskCount)
		}
		if config.TaskGroup.Parallelism > 0 {
			executionTemplate.Parallelism = int32(config.TaskGroup.Parallelism)
		}
	}

	// Step 1: Create the Cloud Run Job.
	createReq := &runpb.CreateJobRequest{
		Parent: parent,
		JobId:  config.JobID,
		Job: &runpb.Job{
			Template: executionTemplate,
			LaunchStage: api.LaunchStage_GA,
		},
	}

	createOp, err := p.jobsClient.CreateJob(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloud Run job: %w", err)
	}

	job, err := createOp.Wait(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed waiting for Cloud Run job creation: %w", err)
	}

	log.Printf("Cloud Run job created: %s", job.GetName())

	// Step 2: Run the job (create an execution).
	runReq := &runpb.RunJobRequest{
		Name: job.GetName(),
	}

	runOp, err := p.jobsClient.RunJob(ctx, runReq)
	if err != nil {
		return nil, fmt.Errorf("failed to run Cloud Run job: %w", err)
	}

	// Don't wait for execution to complete — just get the operation metadata.
	// The execution will be tracked asynchronously via polling.
	execution, err := runOp.Metadata()
	if err != nil {
		// Even if metadata extraction fails, the job was created and started.
		log.Printf("Warning: failed to get execution metadata: %v", err)
		return &batchpkg.JobResult{
			CloudResourcePath: job.GetName(),
			InitialStatus:     batchpkg.JobStatusRunning,
		}, nil
	}

	log.Printf("Cloud Run job execution started: %s", execution)

	return &batchpkg.JobResult{
		CloudResourcePath: job.GetName(),
		InitialStatus:     batchpkg.JobStatusRunning,
	}, nil
}

// GetJobStatus retrieves the current status of a Cloud Run Job by inspecting
// its latest execution.
func (p *GCPCloudRunProvider) GetJobStatus(ctx context.Context, cloudResourcePath string) (batchpkg.JobStatus, error) {
	// List executions for the job to find the latest one.
	it := p.executionClient.ListExecutions(ctx, &runpb.ListExecutionsRequest{
		Parent: cloudResourcePath,
	})

	// Get the first (most recent) execution.
	execution, err := it.Next()
	if err != nil {
		return batchpkg.JobStatusUnknown, fmt.Errorf("failed to list Cloud Run executions: %w", err)
	}

	return mapCloudRunStatus(execution), nil
}

// CancelJob cancels a running Cloud Run Job execution.
func (p *GCPCloudRunProvider) CancelJob(ctx context.Context, cloudResourcePath string) error {
	// List executions to find the running one.
	it := p.executionClient.ListExecutions(ctx, &runpb.ListExecutionsRequest{
		Parent: cloudResourcePath,
	})

	execution, err := it.Next()
	if err != nil {
		return fmt.Errorf("no executions found for Cloud Run job: %w", err)
	}

	// Cancel the execution.
	cancelOp, err := p.executionClient.CancelExecution(ctx, &runpb.CancelExecutionRequest{
		Name: execution.GetName(),
	})
	if err != nil {
		return fmt.Errorf("failed to cancel Cloud Run execution: %w", err)
	}

	_, err = cancelOp.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed waiting for Cloud Run execution cancellation: %w", err)
	}

	log.Printf("Cloud Run execution cancelled: %s", execution.GetName())
	return nil
}

// DeleteJob deletes a Cloud Run Job resource.
func (p *GCPCloudRunProvider) DeleteJob(ctx context.Context, cloudResourcePath string) error {
	deleteOp, err := p.jobsClient.DeleteJob(ctx, &runpb.DeleteJobRequest{
		Name: cloudResourcePath,
	})
	if err != nil {
		return fmt.Errorf("failed to delete Cloud Run job: %w", err)
	}

	_, err = deleteOp.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed waiting for Cloud Run job deletion: %w", err)
	}

	log.Printf("Cloud Run job deleted: %s", cloudResourcePath)
	return nil
}

// ListJobs lists all Cloud Run Jobs in the configured project/region.
func (p *GCPCloudRunProvider) ListJobs(ctx context.Context) ([]string, error) {
	parent := fmt.Sprintf("projects/%s/locations/%s", p.projectID, p.region)

	it := p.jobsClient.ListJobs(ctx, &runpb.ListJobsRequest{
		Parent: parent,
	})

	var jobNames []string
	for {
		job, err := it.Next()
		if err != nil {
			break
		}
		// Only include Jennah-managed jobs (those starting with "jennah-").
		name := job.GetName()
		parts := strings.Split(name, "/")
		if len(parts) > 0 && strings.HasPrefix(parts[len(parts)-1], "jennah-") {
			jobNames = append(jobNames, name)
		}
	}

	return jobNames, nil
}

// Close closes the Cloud Run Jobs and Executions clients.
func (p *GCPCloudRunProvider) Close() error {
	// Close jobsClient
	if err := p.jobsClient.Close(); err != nil {
		log.Printf("Error closing jobsClient: %v", err)
	}

	// Close executionClient
	if err := p.executionClient.Close(); err != nil {
		log.Printf("Error closing executionClient: %v", err)
	}

	return nil
}

// mapCloudRunStatus maps a Cloud Run Execution to a Jennah JobStatus.
//
// Cloud Run execution conditions:
//
// The Cloud Run v2 API uses the "Completed" condition type for terminal states:
//   - CONDITION_SUCCEEDED → job completed successfully
//   - CONDITION_FAILED   → job failed (container error, timeout, etc.)
//
// The "Cancelled" condition type is set when an execution is cancelled.
func mapCloudRunStatus(execution *runpb.Execution) batchpkg.JobStatus {
	if execution == nil {
		return batchpkg.JobStatusUnknown
	}

	// Check terminal conditions first.
	// Cloud Run v2 signals both success and failure via the "Completed" condition
	// with different states (CONDITION_SUCCEEDED vs CONDITION_FAILED).
	for _, condition := range execution.GetConditions() {
		switch condition.GetType() {
		case "Completed":
			switch condition.GetState() {
			case runpb.Condition_CONDITION_SUCCEEDED:
				return batchpkg.JobStatusCompleted
			case runpb.Condition_CONDITION_FAILED:
				return batchpkg.JobStatusFailed
			}
		case "Cancelled":
			if condition.GetState() == runpb.Condition_CONDITION_SUCCEEDED {
				return batchpkg.JobStatusCancelled
			}
		}
	}

	// If we have running tasks, the job is running.
	if execution.GetRunningCount() > 0 {
		return batchpkg.JobStatusRunning
	}

	// Fallback: detect failure from task counts when conditions aren't populated yet.
	if execution.GetFailedCount() > 0 && execution.GetRunningCount() == 0 {
		return batchpkg.JobStatusFailed
	}

	// If no tasks have started yet, it's pending.
	if execution.GetRunningCount() == 0 && execution.GetSucceededCount() == 0 && execution.GetFailedCount() == 0 {
		return batchpkg.JobStatusPending
	}

	return batchpkg.JobStatusRunning
}
