# Database Layer for Project JENNAH

This directory contains the database schema for Cloud Spanner.

## Files

- **schema.sql** - DDL definitions for Tenants, Jobs, and JobStateTransitions tables
- **migrate-batch-integration.sql** - Migration script to add GCP Batch integration fields

## Setup Status

✅ **Complete** - Tables created in `main` database with OAuth and lifecycle tracking  
⚠️ **Migration Required** - Run migrate-batch-integration.sql to add MaxRetries and GcpBatchJobName

## Schema Overview

### Tenants Table
Stores information about each user/organization using the platform (linked via OAuth).

| Column | Type | Description |
|--------|------|-------------|
| TenantId | STRING(36) | Primary key, UUID |
| UserEmail | STRING(255) | User's email from OAuth |
| OAuthProvider | STRING(50) | OAuth provider (google, github, etc.) |
| OAuthUserId | STRING(255) | User ID from OAuth provider |
| CreatedAt | TIMESTAMP | Creation timestamp |
| UpdatedAt | TIMESTAMP | Last update timestamp |

### Jobs Table
Stores deployment job information with lifecycle tracking, interleaved with Tenants for performance.

| Column | Type | Description |
|--------|------|-------------|
| TenantId | STRING(36) | Foreign key to Tenants |
| JobId | STRING(36) | Primary key (with TenantId) |
| Status | STRING(50) | PENDING, SCHEDULED, RUNNING, COMPLETED, FAILED, CANCELLED |
| ImageUri | STRING(1024) | Container image to run |
| Commands | ARRAY<STRING> | Commands to execute |
| CreatedAt | TIMESTAMP | Job creation timestamp |
| UpdatedAt | TIMESTAMP | Last update timestamp |
| ScheduledAt | TIMESTAMP | When job was scheduled (PENDING → SCHEDULED) |
| StartedAt | TIMESTAMP | When job execution began (SCHEDULED → RUNNING) |
| CompletedAt | TIMESTAMP | When job finished (→ COMPLETED/FAILED/CANCELLED) |
| RetryCount | INT64 | Number of retry attempts (default: 0) |
| MaxRetries | INT64 | Maximum retry attempts allowed (default: 3) |
| ErrorMessage | STRING | Error details (nullable) |
| GcpBatchJobName | STRING(1024) | GCP Batch job resource name (nullable) |

### JobStateTransitions Table
Tracks all state changes for audit trail and debugging, interleaved with Jobs.

| Column | Type | Description |
|--------|------|-------------|
| TenantId | STRING(36) | Foreign key to Jobs |
| JobId | STRING(36) | Foreign key to Jobs |
| TransitionId | STRING(36) | Primary key (with TenantId, JobId) |
| FromStatus | STRING(50) | Previous status (nullable for initial state) |
| ToStatus | STRING(50) | New status |
| TransitionedAt | TIMESTAMP | When transition occurred |
| Reason | STRING | Error details, cancellation reason, etc. (nullable) |

### Job Lifecycle Flow

```
PENDING → SCHEDULED → RUNNING → COMPLETED
                               → FAILED → PENDING (retry)
                               → CANCELLED
```

**State Transitions:**
1. **PENDING** → Job created, awaiting worker processing
2. **SCHEDULED** → Worker validated request, GCP Batch job created
3. **RUNNING** → GCP Batch reports job started execution
4. **COMPLETED** → Job finished successfully
5. **FAILED** → Job failed (may retry to PENDING if RetryCount < MaxRetries)
6. **CANCELLED** → User or system cancelled the job

### Why Interleaved Tables?

**Jobs** are interleaved with **Tenants**, and **JobStateTransitions** are interleaved with **Jobs**, meaning:
- Jobs for the same tenant are stored physically close together
- State transitions for a job are stored adjacent to the job
- Queries like "get all jobs for tenant X" or "get all transitions for job Y" are extremely fast
- Deleting a tenant cascades to delete all its jobs and transitions

## Migration Instructions

To add GCP Batch integration fields to existing database:

```bash
gcloud spanner databases ddl update main \
  --instance=alphaus-dev \
  --project=labs-169405 \
  --ddl-file=migrate-batch-integration.sql
```

**Or run these DDL statements in Cloud Console:**

```sql
ALTER TABLE Jobs ADD COLUMN MaxRetries INT64 NOT NULL DEFAULT (3);
ALTER TABLE Jobs ADD COLUMN GcpBatchJobName STRING(1024);
UPDATE Jobs SET MaxRetries = 3 WHERE MaxRetries IS NULL;
```

## Connection Information

Share these details with your team:

```
Project: labs-169405
Instance: alphaus-dev
Database: main
Region: us-central1
```

## Next Steps

1. ✅ ~~Database setup complete~~
2. ⏭️ **Implement database access logic in Go** (current task)
3. ⏭️ Configure IAM roles and service accounts
4. ⏭️ Create Artifact Registry repository
5. ⏭️ Configure networking and static IPs
