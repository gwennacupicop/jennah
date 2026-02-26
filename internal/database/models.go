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
	TenantId          string     `spanner:"TenantId"`
	JobId             string     `spanner:"JobId"`
	Status            string     `spanner:"Status"`
	ImageUri          string     `spanner:"ImageUri"`
	Commands          []string   `spanner:"Commands"`
	CreatedAt         time.Time  `spanner:"CreatedAt"`
	UpdatedAt         time.Time  `spanner:"UpdatedAt"`
	ScheduledAt       *time.Time `spanner:"ScheduledAt"`
	StartedAt         *time.Time `spanner:"StartedAt"`
	CompletedAt       *time.Time `spanner:"CompletedAt"`
	RetryCount        int64      `spanner:"RetryCount"`
	MaxRetries        int64      `spanner:"MaxRetries"`
	ErrorMessage      *string    `spanner:"ErrorMessage"`
	GcpBatchJobName   *string    `spanner:"GcpBatchJobName"`
	GcpBatchTaskGroup *string    `spanner:"GcpBatchTaskGroup"`
	EnvVarsJson       *string    `spanner:"EnvVarsJson"`
	Name              *string    `spanner:"Name"`
	ResourceProfile   *string    `spanner:"ResourceProfile"`
	MachineType       *string    `spanner:"MachineType"`
	BootDiskSizeGb    *int64     `spanner:"BootDiskSizeGb"`
	UseSpotVms        *bool      `spanner:"UseSpotVms"`
	ServiceAccount    *string    `spanner:"ServiceAccount"`
	OwnerWorkerId     *string    `spanner:"OwnerWorkerId"`
	PreferredWorkerId *string    `spanner:"PreferredWorkerId"`
	LeaseExpiresAt    *time.Time `spanner:"LeaseExpiresAt"`
	LastHeartbeatAt   *time.Time `spanner:"LastHeartbeatAt"`
}

// JobStateTransition tracks state changes for audit trail
type JobStateTransition struct {
	TenantId       string    `spanner:"TenantId"`
	JobId          string    `spanner:"JobId"`
	TransitionId   string    `spanner:"TransitionId"`
	FromStatus     *string   `spanner:"FromStatus"`
	ToStatus       string    `spanner:"ToStatus"`
	TransitionedAt time.Time `spanner:"TransitionedAt"`
	Reason         *string   `spanner:"Reason"`
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
