package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/alphauslabs/jennah/internal/batch"
	"github.com/alphauslabs/jennah/internal/database"
)

// JobPoller manages polling of a single job's status from the batch provider.
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

// startJobPoller spawns a background goroutine to poll the batch provider for job status updates.
func (s *WorkerService) startJobPoller(ctx context.Context, tenantID, jobID, gcpResourcePath, initialStatus string) {
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

	// Register poller.
	s.pollersMutex.Lock()
	if s.pollers == nil {
		s.pollers = make(map[string]*JobPoller)
	}
	pollerKey := fmt.Sprintf("%s/%s", tenantID, jobID)
	s.pollers[pollerKey] = poller
	s.pollersMutex.Unlock()

	log.Printf("Starting poller for job %s (tenant: %s)", jobID, tenantID)

	// Start polling in background with a new context (independent of request lifecycle).
	go poller.poll(context.Background(), s)
}

// poll continuously checks job status and updates the database when status changes.
func (poller *JobPoller) poll(ctx context.Context, server *WorkerService) {
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

			poller.failedAttempts = 0 // Reset on successful poll.

			// Convert batch provider status to database status.
			dbStatus := mapBatchStatusToDBStatus(status)

			// Check if status changed.
			if dbStatus != poller.currentStatus {
				oldStatus := poller.currentStatus
				poller.currentStatus = dbStatus

				log.Printf("Job %s status changed: %s → %s", poller.jobID, oldStatus, dbStatus)

				// Update database with new status.
				err := poller.dbClient.UpdateJobStatus(ctx, poller.tenantID, poller.jobID, dbStatus)
				if err != nil {
					log.Printf("Error updating job status in database: %v", err)
				}

				// Record state transition in audit trail.
				transitionID := uuid.New().String()
				reason := "Status updated from GCP Batch API"
				err = poller.dbClient.RecordStateTransition(ctx, poller.tenantID, poller.jobID, transitionID, &oldStatus, dbStatus, &reason)
				if err != nil {
					log.Printf("Error recording state transition: %v", err)
				}

				// Stop polling if job reached a terminal state.
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
func (s *WorkerService) StopAllPollers() {
	s.pollersMutex.Lock()
	defer s.pollersMutex.Unlock()

	log.Printf("Stopping %d active pollers", len(s.pollers))
	for pollerKey, poller := range s.pollers {
		log.Printf("Stopping poller: %s", pollerKey)
		poller.stop()
	}
	s.pollers = make(map[string]*JobPoller)
}

// stopPollerForJob removes and stops a specific job's poller.
func (s *WorkerService) stopPollerForJob(tenantID, jobID string) {
	s.pollersMutex.Lock()
	defer s.pollersMutex.Unlock()

	pollerKey := fmt.Sprintf("%s/%s", tenantID, jobID)
	if poller, exists := s.pollers[pollerKey]; exists {
		log.Printf("Stopping poller for job %s", jobID)
		poller.stop()
		delete(s.pollers, pollerKey)
	}
}

// ResumeActiveJobPollers finds all non-terminal jobs across all tenants and restarts their pollers.
func ResumeActiveJobPollers(ctx context.Context, server *WorkerService, dbClient *database.Client) error {
	log.Println("Scanning for active jobs to resume polling...")

	tenants, err := dbClient.ListTenants(ctx)
	if err != nil {
		return fmt.Errorf("failed to list tenants: %w", err)
	}

	if len(tenants) == 0 {
		log.Println("No tenants found")
		return nil
	}

	log.Printf("Found %d tenant(s)", len(tenants))

	resumedCount := 0

	for _, tenant := range tenants {
		jobs, err := dbClient.ListJobs(ctx, tenant.TenantId)
		if err != nil {
			log.Printf("Error listing jobs for tenant %s: %v", tenant.TenantId, err)
			continue
		}

		for _, job := range jobs {
			// Skip terminal statuses — no need to poll.
			if isTerminalStatus(job.Status) {
				continue
			}

			// Skip jobs without GCP Batch reference.
			if job.GcpBatchJobName == nil {
				log.Printf("Skipping poller for job %s: no GCP Batch resource", job.JobId)
				continue
			}

			log.Printf("Resuming poller for job %s (status: %s)", job.JobId, job.Status)
			server.startJobPoller(ctx, tenant.TenantId, job.JobId, *job.GcpBatchJobName, job.Status)
			resumedCount++
		}
	}

	log.Printf("Job poller resume complete: %d poller(s) started", resumedCount)
	return nil
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
