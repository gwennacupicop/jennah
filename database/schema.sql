CREATE TABLE Tenants (
  TenantId STRING(36) NOT NULL,
  UserEmail STRING(255) NOT NULL,
  OAuthProvider STRING(50) NOT NULL,
  OAuthUserId STRING(255) NOT NULL,
  CreatedAt TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
  UpdatedAt TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (TenantId);

CREATE INDEX TenantsByOAuth ON Tenants(OAuthProvider, OAuthUserId);

CREATE TABLE Jobs (
  TenantId STRING(36) NOT NULL,
  JobId STRING(36) NOT NULL,
  Status STRING(50) NOT NULL,
  ImageUri STRING(1024),
  Commands ARRAY<STRING(MAX)>,
  CreatedAt TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
  UpdatedAt TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
  -- Job Lifecycle Timestamps
  ScheduledAt TIMESTAMP,
  StartedAt TIMESTAMP,
  CompletedAt TIMESTAMP,
  -- Retry and Error Handling
  RetryCount INT64 NOT NULL DEFAULT (0),
  MaxRetries INT64 NOT NULL DEFAULT (3),
  ErrorMessage STRING(MAX),
  -- GCP Batch Integration
  GcpBatchJobName STRING(1024),  -- GCP Batch job resource path: projects/{projectId}/locations/{region}/jobs/{jobId}
  GcpBatchTaskGroup STRING(1024),  -- GCP Batch task group identifier
  EnvVarsJson STRING(MAX),  -- Environment variables stored as JSON
  -- Worker lease ownership for failover
  OwnerWorkerId STRING(128),
  PreferredWorkerId STRING(128),
  LeaseExpiresAt TIMESTAMP,
  LastHeartbeatAt TIMESTAMP,
) PRIMARY KEY (TenantId, JobId),
  INTERLEAVE IN PARENT Tenants ON DELETE CASCADE;

CREATE INDEX JobsByStatus ON Jobs(TenantId, Status, CreatedAt DESC);

CREATE TABLE JobStateTransitions (
  TenantId STRING(36) NOT NULL,
  JobId STRING(36) NOT NULL,
  TransitionId STRING(36) NOT NULL,
  FromStatus STRING(50),
  ToStatus STRING(50) NOT NULL,
  TransitionedAt TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
  Reason STRING(MAX),
) PRIMARY KEY (TenantId, JobId, TransitionId),
  INTERLEAVE IN PARENT Jobs ON DELETE CASCADE;

CREATE INDEX TransitionsByJob ON JobStateTransitions(TenantId, JobId, TransitionedAt DESC);
