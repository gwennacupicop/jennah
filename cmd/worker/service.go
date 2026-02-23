package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	gcpbatch "cloud.google.com/go/batch/apiv1"
	batchpb "cloud.google.com/go/batch/apiv1/batchpb"
	"connectrpc.com/connect"
	"github.com/google/uuid"

	jennahv1 "github.com/alphauslabs/jennah/gen/proto"
	"github.com/alphauslabs/jennah/gen/proto/jennahv1connect"
	"github.com/alphauslabs/jennah/internal/batch"
	"github.com/alphauslabs/jennah/internal/config"
	"github.com/alphauslabs/jennah/internal/database"
)

// JobPoller manages polling of a single job's status from GCP Batch.
type JobPoller struct {
	tenantID          string
	jobID             string
	gcpResourcePath   string
	currentStatus     string
	batchProvider     batch.Provider
	dbClient          *database.Client
	ticker            *time.Ticker
	done              chan bool
	pollingInterval   time.Duration
	maxFailedAttempts int
	failedAttempts    int
}

type WorkerServer struct {
	jennahv1connect.UnimplementedDeploymentServiceHandler
	dbClient       *database.Client
	batchProvider  batch.Provider
	jobConfig      *config.JobConfigFile
	pollers        map[string]*JobPoller // Key: "tenantID/jobID"
	pollersMutex   sync.Mutex
	gcpBatchClient *gcpbatch.Client
}

func (s *WorkerServer) SubmitJob(
	ctx context.Context,
	req *connect.Request[jennahv1.SubmitJobRequest],
) (*connect.Response[jennahv1.SubmitJobResponse], error) {
	tenantId := req.Header().Get("X-Tenant-Id")
	log.Printf("Received SubmitJob request for tenant: %s", tenantId)

	if tenantId == "" {
		log.Printf("Error: X-Tenant-Id header is missing")
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("X-Tenant-Id header is required"))
	}

	if req.Msg.ImageUri == "" {
		log.Printf("Error: image_uri is empty")
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("image_uri is required"))
	}

	// Generate internal UUID for Spanner primary key
	internalJobID := uuid.New().String()
	log.Printf("Generated internal job ID: %s", internalJobID)

	// Generate cloud provider-compatible job ID (lowercase, starts with letter, no underscores)
	providerJobID := generateProviderJobID(internalJobID)
	log.Printf("Generated provider job ID: %s", providerJobID)

	// Insert job record with PENDING status
	err := s.dbClient.InsertJob(ctx, tenantId, internalJobID, req.Msg.ImageUri, []string{})
	if err != nil {
		log.Printf("Error inserting job to database: %v", err)
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to create job record: %w", err),
		)
	}
	log.Printf("Job %s saved to database with PENDING status", internalJobID)

	// Submit job to cloud batch provider.
	// Resolve resource requirements: named preset merged with any per-field override.
	var resourceOverride *config.ResourceOverride
	if o := req.Msg.ResourceOverride; o != nil {
		resourceOverride = &config.ResourceOverride{
			CPUMillis:             o.CpuMillis,
			MemoryMiB:             o.MemoryMib,
			MaxRunDurationSeconds: o.MaxRunDurationSeconds,
		}
	}

	batchJobConfig := batch.JobConfig{
		JobID:     providerJobID,
		ImageURI:  req.Msg.ImageUri,
		EnvVars:   req.Msg.EnvVars,
		Resources: s.jobConfig.ResolveResources(req.Msg.ResourceProfile, resourceOverride),
	}

	jobResult, err := s.batchProvider.SubmitJob(ctx, batchJobConfig)
	if err != nil {
		log.Printf("Error submitting job to batch provider: %v", err)
		failErr := s.dbClient.FailJob(ctx, tenantId, internalJobID, err.Error())
		if failErr != nil {
			log.Printf("Error updating job status to FAILED: %v", failErr)
		}
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to submit batch job: %w", err),
		)
	}
	log.Printf("Batch job created: %s", jobResult.CloudResourcePath)

	// Update job status and GCP Batch job name based on provider's initial status
	statusToSet := string(jobResult.InitialStatus)
	if statusToSet == "" || statusToSet == string(batch.JobStatusUnknown) {
		statusToSet = database.JobStatusRunning
	}

	err = s.dbClient.UpdateJobStatusAndGcpBatchJobName(ctx, tenantId, internalJobID, statusToSet, jobResult.CloudResourcePath)
	if err != nil {
		log.Printf("Error updating job status to %s: %v", statusToSet, err)
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to update job status: %w", err),
		)
	}
	log.Printf("Job %s status updated to %s with GCP Batch job name: %s", internalJobID, statusToSet, jobResult.CloudResourcePath)

	// Start background polling goroutine to track job status
	s.startJobPoller(ctx, tenantId, internalJobID, jobResult.CloudResourcePath, statusToSet)

	response := connect.NewResponse(&jennahv1.SubmitJobResponse{
		JobId:  internalJobID, // Return internal UUID to client
		Status: statusToSet,
	})

	log.Printf("Successfully submitted job %s for tenant %s", internalJobID, tenantId)
	return response, nil
}

// generateProviderJobID creates a provider-compatible job ID from UUID.
// Most cloud providers require lowercase, starting with letter, no underscores.
func generateProviderJobID(uuid string) string {
	// Use "jennah-" prefix + first 8 chars of UUID (no hyphens)
	cleanUUID := strings.ReplaceAll(uuid, "-", "")
	return "jennah-" + strings.ToLower(cleanUUID[:8])
}

func (s *WorkerServer) ListJobs(
	ctx context.Context,
	req *connect.Request[jennahv1.ListJobsRequest],
) (*connect.Response[jennahv1.ListJobsResponse], error) {
	tenantId := req.Header().Get("X-Tenant-Id")
	log.Printf("Received ListJobs request for tenant: %s", tenantId)

	if tenantId == "" {
		log.Printf("Error: X-Tenant-Id header is missing")
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("X-Tenant-Id header is required"))
	}

	jobs, err := s.dbClient.ListJobs(ctx, tenantId)
	if err != nil {
		log.Printf("Error listing jobs from database: %v", err)
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to list jobs: %w", err),
		)
	}
	log.Printf("Retrieved %d jobs for tenant %s", len(jobs), tenantId)

	protoJobs := make([]*jennahv1.Job, 0, len(jobs))
	for _, job := range jobs {
		protoJob := &jennahv1.Job{
			JobId:     job.JobId,
			TenantId:  job.TenantId,
			ImageUri:  job.ImageUri,
			Status:    job.Status,
			CreatedAt: job.CreatedAt.Format(time.RFC3339),
		}
		protoJobs = append(protoJobs, protoJob)
	}

	response := connect.NewResponse(&jennahv1.ListJobsResponse{
		Jobs: protoJobs,
	})

	log.Printf("Successfully listed %d jobs for tenant %s", len(protoJobs), tenantId)
	return response, nil
}

func (s *WorkerServer) CancelJob(
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

	// Retrieve job from database
	job, err := s.dbClient.GetJob(ctx, tenantID, jobID)
	if err != nil {
		log.Printf("Error retrieving job: %v", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("job not found: %w", err))
	}

	// Check if job can be cancelled (only PENDING, SCHEDULED, RUNNING)
	if !isCancellableStatus(job.Status) {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			fmt.Errorf("cannot cancel job with status %s; only PENDING, SCHEDULED, or RUNNING jobs can be cancelled", job.Status),
		)
	}

	// Cancel job in GCP Batch
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

	// Update job status to CANCELLED in database
	err = s.dbClient.UpdateJobStatus(ctx, tenantID, jobID, database.JobStatusCancelled)
	if err != nil {
		log.Printf("Error updating job status to CANCELLED: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update job status: %w", err))
	}

	// Record state transition
	transitionID := uuid.New().String()
	reason := "Job cancelled by user request"
	err = s.dbClient.RecordStateTransition(ctx, tenantID, jobID, transitionID, &job.Status, database.JobStatusCancelled, &reason)
	if err != nil {
		log.Printf("Error recording state transition: %v", err)
	}

	// Stop the poller for this job
	s.stopPollerForJob(tenantID, jobID)

	response := connect.NewResponse(&jennahv1.CancelJobResponse{
		JobId:  jobID,
		Status: database.JobStatusCancelled,
	})

	log.Printf("Successfully cancelled job %s", jobID)
	return response, nil
}

func (s *WorkerServer) DeleteJob(
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

	// Retrieve job from database
	job, err := s.dbClient.GetJob(ctx, tenantID, jobID)
	if err != nil {
		log.Printf("Error retrieving job: %v", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("job not found: %w", err))
	}

	// Delete job from GCP Batch
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

	// Delete job from database (cascades to JobStateTransitions)
	err = s.dbClient.DeleteJob(ctx, tenantID, jobID)
	if err != nil {
		log.Printf("Error deleting job from database: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete job: %w", err))
	}
	log.Printf("Job %s deleted from database", jobID)

	// Stop the poller for this job
	s.stopPollerForJob(tenantID, jobID)

	response := connect.NewResponse(&jennahv1.DeleteJobResponse{
		JobId:   jobID,
		Message: "Job successfully deleted",
	})

	log.Printf("Successfully deleted job %s", jobID)
	return response, nil
}

// startJobPoller spawns a background goroutine to poll GCP Batch for job status updates.
func (s *WorkerServer) startJobPoller(ctx context.Context, tenantID, jobID, gcpResourcePath, initialStatus string) {
	poller := &JobPoller{
		tenantID:          tenantID,
		jobID:             jobID,
		gcpResourcePath:   gcpResourcePath,
		currentStatus:     initialStatus,
		batchProvider:     s.batchProvider,
		dbClient:          s.dbClient,
		pollingInterval:   5 * time.Second,
		maxFailedAttempts: 10,
		failedAttempts:    0,
		done:              make(chan bool),
	}

	// Register poller
	s.pollersMutex.Lock()
	if s.pollers == nil {
		s.pollers = make(map[string]*JobPoller)
	}
	pollerKey := fmt.Sprintf("%s/%s", tenantID, jobID)
	s.pollers[pollerKey] = poller
	s.pollersMutex.Unlock()

	log.Printf("Starting poller for job %s (tenant: %s)", jobID, tenantID)

	// Start polling in background with a new background context (independent of request lifecycle)
	go poller.poll(context.Background(), s)
}

// poll continuously checks job status and updates the database when status changes.
func (poller *JobPoller) poll(ctx context.Context, server *WorkerServer) {
	ticker := time.NewTicker(poller.pollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-poller.done:
			log.Printf("Poller for job %s (tenant: %s) stopped", poller.jobID, poller.tenantID)
			return

		case <-ticker.C:
			status, err := poller.batchProvider.GetJobStatus(ctx, poller.gcpResourcePath)
			if err != nil {
				poller.failedAttempts++
				log.Printf("Error polling job %s (attempt %d/%d): %v", poller.jobID, poller.failedAttempts, poller.maxFailedAttempts, err)

				if poller.failedAttempts >= poller.maxFailedAttempts {
					log.Printf("Max failed attempts reached for job %s, stopping poller", poller.jobID)
					poller.stop()
					return
				}
				continue
			}

			poller.failedAttempts = 0 // Reset on successful poll

			// Convert batch provider status to database status
			dbStatus := mapBatchStatusToDBStatus(status)

			// Check if status changed
			if dbStatus != poller.currentStatus {
				oldStatus := poller.currentStatus
				poller.currentStatus = dbStatus

				log.Printf("Job %s status changed: %s → %s", poller.jobID, oldStatus, dbStatus)

				// Update database with new status
				err := poller.dbClient.UpdateJobStatus(ctx, poller.tenantID, poller.jobID, dbStatus)
				if err != nil {
					log.Printf("Error updating job status in database: %v", err)
				}

				// Record state transition in audit trail
				transitionID := uuid.New().String()
				reason := fmt.Sprintf("Status updated from GCP Batch API")
				err = poller.dbClient.RecordStateTransition(ctx, poller.tenantID, poller.jobID, transitionID, &oldStatus, dbStatus, &reason)
				if err != nil {
					log.Printf("Error recording state transition: %v", err)
				}

				// Stop polling if job reached a terminal state
				if isTerminalStatus(dbStatus) {
					log.Printf("Job %s reached terminal status %s, stopping poller", poller.jobID, dbStatus)
					poller.stop()
					return
				}
			}
		}
	}
}

// stop signals the poller to stop polling.
func (poller *JobPoller) stop() {
	close(poller.done)
}

// StopAllPollers gracefully stops all active job pollers.
func (s *WorkerServer) StopAllPollers() {
	s.pollersMutex.Lock()
	defer s.pollersMutex.Unlock()

	log.Printf("Stopping %d active pollers", len(s.pollers))
	for pollerKey, poller := range s.pollers {
		log.Printf("Stopping poller: %s", pollerKey)
		poller.stop()
	}
	s.pollers = make(map[string]*JobPoller)
}

// mapBatchStatusToDBStatus converts batch provider JobStatus to database status constants.
func mapBatchStatusToDBStatus(status batch.JobStatus) string {
	switch status {
	case batch.JobStatusPending:
		return database.JobStatusPending
	case batch.JobStatusScheduled:
		return database.JobStatusScheduled
	case batch.JobStatusRunning:
		return database.JobStatusRunning
	case batch.JobStatusCompleted:
		return database.JobStatusCompleted
	case batch.JobStatusFailed:
		return database.JobStatusFailed
	case batch.JobStatusCancelled:
		return database.JobStatusCancelled
	default:
		return database.JobStatusPending
	}
}

// isTerminalStatus checks if a status is a terminal state (no further transitions expected).
func isTerminalStatus(status string) bool {
	return status == database.JobStatusCompleted ||
		status == database.JobStatusFailed ||
		status == database.JobStatusCancelled
}

// isCancellableStatus checks if a job can be cancelled in its current status.
func isCancellableStatus(status string) bool {
	return status == database.JobStatusPending ||
		status == database.JobStatusScheduled ||
		status == database.JobStatusRunning
}

// stopPollerForJob removes and stops a specific job's poller.
func (s *WorkerServer) stopPollerForJob(tenantID, jobID string) {
	s.pollersMutex.Lock()
	defer s.pollersMutex.Unlock()

	pollerKey := fmt.Sprintf("%s/%s", tenantID, jobID)
	if poller, exists := s.pollers[pollerKey]; exists {
		log.Printf("Stopping poller for job %s", jobID)
		poller.stop()
		delete(s.pollers, pollerKey)
	}
}