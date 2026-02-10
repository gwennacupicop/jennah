CREATE TABLE Tenants (
  TenantId STRING(36) NOT NULL,
  Name STRING(255) NOT NULL,
  CreatedAt TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
  UpdatedAt TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (TenantId);

CREATE TABLE Jobs (
  TenantId STRING(36) NOT NULL,
  JobId STRING(36) NOT NULL,
  Status STRING(50) NOT NULL,
  ImageUri STRING(1024),
  Commands ARRAY<STRING(MAX)>,
  CreatedAt TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
  UpdatedAt TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
  CompletedAt TIMESTAMP,
  ErrorMessage STRING(MAX),
) PRIMARY KEY (TenantId, JobId),
  INTERLEAVE IN PARENT Tenants ON DELETE CASCADE;

CREATE INDEX JobsByStatus ON Jobs(TenantId, Status, CreatedAt DESC);
