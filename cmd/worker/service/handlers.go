package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	batchpb "cloud.google.com/go/batch/apiv1/batchpb"
	"connectrpc.com/connect"
	"github.com/google/uuid"

	jennahv1 "github.com/alphauslabs/jennah/gen/proto"
	"github.com/alphauslabs/jennah/internal/batch"
	"github.com/alphauslabs/jennah/internal/config"
	"github.com/alphauslabs/jennah/internal/database"
)

// dbJobToProto converts a database Job to a proto Job message.
func dbJobToProto(job *database.Job) *jennahv1.Job {
	p := &jennahv1.Job{
		JobId:      job.JobId,
		TenantId:   job.TenantId,
		ImageUri:   job.ImageUri,
		Status:     job.Status,
		CreatedAt:  job.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  job.UpdatedAt.Format(time.RFC3339),
		RetryCount: job.RetryCount,
		MaxRetries: job.MaxRetries,
		Commands:   job.Commands,
	}

	if job.ScheduledAt != nil {
		p.ScheduledAt = job.ScheduledAt.Format(time.RFC3339)
	}
	if job.StartedAt != nil {
		p.StartedAt = job.StartedAt.Format(time.RFC3339)
	}
	if job.CompletedAt != nil {
		p.CompletedAt = job.CompletedAt.Format(time.RFC3339)
	}
	if job.ErrorMessage != nil {
		p.ErrorMessage = *job.ErrorMessage
	}
	if job.GcpBatchJobName != nil {
		p.GcpBatchJobName = *job.GcpBatchJobName
	}
	if job.GcpBatchTaskGroup != nil {
		p.GcpBatchTaskGroup = *job.GcpBatchTaskGroup
	}
	if job.EnvVarsJson != nil {
		p.EnvVarsJson = *job.EnvVarsJson
	}
	if job.Name != nil {
		p.Name = *job.Name
	}
	if job.ResourceProfile != nil {
		p.ResourceProfile = *job.ResourceProfile
	}
	if job.MachineType != nil {
		p.MachineType = *job.MachineType
	}
	if job.BootDiskSizeGb != nil {
		p.BootDiskSizeGb = *job.BootDiskSizeGb
	}
	if job.UseSpotVms != nil {
		p.UseSpotVms = *job.UseSpotVms
	}
	if job.ServiceAccount != nil {
		p.ServiceAccount = *job.ServiceAccount
	}

	return p
}

// SubmitJob handles a job submission request.
func (s *WorkerService) SubmitJob(
	ctx context.Context,
	req *connect.Request[jennahv1.SubmitJobRequest],
) (*connect.Response[jennahv1.SubmitJobResponse], error) {
	tenantID := req.Header().Get("X-Tenant-Id")
	log.Printf("Received SubmitJob request for tenant: %s", tenantID)

	if tenantID == "" {
		log.Printf("Error: X-Tenant-Id header is missing")
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("X-Tenant-Id header is required"))
	}

	if req.Msg.ImageUri == "" {
		log.Printf("Error: image_uri is empty")
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("image_uri is required"))
	}

	// Use canonical job ID from gateway when provided; otherwise generate one
	// for backward compatibility (e.g., direct worker calls).
	internalJobID := req.Msg.JobId
	if internalJobID == "" {
		internalJobID = uuid.New().String()
		log.Printf("Generated internal job ID (fallback): %s", internalJobID)
	} else {
		log.Printf("Using gateway-provided internal job ID: %s", internalJobID)
	}

	// Generate cloud provider-compatible job ID.
	// Use user-provided name if available, otherwise fall back to UUID-based ID.
	providerJobID := generateProviderJobID(req.Msg.Name, internalJobID)
	log.Printf("Generated provider job ID: %s", providerJobID)

	// Serialize environment variables to JSON for storage.
	var envVarsJson *string
	if len(req.Msg.EnvVars) > 0 {
		envBytes, err := json.Marshal(req.Msg.EnvVars)
		if err != nil {
			log.Printf("Error serializing env vars: %v", err)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to serialize env vars: %w", err))
		}
		s := string(envBytes)
		envVarsJson = &s
	}

	// Insert job record with PENDING status and advanced config.
	now := time.Now().UTC()
	leaseUntil := now.Add(s.leaseTTL)
	err := s.dbClient.InsertJobFull(ctx, &database.Job{
		TenantId:          tenantID,
		JobId:             internalJobID,
		Status:            database.JobStatusPending,
		ImageUri:          req.Msg.ImageUri,
		Commands:          req.Msg.Commands,
		RetryCount:        0,
		MaxRetries:        3,
		EnvVarsJson:       envVarsJson,
		Name:              ptrStringOrNil(req.Msg.Name),
		ResourceProfile:   ptrStringOrNil(req.Msg.ResourceProfile),
		MachineType:       ptrStringOrNil(req.Msg.MachineType),
		BootDiskSizeGb:    ptrInt64OrNil(req.Msg.BootDiskSizeGb),
		UseSpotVms:        ptrBoolOrNil(req.Msg.UseSpotVms),
		ServiceAccount:    ptrStringOrNil(req.Msg.ServiceAccount),
		OwnerWorkerId:     &s.workerID,
		PreferredWorkerId: &s.workerID,
		LeaseExpiresAt:    &leaseUntil,
		LastHeartbeatAt:   &now,
	})
	if err != nil {
		log.Printf("Error inserting job to database: %v", err)
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to create job record: %w", err),
		)
	}
	log.Printf("Job %s saved to database with PENDING status", internalJobID)

	// Submit job to cloud batch provider.
	// Resolve resource requirements: machine type, named preset merged with any per-field override.
	var resourceOverride *config.ResourceOverride
	if o := req.Msg.ResourceOverride; o != nil {
		resourceOverride = &config.ResourceOverride{
			CPUMillis:             o.CpuMillis,
			MemoryMiB:             o.MemoryMib,
			MaxRunDurationSeconds: o.MaxRunDurationSeconds,
		}
	}

	// Extract machine type as string for resource resolution
	machineType := ""
	if req.Msg.MachineType != "" {
		machineType = req.Msg.MachineType
	}

	// Build batch job configuration with all available fields
	batchJobConfig := batch.JobConfig{
		JobID:               providerJobID,
		ImageURI:            req.Msg.ImageUri,
		EnvVars:             req.Msg.EnvVars,
		Resources:           s.jobConfig.ResolveResources(machineType, req.Msg.ResourceProfile, resourceOverride),
		MachineType:         req.Msg.MachineType,
		BootDiskSizeGb:      req.Msg.BootDiskSizeGb,
		UseSpotVMs:          req.Msg.UseSpotVms,
		ServiceAccount:      req.Msg.ServiceAccount,
		Commands:            req.Msg.Commands,
		ContainerEntrypoint: "", // Not exposed in proto yet
		RequestID:           internalJobID, // Use internal job ID as idempotency key
	}

	// Configure task group if needed (currently default: 1 task)
	// Future: allow SubmitJobRequest to specify task groups
	batchJobConfig.TaskGroup = &batch.TaskGroupConfig{
		TaskCount:        1,
		Parallelism:      0,
		SchedulingPolicy: "AS_SOON_AS_POSSIBLE",
		TaskCountPerNode: 0,
		RequireHostsFile: false,
		PermissiveSsh:    false,
		RunAsNonRoot:     false,
	}

	jobResult, err := s.batchProvider.SubmitJob(ctx, batchJobConfig)
	if err != nil {
		log.Printf("Error submitting job to batch provider: %v", err)
		failErr := s.dbClient.FailJob(ctx, tenantID, internalJobID, err.Error())
		if failErr != nil {
			log.Printf("Error updating job status to FAILED: %v", failErr)
		}
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to submit batch job: %w", err),
		)
	}
	log.Printf("Batch job created: %s", jobResult.CloudResourcePath)

	// Update job status and GCP Batch job name based on provider's initial status.
	statusToSet := string(jobResult.InitialStatus)
	if statusToSet == "" || statusToSet == string(batch.JobStatusUnknown) {
		statusToSet = database.JobStatusRunning
	}

	err = s.dbClient.UpdateJobStatusAndGcpBatchJobName(ctx, tenantID, internalJobID, statusToSet, jobResult.CloudResourcePath)
	if err != nil {
		log.Printf("Error updating job status to %s: %v", statusToSet, err)
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to update job status: %w", err),
		)
	}
	log.Printf("Job %s status updated to %s with GCP Batch job name: %s", internalJobID, statusToSet, jobResult.CloudResourcePath)

	// Start background polling goroutine to track job status.
	s.startJobPoller(ctx, tenantID, internalJobID, jobResult.CloudResourcePath, statusToSet)

	response := connect.NewResponse(&jennahv1.SubmitJobResponse{
		JobId:  internalJobID,
		Status: statusToSet,
	})

	log.Printf("Successfully submitted job %s for tenant %s", internalJobID, tenantID)
	return response, nil
}

// ListJobs returns all jobs for the tenant.
func (s *WorkerService) ListJobs(
	ctx context.Context,
	req *connect.Request[jennahv1.ListJobsRequest],
) (*connect.Response[jennahv1.ListJobsResponse], error) {
	tenantID := req.Header().Get("X-Tenant-Id")
	log.Printf("Received ListJobs request for tenant: %s", tenantID)

	if tenantID == "" {
		log.Printf("Error: X-Tenant-Id header is missing")
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("X-Tenant-Id header is required"))
	}

	jobs, err := s.dbClient.ListJobs(ctx, tenantID)
	if err != nil {
		log.Printf("Error listing jobs from database: %v", err)
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to list jobs: %w", err),
		)
	}
	log.Printf("Retrieved %d jobs for tenant %s", len(jobs), tenantID)

	protoJobs := make([]*jennahv1.Job, 0, len(jobs))
	for _, job := range jobs {
		protoJobs = append(protoJobs, dbJobToProto(job))
	}

	response := connect.NewResponse(&jennahv1.ListJobsResponse{
		Jobs: protoJobs,
	})

	log.Printf("Successfully listed %d jobs for tenant %s", len(protoJobs), tenantID)
	return response, nil
}

// CancelJob cancels a running or pending job.
func (s *WorkerService) CancelJob(
	ctx context.Context,
	req *connect.Request[jennahv1.CancelJobRequest],
) (*connect.Response[jennahv1.CancelJobResponse], error) {
	tenantID := req.Header().Get("X-Tenant-Id")
	jobID := req.Msg.JobId

	if tenantID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("X-Tenant-Id header is required"))
	}

	if jobID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("job_id is required"))
	}

	log.Printf("Received CancelJob request for job %s (tenant: %s)", jobID, tenantID)

	// Retrieve job from database.
	job, err := s.dbClient.GetJob(ctx, tenantID, jobID)
	if err != nil {
		log.Printf("Error retrieving job: %v", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("job not found: %w", err))
	}

	// Check if job can be cancelled (only PENDING, SCHEDULED, RUNNING).
	if !isCancellableStatus(job.Status) {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			fmt.Errorf("cannot cancel job with status %s; only PENDING, SCHEDULED, or RUNNING jobs can be cancelled", job.Status),
		)
	}

	// Cancel job in GCP Batch.
	if job.GcpBatchJobName != nil {
		cancelReq := &batchpb.CancelJobRequest{
			Name: *job.GcpBatchJobName,
		}
		op, err := s.gcpBatchClient.CancelJob(ctx, cancelReq)
		if err != nil {
			log.Printf("Error cancelling job in GCP Batch: %v", err)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to cancel job in GCP Batch: %w", err))
		}

		_, err = op.Poll(ctx)
		if err != nil {
			log.Printf("Error polling cancel operation: %v", err)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to cancel operation: %w", err))
		}
		log.Printf("Job %s cancelled in GCP Batch", jobID)
	}

	// Update job status to CANCELLED in database.
	err = s.dbClient.UpdateJobStatus(ctx, tenantID, jobID, database.JobStatusCancelled)
	if err != nil {
		log.Printf("Error updating job status to CANCELLED: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update job status: %w", err))
	}

	// Record state transition.
	transitionID := uuid.New().String()
	reason := "Job cancelled by user request"
	err = s.dbClient.RecordStateTransition(ctx, tenantID, jobID, transitionID, &job.Status, database.JobStatusCancelled, &reason)
	if err != nil {
		log.Printf("Error recording state transition: %v", err)
	}

	// Stop the poller for this job.
	s.stopPollerForJob(tenantID, jobID)

	response := connect.NewResponse(&jennahv1.CancelJobResponse{
		JobId:  jobID,
		Status: database.JobStatusCancelled,
	})

	log.Printf("Successfully cancelled job %s", jobID)
	return response, nil
}

// DeleteJob deletes a job from GCP Batch and the database.
func (s *WorkerService) DeleteJob(
	ctx context.Context,
	req *connect.Request[jennahv1.DeleteJobRequest],
) (*connect.Response[jennahv1.DeleteJobResponse], error) {
	tenantID := req.Header().Get("X-Tenant-Id")
	jobID := req.Msg.JobId

	if tenantID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("X-Tenant-Id header is required"))
	}

	if jobID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("job_id is required"))
	}

	log.Printf("Received DeleteJob request for job %s (tenant: %s)", jobID, tenantID)

	// Retrieve job from database.
	job, err := s.dbClient.GetJob(ctx, tenantID, jobID)
	if err != nil {
		log.Printf("Error retrieving job: %v", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("job not found: %w", err))
	}

	// Delete job from GCP Batch.
	if job.GcpBatchJobName != nil {
		deleteReq := &batchpb.DeleteJobRequest{
			Name: *job.GcpBatchJobName,
		}
		op, err := s.gcpBatchClient.DeleteJob(ctx, deleteReq)
		if err != nil {
			log.Printf("Error deleting job from GCP Batch: %v", err)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete job from GCP Batch: %w", err))
		}

		err = op.Poll(ctx)
		if err != nil {
			log.Printf("Error polling delete operation: %v", err)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete operation: %w", err))
		}
		log.Printf("Job %s deleted from GCP Batch", jobID)
	}

	// Delete job from database (cascades to JobStateTransitions).
	err = s.dbClient.DeleteJob(ctx, tenantID, jobID)
	if err != nil {
		log.Printf("Error deleting job from database: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete job: %w", err))
	}
	log.Printf("Job %s deleted from database", jobID)

	// Stop the poller for this job.
	s.stopPollerForJob(tenantID, jobID)

	response := connect.NewResponse(&jennahv1.DeleteJobResponse{
		JobId:   jobID,
		Message: "Job successfully deleted",
	})

	log.Printf("Successfully deleted job %s", jobID)
	return response, nil
}

// GetJob returns full details for a single job.
func (s *WorkerService) GetJob(
	ctx context.Context,
	req *connect.Request[jennahv1.GetJobRequest],
) (*connect.Response[jennahv1.GetJobResponse], error) {
	tenantID := req.Header().Get("X-Tenant-Id")
	jobID := req.Msg.JobId

	if tenantID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("X-Tenant-Id header is required"))
	}

	if jobID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("job_id is required"))
	}

	log.Printf("Received GetJob request for job %s (tenant: %s)", jobID, tenantID)

	job, err := s.dbClient.GetJob(ctx, tenantID, jobID)
	if err != nil {
		log.Printf("Error retrieving job: %v", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("job not found: %w", err))
	}

	response := connect.NewResponse(&jennahv1.GetJobResponse{
		Job: dbJobToProto(job),
	})

	log.Printf("Successfully retrieved job %s for tenant %s", jobID, tenantID)
	return response, nil
}

// generateProviderJobID creates a GCP Batch-compatible job ID.
// If a user-provided name is given, it is sanitized (lowercased, invalid chars
// replaced with hyphens, trimmed to fit) and a short UUID suffix is appended to
// guarantee uniqueness. If name is empty, falls back to "jennah-{uuid[:8]}".
//
// GCP Batch constraints: ^[a-z]([a-z0-9-]{0,62}[a-z0-9])?$ (max 64 chars).
func generateProviderJobID(name, jobID string) string {
	shortID := strings.ReplaceAll(jobID, "-", "")[:8]

	if name == "" {
		return "jennah-" + strings.ToLower(shortID)
	}

	// Sanitize: lowercase, replace non-alphanumeric with hyphens, collapse runs.
	sanitized := strings.ToLower(name)
	var b strings.Builder
	prevHyphen := false
	for _, r := range sanitized {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	sanitized = strings.TrimRight(b.String(), "-")

	// Ensure it starts with a letter.
	if len(sanitized) == 0 || sanitized[0] < 'a' || sanitized[0] > 'z' {
		sanitized = "j" + sanitized
	}

	// Format: "{sanitized}-{shortID}", max 64 chars total.
	suffix := "-" + shortID // 9 chars
	maxNameLen := 64 - len(suffix)
	if len(sanitized) > maxNameLen {
		sanitized = strings.TrimRight(sanitized[:maxNameLen], "-")
	}

	return sanitized + suffix
}

// ptrStringOrNil returns a pointer to s if non-empty, nil otherwise.
func ptrStringOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ptrInt64OrNil returns a pointer to v if non-zero, nil otherwise.
func ptrInt64OrNil(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

// ptrBoolOrNil returns a pointer to v if true, nil otherwise.
// For Spanner nullable BOOL columns, false maps to nil (unset).
func ptrBoolOrNil(v bool) *bool {
	if !v {
		return nil
	}
	return &v
}
