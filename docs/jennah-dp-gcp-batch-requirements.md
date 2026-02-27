# GCP Batch API - Frontend Input Requirements

This document defines all input structures required by the frontend to create and manage GCP Batch jobs. Use this as a reference for building forms, validation, and API request payloads.

## Overview

GCP Batch manages batch processing workloads on Google Compute Engine. The frontend needs to collect user inputs to construct job configurations that are sent to the backend API.

## Table of Contents

1. [Job Creation Inputs](#job-creation-inputs)
2. [Field Definitions](#field-definitions)
3. [Status Values (Read-Only)](#status-values-read-only)
4. [Validation Rules](#validation-rules)
5. [Example Payloads](#example-payloads)

---

## Job Creation Inputs

### Required Fields for Job Creation

```json
{
  "taskGroups": [
    {
      "taskSpec": {
        "runnables": [
          {
            // At least one runnable is required
          }
        ]
      }
    }
  ]
}
```

### Complete Job Input Schema

```json
{
  "priority": 50,                    // Optional: 0-100 (default: 0)
  "taskGroups": [],                  // Required: Array of task groups
  "allocationPolicy": {},            // Optional: Resource allocation
  "labels": {},                      // Optional: Key-value labels
  "logsPolicy": {}                   // Optional: Logging configuration
}
```

**Output-Only Fields** (returned by API, not submitted):
- `name` - Resource identifier
- `uid` - Unique job ID
- `status` - Current job state
- `createTime` - Timestamp of creation
- `updateTime` - Last modification timestamp

---

## Field Definitions

### 1. Task Groups

**Field**: `taskGroups`
**Type**: Array
**Required**: Yes
**Min Items**: 1

Each task group contains:

```json
{
  "taskSpec": {
    "runnables": [],              // Required: Array of executables
    "computeResource": {},        // Optional: CPU/memory requirements
    "maxRunDuration": "3600s",    // Optional: Max duration (e.g., "3600s")
    "maxRetryCount": 3,           // Optional: Number of retries (default: 0)
    "lifecyclePolicy": {},        // Optional: Lifecycle handling
    "environment": {}             // Optional: Environment variables
  },
  "taskCount": 1,                 // Optional: Number of tasks (default: 1)
  "parallelism": 1                // Optional: Parallel execution count
}
```

### 2. Runnables

**Field**: `taskGroups[].taskSpec.runnables`
**Type**: Array
**Required**: Yes
**Min Items**: 1

Three types of runnables (choose one per runnable):

#### A. Container Runnable

```json
{
  "container": {
    "imageUri": "gcr.io/project/image:tag",  // Required: Container image
    "commands": ["/bin/bash"],               // Optional: Override entrypoint
    "entrypoint": "",                        // Optional: Container entrypoint
    "volumes": [],                           // Optional: Volume mounts
    "options": "",                           // Optional: Docker options
    "username": "",                          // Optional: Registry username
    "password": ""                           // Optional: Registry password
  },
  "environment": {                           // Optional: Env variables
    "variables": {
      "KEY": "value"
    }
  },
  "timeout": "3600s",                        // Optional: Timeout duration
  "ignoreExitStatus": false                  // Optional: Continue on failure
}
```

**Form Inputs Needed**:
- Text input: Container image URI (required)
- Text area: Commands (optional, comma/newline separated)
- Key-value input: Environment variables
- Duration input: Timeout (format: "3600s")
- Checkbox: Ignore exit status

#### B. Script Runnable

```json
{
  "script": {
    "text": "#!/bin/bash\necho 'Hello'",    // Either text OR path
    "path": "/path/to/script.sh"            // Either path OR text
  },
  "environment": {
    "variables": {
      "KEY": "value"
    }
  },
  "timeout": "3600s",
  "ignoreExitStatus": false
}
```

**Form Inputs Needed**:
- Radio: Choose between inline script or file path
- Text area: Inline script text
- Text input: Script file path
- Key-value input: Environment variables
- Duration input: Timeout
- Checkbox: Ignore exit status

#### C. Barrier Runnable

```json
{
  "barrier": {
    "name": "barrier-name"                   // Required: Barrier identifier
  }
}
```

**Form Inputs Needed**:
- Text input: Barrier name (used for synchronization)

### 3. Compute Resources

**Field**: `taskGroups[].taskSpec.computeResource`
**Type**: Object
**Required**: No (but recommended)

```json
{
  "cpuMilli": 2000,        // Optional: CPU in milli-cores (1000 = 1 core)
  "memoryMib": 4096,       // Optional: Memory in MiB
  "bootDiskMib": 10240     // Optional: Boot disk in MiB
}
```

**Form Inputs Needed**:
- Number input: CPU cores (convert to milli: value × 1000)
  - Label: "CPU Cores"
  - Placeholder: "2.0"
  - Help text: "Number of CPU cores (e.g., 2.0)"
- Number input: Memory in GB (convert to MiB: value × 1024)
  - Label: "Memory (GB)"
  - Placeholder: "4"
  - Help text: "Memory in gigabytes"
- Number input: Boot disk in GB (convert to MiB: value × 1024)
  - Label: "Boot Disk (GB)"
  - Placeholder: "10"
  - Help text: "Boot disk size in gigabytes"

**Conversion Examples**:
- 2.5 CPU cores → 2500 cpuMilli
- 4 GB memory → 4096 memoryMib
- 10 GB disk → 10240 bootDiskMib

### 4. Allocation Policy

**Field**: `allocationPolicy`
**Type**: Object
**Required**: No (but recommended)

```json
{
  "location": {
    "allowedLocations": ["zones/us-central1-a"]  // Optional: Allowed zones
  },
  "instances": [
    {
      "policy": {
        "machineType": "e2-standard-4",          // Optional: Machine type
        "provisioningModel": "STANDARD",         // Optional: STANDARD/SPOT/PREEMPTIBLE
        "accelerators": [],                      // Optional: GPU accelerators
        "disks": [],                             // Optional: Attached disks
        "minCpuPlatform": ""                     // Optional: CPU platform
      },
      "installGpuDrivers": false                 // Optional: Auto-install GPU drivers
    }
  ],
  "serviceAccount": {
    "email": "service-account@project.iam.gserviceaccount.com",
    "scopes": ["https://www.googleapis.com/auth/cloud-platform"]
  },
  "labels": {                                    // Optional: Resource labels
    "environment": "production"
  },
  "network": {
    "networkInterfaces": [
      {
        "network": "projects/PROJECT/global/networks/default",
        "subnetwork": "projects/PROJECT/regions/REGION/subnetworks/default",
        "noExternalIpAddress": false             // Optional: Disable external IP
      }
    ]
  },
  "tags": ["web-server", "batch-job"]            // Optional: Network tags
}
```

**Form Inputs Needed**:

**Location Section**:
- Multi-select dropdown: Zones
  - Options: `zones/us-central1-a`, `zones/us-central1-b`, etc.

**Instance Section**:
- Dropdown: Machine type
  - Options: `e2-standard-2`, `e2-standard-4`, `n1-standard-1`, etc.
- Radio buttons: Provisioning model
  - Options: `STANDARD` (Regular), `SPOT` (Low cost), `PREEMPTIBLE` (Legacy)
  - Default: `STANDARD`

**Service Account Section**:
- Text input: Service account email
- Multi-select: Scopes (or use full scope by default)

**Network Section**:
- Text input: Network path
- Text input: Subnetwork path
- Checkbox: "Disable external IP"

**Tags Section**:
- Tag input: Network tags (for firewall rules)

### 5. Priority

**Field**: `priority`
**Type**: Integer
**Required**: No
**Default**: 0
**Range**: 0-100

```json
{
  "priority": 50  // Higher = higher priority
}
```

**Form Input**:
- Slider or number input: Priority (0-100)
- Default value: 0
- Help text: "Higher values get scheduled first"

### 6. Labels

**Field**: `labels`
**Type**: Object (key-value pairs)
**Required**: No

```json
{
  "labels": {
    "environment": "production",
    "team": "data-engineering",
    "cost-center": "12345"
  }
}
```

**Form Input**:
- Key-value pair input component
- Both keys and values must be strings
- Help text: "Labels for organizing and filtering jobs"

### 7. Logs Policy

**Field**: `logsPolicy`
**Type**: Object
**Required**: No

```json
{
  "destination": "CLOUD_LOGGING",  // CLOUD_LOGGING or PATH
  "logsPath": "/var/logs/"         // Required if destination is PATH
}
```

**Form Inputs**:
- Radio buttons: Destination
  - Options: `CLOUD_LOGGING`, `PATH`
  - Default: `CLOUD_LOGGING`
- Text input: Logs path (conditional, shown if `PATH` selected)

### 8. Environment Variables

**Field**: `taskGroups[].taskSpec.environment` or `runnables[].environment`
**Type**: Object

```json
{
  "variables": {
    "API_KEY": "secret-key",
    "ENVIRONMENT": "production",
    "DEBUG": "false"
  },
  "secretVariables": {
    "DB_PASSWORD": "projects/PROJECT/secrets/db-pass/versions/latest"
  }
}
```

**Form Inputs**:
- Key-value pair component for regular variables
- Key-value pair component for secret variables (links to Secret Manager)
- Help text: "Environment variables available to the task"

### 9. Volumes

**Field**: `taskGroups[].taskSpec.volumes`
**Type**: Array
**Required**: No

#### A. GCS Volume

```json
{
  "gcs": {
    "remotePath": "gs://bucket-name/path"  // GCS bucket path
  },
  "mountPath": "/mnt/data",                // Mount point in container
  "mountOptions": ["ro"]                   // Optional: Mount options (e.g., read-only)
}
```

#### B. NFS Volume

```json
{
  "nfs": {
    "server": "10.0.0.1",                  // NFS server IP
    "remotePath": "/export/data"           // Remote path
  },
  "mountPath": "/mnt/nfs",
  "mountOptions": []
}
```

#### C. Device Volume

```json
{
  "deviceName": "disk-1",                  // Device name
  "mountPath": "/mnt/disk"
}
```

**Form Inputs**:
- Dropdown: Volume type (GCS, NFS, Device)
- Conditional inputs based on volume type:
  - **GCS**: Text input for bucket path (gs://...)
  - **NFS**: Text inputs for server IP and remote path
  - **Device**: Text input for device name
- Text input: Mount path (where to mount in container)
- Multi-select: Mount options (ro, rw, etc.)

---

## Status Values (Read-Only)

These values are returned by the API and should be displayed in the UI, but not submitted as inputs.

### Job Status States

```typescript
enum JobStatus {
  QUEUED = "QUEUED",                           // Awaiting resources
  SCHEDULED = "SCHEDULED",                     // Resources allocated
  RUNNING = "RUNNING",                         // Currently executing
  SUCCEEDED = "SUCCEEDED",                     // Completed successfully
  FAILED = "FAILED",                           // Job failed
  DELETION_IN_PROGRESS = "DELETION_IN_PROGRESS",
  CANCELLATION_IN_PROGRESS = "CANCELLATION_IN_PROGRESS",
  CANCELLED = "CANCELLED"
}
```

**UI Display**:
- Use badges/chips with color coding
- Green: SUCCEEDED
- Blue: QUEUED, SCHEDULED, RUNNING
- Yellow: CANCELLATION_IN_PROGRESS, DELETION_IN_PROGRESS
- Red: FAILED, CANCELLED

### Task Status States

```typescript
enum TaskStatus {
  PENDING = "PENDING",                         // Not yet assigned
  ASSIGNED = "ASSIGNED",                       // Assigned to VM
  RUNNING = "RUNNING",                         // Executing
  FAILED = "FAILED",                           // Task failed
  SUCCEEDED = "SUCCEEDED",                     // Completed successfully
  UNEXECUTED = "UNEXECUTED"                    // Was not executed
}
```

---

## Validation Rules

### Client-Side Validation

#### Required Fields

1. **Task Groups**
   - Must have at least 1 task group
   - Each task group must have a `taskSpec`
   - Each `taskSpec` must have at least 1 runnable

2. **Runnables**
   - Must specify exactly one of: `container`, `script`, or `barrier`
   - Container: `imageUri` is required
   - Script: Either `text` or `path` is required (not both)
   - Barrier: `name` is required

#### Field Constraints

| Field | Min | Max | Pattern/Notes |
|-------|-----|-----|---------------|
| `priority` | 0 | 100 | Integer |
| `cpuMilli` | 1 | - | Positive integer |
| `memoryMib` | 1 | - | Positive integer |
| `bootDiskMib` | 1 | - | Positive integer |
| `maxRetryCount` | 0 | - | Non-negative integer |
| `maxRunDuration` | - | - | Duration string: "3600s", "1h", "30m" |
| `timeout` | - | - | Duration string |
| Label keys | - | 63 chars | Lowercase, alphanumeric, hyphens, underscores |
| Label values | - | 63 chars | Same as keys |

#### Duration Format

Valid duration formats:
- Seconds: `"3600s"`
- Minutes: `"60m"`
- Hours: `"2h"`
- Combined: `"1h30m"`

**Validation Regex**: `^(\d+h)?(\d+m)?(\d+s)?$`

#### Label Validation

- Keys: `^[a-z0-9_-]{1,63}$`
- Values: `^[a-z0-9_-]{0,63}$`
- Max 64 labels per resource

---

## Example Payloads

### Minimal Job (Container)

```json
{
  "taskGroups": [
    {
      "taskSpec": {
        "runnables": [
          {
            "container": {
              "imageUri": "gcr.io/my-project/my-image:latest"
            }
          }
        ]
      }
    }
  ]
}
```

### Minimal Job (Script)

```json
{
  "taskGroups": [
    {
      "taskSpec": {
        "runnables": [
          {
            "script": {
              "text": "#!/bin/bash\necho 'Hello World'\npython process.py"
            }
          }
        ]
      }
    }
  ]
}
```

### Complete Job Example

```json
{
  "priority": 75,
  "labels": {
    "environment": "production",
    "team": "data-engineering"
  },
  "taskGroups": [
    {
      "taskCount": 10,
      "parallelism": 5,
      "taskSpec": {
        "computeResource": {
          "cpuMilli": 4000,
          "memoryMib": 8192,
          "bootDiskMib": 20480
        },
        "maxRunDuration": "7200s",
        "maxRetryCount": 3,
        "runnables": [
          {
            "container": {
              "imageUri": "gcr.io/my-project/data-processor:v2.0"
            },
            "environment": {
              "variables": {
                "BATCH_SIZE": "1000",
                "OUTPUT_PATH": "/mnt/output"
              }
            },
            "timeout": "3600s"
          }
        ],
        "volumes": [
          {
            "gcs": {
              "remotePath": "gs://my-bucket/data"
            },
            "mountPath": "/mnt/data"
          },
          {
            "gcs": {
              "remotePath": "gs://my-bucket/output"
            },
            "mountPath": "/mnt/output"
          }
        ],
        "environment": {
          "variables": {
            "PROJECT_ID": "my-project",
            "REGION": "us-central1"
          }
        }
      }
    }
  ],
  "allocationPolicy": {
    "location": {
      "allowedLocations": ["zones/us-central1-a", "zones/us-central1-b"]
    },
    "instances": [
      {
        "policy": {
          "machineType": "e2-standard-4",
          "provisioningModel": "SPOT"
        }
      }
    ],
    "serviceAccount": {
      "email": "batch-service@my-project.iam.gserviceaccount.com"
    },
    "network": {
      "networkInterfaces": [
        {
          "network": "projects/my-project/global/networks/default"
        }
      ]
    },
    "tags": ["batch-worker", "data-processing"]
  },
  "logsPolicy": {
    "destination": "CLOUD_LOGGING"
  }
}
```

### Multi-Step Job (Multiple Runnables)

```json
{
  "taskGroups": [
    {
      "taskSpec": {
        "runnables": [
          {
            "script": {
              "text": "#!/bin/bash\necho 'Step 1: Download data'\nwget https://example.com/data.zip\nunzip data.zip"
            }
          },
          {
            "container": {
              "imageUri": "gcr.io/my-project/processor:latest",
              "commands": ["python", "process.py", "--input", "/data"]
            }
          },
          {
            "script": {
              "text": "#!/bin/bash\necho 'Step 3: Upload results'\ngsutil cp /output/* gs://my-bucket/results/"
            }
          }
        ],
        "computeResource": {
          "cpuMilli": 2000,
          "memoryMib": 4096
        }
      }
    }
  ]
}
```

---

## API Endpoints Summary

### Job Operations

| Method | Endpoint | Input Required | Returns |
|--------|----------|----------------|---------|
| Create Job | `POST /projects/{project}/locations/{location}/jobs` | Job object (see above) | Job with status |
| Get Job | `GET /projects/{project}/locations/{location}/jobs/{jobId}` | Job ID in path | Job details |
| List Jobs | `GET /projects/{project}/locations/{location}/jobs` | Query params (filter, pageSize) | Array of jobs |
| Delete Job | `DELETE /projects/{project}/locations/{location}/jobs/{jobId}` | Job ID in path | Operation ID |
| Cancel Job | `POST /projects/{project}/locations/{location}/jobs/{jobId}:cancel` | Job ID in path | Operation ID |

### Query Parameters for List Jobs

```typescript
interface ListJobsParams {
  filter?: string;        // Filter expression (e.g., "status=RUNNING")
  pageSize?: number;      // Results per page (1-1000)
  pageToken?: string;     // Token from previous response
  orderBy?: string;       // Sort field (e.g., "createTime desc")
}
```

---

## Form Structure Recommendation

### Suggested Form Sections

1. **Basic Information**
   - Job name (generated or custom)
   - Priority (slider 0-100)
   - Labels (key-value pairs)

2. **Task Configuration**
   - Runnable type selector (Container/Script/Barrier)
   - Conditional fields based on runnable type
   - Environment variables
   - Timeout settings

3. **Compute Resources** (Collapsible/Optional)
   - CPU cores
   - Memory (GB)
   - Boot disk (GB)

4. **Advanced Settings** (Collapsible/Optional)
   - Max retry count
   - Max run duration
   - Task count
   - Parallelism

5. **Resource Allocation** (Collapsible/Optional)
   - Zones/regions
   - Machine type
   - Provisioning model
   - Service account

6. **Storage** (Collapsible/Optional)
   - Volume mounts (GCS, NFS, Device)
   - Add volume button

7. **Networking** (Collapsible/Optional)
   - Network configuration
   - Tags

8. **Logging** (Collapsible/Optional)
   - Destination (Cloud Logging / Path)
   - Log path (conditional)

---

## Response Format

When a job is created or retrieved, the response includes:

```json
{
  "name": "projects/PROJECT/locations/LOCATION/jobs/JOB_ID",
  "uid": "550e8400-e29b-41d4-a716-446655440000",
  "priority": 75,
  "taskGroups": [ /* as submitted */ ],
  "allocationPolicy": { /* as submitted */ },
  "labels": { /* as submitted */ },
  "status": {
    "state": "RUNNING",
    "statusEvents": [
      {
        "type": "TYPE_TASK_STATE_CHANGED",
        "description": "Task 0 changed from PENDING to RUNNING",
        "eventTime": "2024-01-15T10:30:00.000Z"
      }
    ],
    "taskGroups": {
      "0": {
        "counts": {
          "PENDING": 5,
          "RUNNING": 3,
          "SUCCEEDED": 2
        }
      }
    }
  },
  "createTime": "2024-01-15T10:00:00.000Z",
  "updateTime": "2024-01-15T10:30:00.000Z",
  "logsPolicy": { /* as submitted */ }
}
```

**Use for UI**:
- Display job name, UID
- Show status badge
- Display creation/update times
- Show task completion progress
- List status events in timeline

---

## Common Machine Types

For the machine type dropdown, common options:

**E2 Series** (Cost-optimized):
- `e2-micro` - 0.25-2 vCPU, 1 GB
- `e2-small` - 0.5-2 vCPU, 2 GB
- `e2-medium` - 1-2 vCPU, 4 GB
- `e2-standard-2` - 2 vCPU, 8 GB
- `e2-standard-4` - 4 vCPU, 16 GB
- `e2-standard-8` - 8 vCPU, 32 GB

**N1 Series** (Balanced):
- `n1-standard-1` - 1 vCPU, 3.75 GB
- `n1-standard-2` - 2 vCPU, 7.5 GB
- `n1-standard-4` - 4 vCPU, 15 GB
- `n1-highmem-2` - 2 vCPU, 13 GB
- `n1-highcpu-4` - 4 vCPU, 3.6 GB

**N2 Series** (Higher performance):
- `n2-standard-2` - 2 vCPU, 8 GB
- `n2-standard-4` - 4 vCPU, 16 GB
- `n2-highmem-4` - 4 vCPU, 32 GB

---

## Error Handling

Common validation errors the backend might return:

| Error Code | Message | Frontend Fix |
|------------|---------|--------------|
| 400 | Invalid priority value | Ensure 0-100 range |
| 400 | Missing required field: runnables | Add at least one runnable |
| 400 | Invalid duration format | Use format: "3600s" |
| 403 | Permission denied | Check service account permissions |
| 404 | Machine type not found | Verify machine type exists in region |
| 409 | Job already exists | Generate new job name |

## References

- [GCP Batch API Documentation](https://cloud.google.com/batch/docs)
- API Version: v1.13.0
