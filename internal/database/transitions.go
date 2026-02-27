package database

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
)

// RecordStateTransition creates a new state transition record
func (c *Client) RecordStateTransition(ctx context.Context, tenantID, jobID, transitionID string, fromStatus *string, toStatus string, reason *string) error {
	_, err := c.client.Apply(ctx, []*spanner.Mutation{
		spanner.Insert("JobStateTransitions",
			[]string{"TenantId", "JobId", "TransitionId", "FromStatus", "ToStatus", "TransitionedAt", "Reason"},
			[]interface{}{tenantID, jobID, transitionID, fromStatus, toStatus, spanner.CommitTimestamp, reason},
		),
	})
	if err != nil {
		return fmt.Errorf("failed to record state transition: %w", err)
	}
	return nil
}

// GetJobTransitions retrieves all state transitions for a job
func (c *Client) GetJobTransitions(ctx context.Context, tenantID, jobID string) ([]*JobStateTransition, error) {
	stmt := spanner.Statement{
		SQL: `SELECT TenantId, JobId, TransitionId, FromStatus, ToStatus, TransitionedAt, Reason 
		      FROM JobStateTransitions 
		      WHERE TenantId = @tenantId AND JobId = @jobId 
		      ORDER BY TransitionedAt DESC`,
		Params: map[string]interface{}{
			"tenantId": tenantID,
			"jobId":    jobID,
		},
	}

	iter := c.client.Single().Query(ctx, stmt)
	defer iter.Stop()

	var transitions []*JobStateTransition
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate transitions: %w", err)
		}

		var transition JobStateTransition
		if err := row.ToStruct(&transition); err != nil {
			return nil, fmt.Errorf("failed to parse transition: %w", err)
		}
		transitions = append(transitions, &transition)
	}

	return transitions, nil
}
