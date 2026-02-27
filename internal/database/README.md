# Database Package

Go package for interacting with Cloud Spanner database.

## Overview

This package provides a clean API for managing Tenants and Jobs in Cloud Spanner.

## Installation

Add the Cloud Spanner dependency:

```bash
go get cloud.google.com/go/spanner
```

## Usage

### Initialize Client

```go
import (
    "context"
    "github.com/alphauslabs/jennah/internal/database"
)

ctx := context.Background()
client, err := database.NewClient(ctx, "labs-169405", "alphaus-dev", "main")
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

### Tenant Operations

```go
// Create a tenant
err := client.InsertTenant(ctx, "tenant-123", "Acme Corp")

// Get a tenant
tenant, err := client.GetTenant(ctx, "tenant-123")

// List all tenants
tenants, err := client.ListTenants(ctx)

// Delete a tenant (also deletes all jobs)
err := client.DeleteTenant(ctx, "tenant-123")
```

### Job Operations

```go
// Create a job
err := client.InsertJob(ctx, "tenant-123", "job-456", 
    "gcr.io/project/image:latest", 
    []string{"echo", "hello"})

// Get a job
job, err := client.GetJob(ctx, "tenant-123", "job-456")

// List all jobs for a tenant
jobs, err := client.ListJobs(ctx, "tenant-123")

// List jobs by status
runningJobs, err := client.ListJobsByStatus(ctx, "tenant-123", database.JobStatusRunning)

// Update job status
err := client.UpdateJobStatus(ctx, "tenant-123", "job-456", database.JobStatusRunning)

// Mark job as completed
err := client.CompleteJob(ctx, "tenant-123", "job-456")

// Mark job as failed
err := client.FailJob(ctx, "tenant-123", "job-456", "Container failed to start")

// Delete a job
err := client.DeleteJob(ctx, "tenant-123", "job-456")
```

## Job Status Constants

- `database.JobStatusPending` - "PENDING"
- `database.JobStatusRunning` - "RUNNING"
- `database.JobStatusCompleted` - "COMPLETED"
- `database.JobStatusFailed` - "FAILED"

## Data Models

### Tenant
```go
type Tenant struct {
    TenantId  string
    Name      string
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### Job
```go
type Job struct {
    TenantId     string
    JobId        string
    Status       string
    ImageUri     string
    Commands     []string
    CreatedAt    time.Time
    UpdatedAt    time.Time
    CompletedAt  *time.Time
    ErrorMessage *string
}
```

## Connection Details

```
Project:  labs-169405
Instance: alphaus-dev
Database: main
```

## Notes

- Jobs are interleaved with Tenants for optimal query performance
- Deleting a tenant automatically cascades to delete all its jobs
- `CreatedAt` and `UpdatedAt` use Spanner commit timestamps
- The `JobsByStatus` index optimizes status-based queries
