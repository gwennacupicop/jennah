package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	batch "github.com/alphauslabs/jennah/internal/cloudexec"
	"github.com/alphauslabs/jennah/internal/database"
	"github.com/alphauslabs/jennah/internal/router"
)

// JobPoller manages polling of a single job's status from the batch provider.
type JobPoller struct {
	tenantID          string
	jobID             string
	gcpResourcePath   string
	currentStatus     string
	serviceTier       string
	assignedService   router.AssignedService
	batchProvider     batch.Provider
	dbClient          *database.Client
	ticker            *time.Ticker
	done              chan bool
	stopOnce          sync.Once
	pollingInterval   time.Duration
	maxFailedAttempts int
	failedAttempts    int
}

// startJobPoller spawns a background goroutine to poll the batch provider for job status updates.
func (s *WorkerService) startJobPoller(ctx context.Context, tenantID, jobID, gcpResourcePath, initialStatus, serviceTier string) {
	s.startJobPollerWithService(ctx, tenantID, jobID, gcpResourcePath, initialStatus, serviceTier, router.AssignedServiceUnspecified)
}

// startJobPollerWithService spawns a background goroutine to poll the correct provider for job status updates.
func (s *WorkerService) startJobPollerWithService(ctx context.Context, tenantID, jobID, gcpResourcePath, initialStatus, serviceTier string, assignedService router.AssignedService) {
	pollerKey := fmt.Sprintf("%s/%s", tenantID, jobID)

	// Determine the correct provider to use based on assignedService or serviceTier
	var provider batch.Provider
	var inferredService router.AssignedService

	if assignedService == router.AssignedServiceUnspecified {
		// Infer from service tier (for recovered jobs)
		if serviceTier == database.ServiceTierComplex {
			inferredService = router.AssignedServiceCloudBatch
		} else {
			// SIMPLE tier - default to Cloud Run (Cloud Tasks don't need polling)
			inferredService = router.AssignedServiceCloudRunJob
		}
	} else {
		inferredService = assignedService
	}

	// Get the correct provider from dispatcher
	if s.dispatcher != nil {
		var err error
		provider, err = s.dispatcher.ProviderFor(inferredService)
		if err != nil {
			log.Printf("Failed to get provider for service %s, falling back to batchProvider: %v", inferredService, err)
			provider = s.batchProvider
		}
	} else {
		provider = s.batchProvider
	}

	poller := &JobPoller{
		tenantID:          tenantID,
		jobID:             jobID,
		gcpResourcePath:   gcpResourcePath,
		currentStatus:     initialStatus,
		serviceTier:       serviceTier,
		assignedService:   inferredService,
		batchProvider:     provider,
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
	if _, exists := s.pollers[pollerKey]; exists {
		s.pollersMutex.Unlock()
		return
	}
	s.pollers[pollerKey] = poller
	s.pollersMutex.Unlock()

	log.Printf("Starting poller for job %s (tenant: %s, service: %s, provider: %s)", jobID, tenantID, inferredService, provider.ServiceType())

	// Start polling in background with a new context (independent of request lifecycle).
	go poller.poll(context.Background(), s, pollerKey)
}

// poll continuously checks job status and updates the database when status changes.
func (poller *JobPoller) poll(ctx context.Context, server *WorkerService, pollerKey string) {
	ticker := time.NewTicker(poller.pollingInterval)
	defer ticker.Stop()
	defer server.unregisterPoller(pollerKey)

	for {
		select {
		case <-poller.done:
			log.Printf("Poller for job %s (tenant: %s) stopped", poller.jobID, poller.tenantID)
			return

		case <-ticker.C:
			leaseUntil := time.Now().UTC().Add(server.leaseTTL)
			owned, err := poller.dbClient.TryClaimOrRenewJobLease(ctx, poller.tenantID, poller.jobID, server.workerID, leaseUntil)
			if err != nil {
				log.Printf("Error renewing lease for job %s: %v", poller.jobID, err)
				continue
			}

			if !owned {
				log.Printf("Lease ownership lost for job %s; stopping local poller", poller.jobID)
				poller.stop()
				return
			}

			status, err := poller.batchProvider.GetJobStatus(ctx, poller.gcpResourcePath)
			if err != nil {
				poller.failedAttempts++
				log.Printf("Error polling job %s (attempt %d/%d) [service=%s, tier=%s, path=%s]: %v", 
					poller.jobID, poller.failedAttempts, poller.maxFailedAttempts, 
					poller.assignedService, poller.serviceTier, poller.gcpResourcePath, err)

				// If this is a SIMPLE tier job failing with Cloud Run provider, it might actually be a Cloud Batch job
				// (e.g., created before the dispatcher was implemented). Try falling back to Cloud Batch.
				if poller.serviceTier == database.ServiceTierSimple && poller.assignedService == router.AssignedServiceCloudRunJob {
					if server.dispatcher != nil {
						if batchProvider, err := server.dispatcher.ProviderFor(router.AssignedServiceCloudBatch); err == nil {
							log.Printf("Retrying job %s with Cloud Batch provider (fallback)", poller.jobID)
							status, err = batchProvider.GetJobStatus(ctx, poller.gcpResourcePath)
							if err == nil {
								// Success! Update the poller to use Cloud Batch going forward
								log.Printf("Job %s is actually a Cloud Batch job, updating poller", poller.jobID)
								poller.batchProvider = batchProvider
								poller.assignedService = router.AssignedServiceCloudBatch
								poller.failedAttempts = 0 // Reset since we found the right provider
								// Don't continue; process the status below
								goto processStatus
							}
							log.Printf("Cloud Batch fallback also failed for job %s: %v", poller.jobID, err)
						}
					}
				}

				if poller.failedAttempts >= poller.maxFailedAttempts {
					log.Printf("Max failed attempts reached for job %s, stopping poller", poller.jobID)
					poller.stop()
					return
				}
				continue
			}

		processStatus:
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
	poller.stopOnce.Do(func() {
		close(poller.done)
	})
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

func (s *WorkerService) unregisterPoller(pollerKey string) {
	s.pollersMutex.Lock()
	defer s.pollersMutex.Unlock()
	delete(s.pollers, pollerKey)
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
	_ = dbClient
	return server.reconcileActiveJobLeases(ctx, true)
}

// StartLeaseReconciler continuously claims/renews active job ownership so jobs fail over across workers.
func (s *WorkerService) StartLeaseReconciler(ctx context.Context) {
	go func() {
		if err := s.reconcileActiveJobLeases(ctx, true); err != nil {
			log.Printf("Initial lease reconcile failed: %v", err)
		}

		ticker := time.NewTicker(s.claimInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("Lease reconciler stopped")
				return
			case <-ticker.C:
				if err := s.reconcileActiveJobLeases(context.Background(), false); err != nil {
					log.Printf("Lease reconcile tick failed: %v", err)
				}
			}
		}
	}()
}

func (s *WorkerService) reconcileActiveJobLeases(ctx context.Context, startup bool) error {
	if startup {
		log.Println("Scanning active jobs to claim poller leases...")
	}

	jobs, err := s.dbClient.ListActiveJobs(ctx)
	if err != nil {
		return fmt.Errorf("failed to list active jobs: %w", err)
	}

	claimedCount := 0
	for _, job := range jobs {
		if job.GcpBatchJobPath == nil {
			continue
		}

		// Note: We don't skip SIMPLE tier jobs anymore because Cloud Run jobs need polling.
		// Cloud Tasks jobs are rare and will just fail polling gracefully if encountered.

		owned, err := s.dbClient.TryClaimOrRenewJobLease(ctx, job.TenantId, job.JobId, s.workerID, time.Now().UTC().Add(s.leaseTTL))
		if err != nil {
			log.Printf("Lease claim failed for job %s: %v", job.JobId, err)
			continue
		}

		if !owned {
			continue
		}

		s.startJobPoller(ctx, job.TenantId, job.JobId, *job.GcpBatchJobPath, job.Status, ptrToString(job.ServiceTier))
		claimedCount++
	}

	if startup {
		log.Printf("Lease reconcile complete: %d job(s) owned by worker %s", claimedCount, s.workerID)
	}

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

// ptrToString safely dereferences a *string, returning "" if nil.
func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
