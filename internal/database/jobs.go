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
			[]string{"TenantId", "JobId", "Status", "ImageUri", "Commands", "CreatedAt", "UpdatedAt", "RetryCount", "MaxRetries", "CloudJobResourcePath"},
			[]interface{}{tenantID, jobID, JobStatusPending, imageUri, commands, spanner.CommitTimestamp, spanner.CommitTimestamp, 0, 3},
		),
	})
	return err
}

// GetJob retrieves a job by tenant ID and job ID
func (c *Client) GetJob(ctx context.Context, tenantID, jobID string) (*Job, error) {
	row, err := c.client.Single().ReadRow(ctx, "Jobs",
		spanner.Key{tenantID, jobID},
		[]string{"TenantId", "JobId", "Status", "ImageUri", "Commands", "CreatedAt", "UpdatedAt", "ScheduledAt", "StartedAt", "CompletedAt", "RetryCount", "MaxRetries", "ErrorMessage", "CloudJobResourcePath"},
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
		SQL: `SELECT TenantId, JobId, Status, ImageUri, Commands, CreatedAt, UpdatedAt, ScheduledAt, StartedAt, CompletedAt, RetryCount, MaxRetries, ErrorMessage, CloudJobResourcePath
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
		SQL: `SELECT TenantId, JobId, Status, ImageUri, Commands, CreatedAt, UpdatedAt, ScheduledAt, StartedAt, CompletedAt, RetryCount, MaxRetries, ErrorMessage, CloudJobResourcePath
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
