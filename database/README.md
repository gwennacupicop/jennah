# Database Layer for Project JENNAH

This directory contains the database schema for Cloud Spanner.

## Files

- **schema.sql** - DDL definitions for Tenants and Jobs tables

## Setup Status

✅ **Complete** - Tables have been created in the `main` database

## Schema Overview

### Tenants Table
Stores information about each organization/team using the platform.

| Column | Type | Description |
|--------|------|-------------|
| TenantId | STRING(36) | Primary key, UUID |
| Name | STRING(255) | Organization name |
| CreatedAt | TIMESTAMP | Creation timestamp |
| UpdatedAt | TIMESTAMP | Last update timestamp |

### Jobs Table
Stores deployment job information, interleaved with Tenants for performance.

| Column | Type | Description |
|--------|------|-------------|
| TenantId | STRING(36) | Foreign key to Tenants |
| JobId | STRING(36) | Primary key (with TenantId) |
| Status | STRING(50) | PENDING, RUNNING, COMPLETED, FAILED |
| ImageUri | STRING(1024) | Container image to run |
| Commands | ARRAY<STRING> | Commands to execute |
| CreatedAt | TIMESTAMP | Creation timestamp |
| UpdatedAt | TIMESTAMP | Last update timestamp |
| CompletedAt | TIMESTAMP | Completion timestamp (nullable) |
| ErrorMessage | STRING | Error details (nullable) |

### Why Interleaved Tables?

Jobs are **interleaved** with Tenants, meaning:
- Jobs for the same tenant are stored physically close together
- Queries like "get all jobs for tenant X" are extremely fast
- Deleting a tenant automatically deletes all its jobs (CASCADE)

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
