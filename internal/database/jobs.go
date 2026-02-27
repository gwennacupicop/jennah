package database

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
)

// InsertJob creates a new job with PENDING status
func (c *Client) InsertJob(ctx context.Context, tenantID, jobID, imageUri string, commands []string) error {
	_, err := c.client.Apply(ctx, []*spanner.Mutation{
		spanner.Insert("Jobs",
			[]string{"TenantId", "JobId", "Status", "ImageUri", "Commands", "CreatedAt", "UpdatedAt", "RetryCount", "MaxRetries", "GcpBatchJobName", "GcpBatchTaskGroup", "OwnerWorkerId", "PreferredWorkerId", "LeaseExpiresAt", "LastHeartbeatAt"},
			[]interface{}{tenantID, jobID, JobStatusPending, imageUri, commands, spanner.CommitTimestamp, spanner.CommitTimestamp, 0, 3, nil, nil, nil, nil, nil, nil},
		),
	})
	return err
}

// InsertJobFull creates a new job with all fields including advanced configuration.
func (c *Client) InsertJobFull(ctx context.Context, job *Job) error {
	_, err := c.client.Apply(ctx, []*spanner.Mutation{
		spanner.Insert("Jobs",
			[]string{
				"TenantId", "JobId", "Status", "ImageUri", "Commands",
				"CreatedAt", "UpdatedAt", "RetryCount", "MaxRetries",
				"GcpBatchJobName", "GcpBatchTaskGroup", "EnvVarsJson",
				"Name", "ResourceProfile", "MachineType",
				"BootDiskSizeGb", "UseSpotVms", "ServiceAccount",
				"OwnerWorkerId", "PreferredWorkerId", "LeaseExpiresAt", "LastHeartbeatAt",
			},
			[]interface{}{
				job.TenantId, job.JobId, job.Status, job.ImageUri, job.Commands,
				spanner.CommitTimestamp, spanner.CommitTimestamp, job.RetryCount, job.MaxRetries,
				job.GcpBatchJobName, job.GcpBatchTaskGroup, job.EnvVarsJson,
				job.Name, job.ResourceProfile, job.MachineType,
				job.BootDiskSizeGb, job.UseSpotVms, job.ServiceAccount,
				job.OwnerWorkerId, job.PreferredWorkerId, job.LeaseExpiresAt, job.LastHeartbeatAt,
			},
		),
	})
	return err
}

// GetJob retrieves a job by tenant ID and job ID
func (c *Client) GetJob(ctx context.Context, tenantID, jobID string) (*Job, error) {
	row, err := c.client.Single().ReadRow(ctx, "Jobs",
		spanner.Key{tenantID, jobID},
		[]string{"TenantId", "JobId", "Status", "ImageUri", "Commands", "CreatedAt", "UpdatedAt", "ScheduledAt", "StartedAt", "CompletedAt", "RetryCount", "MaxRetries", "ErrorMessage", "GcpBatchJobName", "GcpBatchTaskGroup", "EnvVarsJson", "Name", "ResourceProfile", "MachineType", "BootDiskSizeGb", "UseSpotVms", "ServiceAccount", "OwnerWorkerId", "PreferredWorkerId", "LeaseExpiresAt", "LastHeartbeatAt"},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	var job Job
	if err := row.ToStruct(&job); err != nil {
		return nil, fmt.Errorf("failed to parse job: %w", err)
	}

	return &job, nil
}

// ListJobs returns all jobs for a tenant
func (c *Client) ListJobs(ctx context.Context, tenantID string) ([]*Job, error) {
	stmt := spanner.Statement{
		SQL: `SELECT TenantId, JobId, Status, ImageUri, Commands, CreatedAt, UpdatedAt, ScheduledAt, StartedAt, CompletedAt, RetryCount, MaxRetries, ErrorMessage, GcpBatchJobName, GcpBatchTaskGroup, EnvVarsJson, Name, ResourceProfile, MachineType, BootDiskSizeGb, UseSpotVms, ServiceAccount, OwnerWorkerId, PreferredWorkerId, LeaseExpiresAt, LastHeartbeatAt
		      FROM Jobs 
		      WHERE TenantId = @tenantId 
		      ORDER BY CreatedAt DESC`,
		Params: map[string]interface{}{
			"tenantId": tenantID,
		},
	}

	iter := c.client.Single().Query(ctx, stmt)
	defer iter.Stop()

	var jobs []*Job
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate jobs: %w", err)
		}

		var job Job
		if err := row.ToStruct(&job); err != nil {
			return nil, fmt.Errorf("failed to parse job: %w", err)
		}
		jobs = append(jobs, &job)
	}

	return jobs, nil
}

// ListJobsByStatus returns jobs for a tenant filtered by status
func (c *Client) ListJobsByStatus(ctx context.Context, tenantID, status string) ([]*Job, error) {
	stmt := spanner.Statement{
		SQL: `SELECT TenantId, JobId, Status, ImageUri, Commands, CreatedAt, UpdatedAt, ScheduledAt, StartedAt, CompletedAt, RetryCount, MaxRetries, ErrorMessage, GcpBatchJobName, GcpBatchTaskGroup, EnvVarsJson, Name, ResourceProfile, MachineType, BootDiskSizeGb, UseSpotVms, ServiceAccount, OwnerWorkerId, PreferredWorkerId, LeaseExpiresAt, LastHeartbeatAt
		      FROM Jobs@{FORCE_INDEX=JobsByStatus}
		      WHERE TenantId = @tenantId AND Status = @status 
		      ORDER BY CreatedAt DESC`,
		Params: map[string]interface{}{
			"tenantId": tenantID,
			"status":   status,
		},
	}

	iter := c.client.Single().Query(ctx, stmt)
	defer iter.Stop()

	var jobs []*Job
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate jobs: %w", err)
		}

		var job Job
		if err := row.ToStruct(&job); err != nil {
			return nil, fmt.Errorf("failed to parse job: %w", err)
		}
		jobs = append(jobs, &job)
	}

	return jobs, nil
}

// UpdateJobStatus updates the status of a job
func (c *Client) UpdateJobStatus(ctx context.Context, tenantID, jobID, status string) error {
	_, err := c.client.Apply(ctx, []*spanner.Mutation{
		spanner.Update("Jobs",
			[]string{"TenantId", "JobId", "Status", "UpdatedAt"},
			[]any{tenantID, jobID, status, spanner.CommitTimestamp},
		),
	})
	if err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}
	return nil
}

// UpdateJobStatusAndGcpBatchJobName updates the status and GCP Batch job name of a job
func (c *Client) UpdateJobStatusAndGcpBatchJobName(ctx context.Context, tenantID, jobID, status, gcpBatchJobName string) error {
	_, err := c.client.Apply(ctx, []*spanner.Mutation{
		spanner.Update("Jobs",
			[]string{"TenantId", "JobId", "Status", "GcpBatchJobName", "UpdatedAt"},
			[]any{tenantID, jobID, status, gcpBatchJobName, spanner.CommitTimestamp},
		),
	})
	if err != nil {
		return fmt.Errorf("failed to update job status and GCP Batch job name: %w", err)
	}
	return nil
}

// CompleteJob marks a job as completed with a completion timestamp
func (c *Client) CompleteJob(ctx context.Context, tenantID, jobID string) error {
	now := time.Now()
	_, err := c.client.Apply(ctx, []*spanner.Mutation{
		spanner.Update("Jobs",
			[]string{"TenantId", "JobId", "Status", "CompletedAt", "UpdatedAt"},
			[]any{tenantID, jobID, JobStatusCompleted, now, spanner.CommitTimestamp},
		),
	})
	if err != nil {
		return fmt.Errorf("failed to complete job: %w", err)
	}
	return nil
}

// FailJob marks a job as failed with an error message
func (c *Client) FailJob(ctx context.Context, tenantID, jobID, errorMessage string) error {
	now := time.Now()
	_, err := c.client.Apply(ctx, []*spanner.Mutation{
		spanner.Update("Jobs",
			[]string{"TenantId", "JobId", "Status", "ErrorMessage", "CompletedAt", "UpdatedAt"},
			[]any{tenantID, jobID, JobStatusFailed, errorMessage, now, spanner.CommitTimestamp},
		),
	})
	if err != nil {
		return fmt.Errorf("failed to fail job: %w", err)
	}
	return nil
}

// ScheduleJob marks a job as SCHEDULED with a scheduled timestamp
func (c *Client) ScheduleJob(ctx context.Context, tenantID, jobID string) error {
	now := time.Now()
	_, err := c.client.Apply(ctx, []*spanner.Mutation{
		spanner.Update("Jobs",
			[]string{"TenantId", "JobId", "Status", "ScheduledAt", "UpdatedAt"},
			[]any{tenantID, jobID, JobStatusScheduled, now, spanner.CommitTimestamp},
		),
	})
	if err != nil {
		return fmt.Errorf("failed to schedule job: %w", err)
	}
	return nil
}

// StartJob marks a job as RUNNING with a started timestamp
func (c *Client) StartJob(ctx context.Context, tenantID, jobID string) error {
	now := time.Now()
	_, err := c.client.Apply(ctx, []*spanner.Mutation{
		spanner.Update("Jobs",
			[]string{"TenantId", "JobId", "Status", "StartedAt", "UpdatedAt"},
			[]interface{}{tenantID, jobID, JobStatusRunning, now, spanner.CommitTimestamp},
		),
	})
	if err != nil {
		return fmt.Errorf("failed to start job: %w", err)
	}
	return nil
}

// CancelJob marks a job as CANCELLED
func (c *Client) CancelJob(ctx context.Context, tenantID, jobID string) error {
	now := time.Now()
	_, err := c.client.Apply(ctx, []*spanner.Mutation{
		spanner.Update("Jobs",
			[]string{"TenantId", "JobId", "Status", "CompletedAt", "UpdatedAt"},
			[]interface{}{tenantID, jobID, JobStatusCancelled, now, spanner.CommitTimestamp},
		),
	})
	if err != nil {
		return fmt.Errorf("failed to cancel job: %w", err)
	}
	return nil
}

// DeleteJob removes a job
func (c *Client) DeleteJob(ctx context.Context, tenantID, jobID string) error {
	_, err := c.client.Apply(ctx, []*spanner.Mutation{
		spanner.Delete("Jobs", spanner.Key{tenantID, jobID}),
	})
	if err != nil {
		return fmt.Errorf("failed to delete job: %w", err)
	}
	return nil
}

// ListActiveJobs returns all active (non-terminal) jobs across tenants that have a cloud resource path.
func (c *Client) ListActiveJobs(ctx context.Context) ([]*Job, error) {
	stmt := spanner.Statement{
		SQL: `SELECT TenantId, JobId, Status, ImageUri, Commands, CreatedAt, UpdatedAt, ScheduledAt, StartedAt, CompletedAt, RetryCount, MaxRetries, ErrorMessage, GcpBatchJobName, GcpBatchTaskGroup, EnvVarsJson, Name, ResourceProfile, MachineType, BootDiskSizeGb, UseSpotVms, ServiceAccount, OwnerWorkerId, PreferredWorkerId, LeaseExpiresAt, LastHeartbeatAt
		      FROM Jobs
		      WHERE Status IN (@pending, @scheduled, @running)
		        AND GcpBatchJobName IS NOT NULL
		      ORDER BY UpdatedAt DESC`,
		Params: map[string]interface{}{
			"pending":   JobStatusPending,
			"scheduled": JobStatusScheduled,
			"running":   JobStatusRunning,
		},
	}

	iter := c.client.Single().Query(ctx, stmt)
	defer iter.Stop()

	var jobs []*Job
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate active jobs: %w", err)
		}

		var job Job
		if err := row.ToStruct(&job); err != nil {
			return nil, fmt.Errorf("failed to parse active job: %w", err)
		}
		jobs = append(jobs, &job)
	}

	return jobs, nil
}

// TryClaimOrRenewJobLease attempts to claim/renew ownership for an active job.
// Returns true when caller becomes/continues owner.
func (c *Client) TryClaimOrRenewJobLease(ctx context.Context, tenantID, jobID, workerID string, leaseUntil time.Time) (bool, error) {
	claimed := false
	_, err := c.client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		row, err := txn.ReadRow(ctx, "Jobs", spanner.Key{tenantID, jobID}, []string{"Status", "OwnerWorkerId", "PreferredWorkerId", "LeaseExpiresAt"})
		if err != nil {
			return fmt.Errorf("failed to read job lease state: %w", err)
		}

		var status string
		var ownerWorkerID spanner.NullString
		var preferredWorkerID spanner.NullString
		var leaseExpiresAt spanner.NullTime
		if err := row.Columns(&status, &ownerWorkerID, &preferredWorkerID, &leaseExpiresAt); err != nil {
			return fmt.Errorf("failed to parse job lease state: %w", err)
		}

		if status == JobStatusCompleted || status == JobStatusFailed || status == JobStatusCancelled {
			return nil
		}

		now := time.Now().UTC()
		isOwner := ownerWorkerID.Valid && ownerWorkerID.StringVal == workerID
		leaseExpired := !leaseExpiresAt.Valid || leaseExpiresAt.Time.Before(now)
		isUnowned := !ownerWorkerID.Valid || ownerWorkerID.StringVal == ""
		preferredTakeover := preferredWorkerID.Valid && preferredWorkerID.StringVal == workerID && !isOwner

		canClaim := isOwner || leaseExpired || isUnowned || preferredTakeover
		if !canClaim {
			return nil
		}

		mutation := spanner.Update("Jobs",
			[]string{"TenantId", "JobId", "OwnerWorkerId", "LeaseExpiresAt", "LastHeartbeatAt", "UpdatedAt"},
			[]interface{}{tenantID, jobID, workerID, leaseUntil, now, spanner.CommitTimestamp},
		)
		if err := txn.BufferWrite([]*spanner.Mutation{mutation}); err != nil {
			return fmt.Errorf("failed to buffer lease mutation: %w", err)
		}
		claimed = true
		return nil
	})

	if err != nil {
		return false, fmt.Errorf("failed to claim/renew lease: %w", err)
	}

	return claimed, nil
}
