package database

import "time"

// Tenant represents an organization/team using the platform
type Tenant struct {
	TenantId      string    `spanner:"TenantId"`
	UserEmail     string    `spanner:"UserEmail"`
	OAuthProvider string    `spanner:"OAuthProvider"`
	OAuthUserId   string    `spanner:"OAuthUserId"`
	CreatedAt     time.Time `spanner:"CreatedAt"`
	UpdatedAt     time.Time `spanner:"UpdatedAt"`
}

// Job represents a deployment job
type Job struct {
	TenantId     string     `spanner:"TenantId"`
	JobId        string     `spanner:"JobId"`
	Status       string     `spanner:"Status"`
	ImageUri     string     `spanner:"ImageUri"`
	Commands     []string   `spanner:"Commands"`
	CreatedAt    time.Time  `spanner:"CreatedAt"`
	ScheduledAt  *time.Time `spanner:"ScheduledAt"`
	StartedAt    *time.Time `spanner:"StartedAt"`
	CompletedAt  *time.Time `spanner:"CompletedAt"`
	UpdatedAt    time.Time  `spanner:"UpdatedAt"`
	ErrorMessage *string    `spanner:"ErrorMessage"`
	RetryCount   int64      `spanner:"RetryCount"`
}

// JobStateTransition tracks state changes for audit trail
type JobStateTransition struct {
	TenantId       string    `spanner:"TenantId"`
	JobId          string    `spanner:"JobId"`
	TransitionId   string    `spanner:"TransitionId"`
	FromStatus     *string   `spanner:"FromStatus"`
	ToStatus       string    `spanner:"ToStatus"`
	TransitionedAt time.Time `spanner:"TransitionedAt"`
	Notes          *string   `spanner:"Notes"`
}

// JobStatus constants
const (
	JobStatusPending   = "PENDING"
	JobStatusScheduled = "SCHEDULED"
	JobStatusRunning   = "RUNNING"
	JobStatusCompleted = "COMPLETED"
	JobStatusFailed    = "FAILED"
	JobStatusCancelled = "CANCELLED"
)
