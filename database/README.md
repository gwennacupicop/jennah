# Database Layer for Project JENNAH

This directory contains the database schema for Cloud Spanner.

## Files

- **schema.sql** - DDL definitions for Tenants, Jobs, and JobStateTransitions tables
- **migrate-oauth.sql** - Migration script to add OAuth fields to existing database
- **migrate-lifecycle.sql** - Migration script to add job lifecycle tracking

## Setup Status

✅ **Complete** - Tables have been created in the `main` database  
⚠️ **Migration Required** - Run migrate-oauth.sql and migrate-lifecycle.sql to add new fields

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
| ScheduledAt | TIMESTAMP | When job was scheduled (PENDING → SCHEDULED) |
| StartedAt | TIMESTAMP | When job execution began (SCHEDULED → RUNNING) |
| CompletedAt | TIMESTAMP | When job finished (→ COMPLETED/FAILED/CANCELLED) |
| UpdatedAt | TIMESTAMP | Last update timestamp |
| ErrorMessage | STRING | Error details (nullable) |
| RetryCount | INT64 | Number of retry attempts |

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
| Notes | STRING | Additional context (nullable) |

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

To update the existing database:

**1. Add OAuth fields:**
```bash
gcloud spanner databases ddl update main \
  --instance=alphaus-dev \
  --project=labs-169405 \
  --ddl-file=migrate-oauth.sql
```

**2. Add lifecycle tracking:**
```bash
gcloud spanner databases ddl update main \
  --instance=alphaus-dev \
  --project=labs-169405 \
  --ddl-file=migrate-lifecycle.sql
```

Or run each statement individually in Cloud Console.

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
