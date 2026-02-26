# GCP Batch API Field Mapping Reference

This document maps all fields from `JobConfig` to the final GCP Batch API (`batchpb`) structures. Use this when troubleshooting GCP Batch job submissions or understanding how Jennah translates user inputs to GCP Batch configurations.

---

## Overview

```
JobConfig (Jennah)
    ↓
batchpb.CreateJobRequest
    ├─ Parent: "projects/{project}/locations/{region}"
    ├─ JobId: "{provider-compatible-id}"
    ├─ RequestId: "{idempotency-key}"
    └─ Job: batchpb.Job
```

---

## batchpb.CreateJobRequest Mapping

### Required Fields

#### `Parent` (string)

- **Value**: `projects/{gcp_project}/locations/{gcp_region}`
- **Hardcoded**: `projects/labs-169405/locations/asia-northeast1`
- **Source**: GCPBatchProvider initialization

#### `JobId` (string)

- **Source**: `JobConfig.JobID`
- **Format**: Must be alphanumeric + hyphens, max 63 chars
- **Generation**: `jennah-{name}` or `jennah-{uuid[:8]}`
- **Example**: `jennah-daily-pipeline` or `jennah-a1b2c3d4`

#### `Job` (\*batchpb.Job)

- **Source**: Built from JobConfig
- **Details**: See [batchpb.Job Mapping](#batchpbjob-mapping) section

### Optional Fields

#### `RequestId` (string)

- **Source**: `JobConfig.RequestID`
- **Format**: UUID format recommended
- **Purpose**: Idempotent deduplication
- **Behavior**: If two requests with same RequestId arrive, GCP returns first result
- **Implementation**: Set to internal `job_id` in worker

---

## batchpb.Job Mapping

### Container Execution

#### TaskGroups[0].TaskSpec.Runnables[0].Container

##### ImageUri (string)

- **Source**: `JobConfig.ImageURI`
- **Type**: Container registry URI
- **Required**: Yes
- **Example**: `gcr.io/my-project/my-app:latest`

##### Commands ([]string)

- **Source**: `JobConfig.Commands`
- **Type**: Array of command arguments
- **Default**: Empty (use CMD from image)
- **Behavior**: Appended to/overrides container's CMD

##### Entrypoint (string)

- **Source**: `JobConfig.ContainerEntrypoint`
- **Type**: ENTRYPOINT override
- **Default**: Empty (use ENTRYPOINT from image)

##### Environment (batchpb.Environment)

- **Source**: `JobConfig.EnvVars`
- **Mapping**:
  ```proto
  Environment {
    Variables: JobConfig.EnvVars  // map[string]string
  }
  ```
- **Default**: Empty map (inherit from image)

---

### Compute Resources

#### TaskGroups[0].TaskSpec.ComputeResource

##### CpuMilli (int64)

- **Source**: `JobConfig.Resources.CPUMillis`
- **Unit**: Milli-cores (1000 = 1 vCPU)
- **Origin**: Resolved from `resource_profile` + `resource_override`
- **Default**: 4000 (4 vCPU, from "medium" profile)

##### MemoryMib (int64)

- **Source**: `JobConfig.Resources.MemoryMiB`
- **Unit**: Mebibytes (1 GiB = 1024 MiB)
- **Origin**: Resolved from `resource_profile` + `resource_override`
- **Default**: 4096 (4 GiB, from "medium" profile)

##### BootDiskMib (int64)

- **Source**: `JobConfig.BootDiskSizeGb * 1024`
- **Unit**: Mebibytes
- **Conversion**: GB → MiB (1 GB = 1024 MiB)
- **Example**: 50 GB → 51200 MiB
- **Note**: Only set if BootDiskSizeGb > 0

---

### Task Execution Control

#### TaskGroups[0].TaskSpec

##### MaxRunDuration (\*durationpb.Duration)

- **Source**: `JobConfig.Resources.MaxRunDurationSeconds`
- **Unit**: Seconds (converted to durationpb.Duration)
- **Origin**: Resolved from `resource_profile` + `resource_override`
- **Default**: 3600 seconds (1 hour, from "medium" profile)
- **Range**: 0 to unbounded
- **Behavior**: Job is killed if it exceeds this duration

##### MaxRetryCount (int32)

- **Source**: `JobConfig.MaxRetryCount`
- **Range**: 0-10
- **Default**: 0 (no retries)
- **Behavior**: GCP automatically retries failed tasks up to N times

---

### Task Group Configuration

#### TaskGroups[0] (batchpb.TaskGroup)

##### TaskCount (int64)

- **Source**: `JobConfig.TaskGroup.TaskCount`
- **Default**: 1 (currently hardcoded for single-task jobs)
- **Future**: Will support N tasks for distributed computing

##### Parallelism (int64)

- **Source**: `JobConfig.TaskGroup.Parallelism`
- **Default**: 0 (unlimited concurrent tasks)
- **Constraint**: 0 or ≤ TaskCount
- **Behavior**: Max number of tasks running simultaneously

##### SchedulingPolicy (batchpb.TaskGroup_SchedulingPolicy enum)

- **Source**: `JobConfig.TaskGroup.SchedulingPolicy` (string)
- **Mapping**:
  - `"IN_ORDER"` → `batchpb.TaskGroup_IN_ORDER`
  - `"AS_SOON_AS_POSSIBLE"` (default) → `batchpb.TaskGroup_AS_SOON_AS_POSSIBLE`
- **Behavior**:
  - `IN_ORDER`: Tasks run sequentially
  - `AS_SOON_AS_POSSIBLE`: Tasks run as soon as resources available

##### TaskCountPerNode (int64)

- **Source**: `JobConfig.TaskGroup.TaskCountPerNode`
- **Default**: 0 (no limit)
- **Behavior**: If set, limit to N tasks per VM (affects packing)
- **Use case**: Control task density on each machine

##### RequireHostsFile (bool)

- **Source**: `JobConfig.TaskGroup.RequireHostsFile`
- **Default**: false
- **Behavior**: If true, populate /etc/hosts with all task IPs
- **Use case**: Multi-VM distributed computing

##### PermissiveSsh (bool)

- **Source**: `JobConfig.TaskGroup.PermissiveSsh`
- **Default**: false
- **Behavior**: If true, enable passwordless SSH between task VMs
- **Use case**: Distributed frameworks (Spark, MPI)

##### RunAsNonRoot (bool)

- **Source**: `JobConfig.TaskGroup.RunAsNonRoot`
- **Default**: false
- **Behavior**: If true, enforce non-root execution via OS Login
- **Use case**: Security hardening

---

### Resource Allocation

#### Job.AllocationPolicy (batchpb.AllocationPolicy)

##### Instances[0] (batchpb.AllocationPolicy_InstancePolicyOrTemplate)

###### InstallGpuDrivers (bool)

- **Source**: `JobConfig.InstallGpuDrivers`
- **Default**: false
- **Behavior**: Auto-install GPU drivers from Google Cloud
- **When to use**: If Accelerators.Type is specified

###### InstallOpsAgent (bool)

- **Source**: `JobConfig.InstallOpsAgent`
- **Default**: false
- **Behavior**: Auto-install Google Cloud Operations Agent for monitoring

###### BlockProjectSshKeys (bool)

- **Source**: `JobConfig.BlockProjectSshKeys`
- **Default**: false
- **Behavior**: Prevent project-level SSH keys from accessing VMs
- **Use case**: Security isolation

###### Policy (batchpb.AllocationPolicy_InstancePolicy)

**MachineType** (string)

- **Source**: `JobConfig.MachineType`
- **Format**: GCP machine type name (e.g., "e2-standard-4")
- **Default**: Empty (GCP auto-selects based on CPU/memory)
- **Validation**: Must be available in job's region (asia-northeast1)
- **Example values**:
  - "e2-standard-4" (4 vCPU, 16 GiB)
  - "n1-standard-16" (16 vCPU, 60 GiB)
  - "e2-highmem-4" (4 vCPU, 32 GiB)

**MinCpuPlatform** (string)

- **Source**: `JobConfig.MinCpuPlatform`
- **Format**: Processor generation name
- **Default**: Empty (any CPU platform acceptable)
- **Examples**: "Intel Cascade Lake", "AMD EPYC Rome", "Intel Skylake"
- **Use case**: Enforce consistent processor generation

**ProvisioningModel** (batchpb.AllocationPolicy_ProvisioningModel enum)

- **Source**: `JobConfig.UseSpotVMs` (bool)
- **Mapping**:
  - `true` → `AllocationPolicy_SPOT` (lower price, can preempt)
  - `false` → `AllocationPolicy_STANDARD` (reliable, higher price)
- **Trade-off**: SPOT is 60-90% cheaper but less reliable

**BootDisk** (batchpb.AllocationPolicy_Disk)

- **Source**: `JobConfig.BootDiskSizeGb`
- **Fields**:
  - `Type`: "pd-standard" (always used for boot disks in Jennah)
  - `SizeGb`: `JobConfig.BootDiskSizeGb`
- **Validation**: 10 ≤ size ≤ 65536
- **Example**: 50 GB boot disk

**Accelerators** ([]batchpb.AllocationPolicy_Accelerator)

- **Source**: `JobConfig.Accelerators`
- **Structure**:
  ```proto
  Accelerators {
    Type: JobConfig.Accelerators.Type    // e.g., "nvidia-tesla-t4"
    Count: JobConfig.Accelerators.Count  // e.g., 2 (for 2 GPUs)
  }
  ```
- **Default**: None (no GPUs unless specified)
- **Common Types**:
  - "nvidia-tesla-t4" (cost-effective)
  - "nvidia-tesla-v100" (high-performance)
  - "tpu-v4" (AI/ML specialized)

##### ServiceAccount (batchpb.ServiceAccount)

- **Source**: `JobConfig.ServiceAccount` (if specified)
- **Fields**:
  - `Email`: `JobConfig.ServiceAccount` (e.g., `my-sa@project.iam.gserviceaccount.com`)
  - `Scopes`: `["https://www.googleapis.com/auth/cloud-platform"]` (hardcoded default)
- **Default**: Uses default Compute Engine service account if not specified

##### Network (batchpb.AllocationPolicy_NetworkPolicy)

- **Condition**: Only set if `NetworkName` or `SubnetworkName` specified
- **Structure**:
  ```proto
  NetworkInterfaces {
    Network: JobConfig.NetworkName
    Subnetwork: JobConfig.SubnetworkName
    NoExternalIpAddress: JobConfig.BlockExternalIP
  }
  ```
- **Use case**: VPC isolation, private IPs only

###### Network (string)

- **Source**: `JobConfig.NetworkName`
- **Format**: `projects/{project}/global/networks/{network}`
- **Default**: Empty (uses default network)

###### Subnetwork (string)

- **Source**: `JobConfig.SubnetworkName`
- **Format**: `projects/{project}/regions/{region}/subnetworks/{subnet}`
- **Default**: Empty (auto-selects subnet in chosen region)

###### NoExternalIpAddress (bool)

- **Source**: `JobConfig.BlockExternalIP`
- **Default**: false (VMs get external IPs)
- **When true**: VMs only get internal IPs (requires NAT/proxy for internet)

##### Location (batchpb.AllocationPolicy_LocationPolicy)

- **Condition**: Only set if `AllowedLocations` specified
- **Structure**:
  ```proto
  Location {
    AllowedLocations: JobConfig.AllowedLocations  // ["us-central1", "us-west1"]
  }
  ```
- **Behavior**: Restricts VM creation to specified regions
- **Use case**: Compliance, latency optimization

---

### Job Metadata

#### Priority (int64)

- **Source**: `JobConfig.Priority`
- **Range**: 0-100 (100 is highest)
- **Default**: 0
- **Behavior**: Higher priority jobs scheduled first (when resources compete)

#### Labels (map[string]string)

- **Source**: `JobConfig.JobLabels`
- **Use case**: Organization, billing tracking, filtering
- **Example**:
  ```json
  {
    "team": "data-science",
    "cost-center": "CC-123",
    "environment": "production"
  }
  ```

#### LogsPolicy (batchpb.LogsPolicy)

- **Destination**: Always set to `batchpb.LogsPolicy_CLOUD_LOGGING`
- **Behavior**: Stream job output to Google Cloud Logging
- **Access**: Via Cloud Logging console or API

---

## Status Mapping

### From GCP to Jennah

| GCP Status                           | Jennah Status | Meaning                        |
| ------------------------------------ | ------------- | ------------------------------ |
| `JobStatus_QUEUED`                   | `PENDING`     | Waiting for resources          |
| `JobStatus_SCHEDULED`                | `SCHEDULED`   | Resources allocated, preparing |
| `JobStatus_RUNNING`                  | `RUNNING`     | Tasks executing                |
| `JobStatus_SUCCEEDED`                | `COMPLETED`   | All tasks succeeded            |
| `JobStatus_FAILED`                   | `FAILED`      | At least one task failed       |
| `JobStatus_CANCELLATION_IN_PROGRESS` | `CANCELLED`   | Cancellation in progress       |
| `JobStatus_DELETION_IN_PROGRESS`     | `CANCELLED`   | Deletion in progress           |
| `JobStatus_CANCELLED`                | `CANCELLED`   | Fully cancelled                |

---

## Field Population Flow

### Minimal Configuration

```
User Input:
  image_uri: "alpine:latest"

JobConfig:
  ImageURI: "alpine:latest"
  Resources: {CPUMillis: 4000, MemoryMiB: 4096, MaxRunDurationSeconds: 3600} // from "medium" preset
  TaskGroup: {TaskCount: 1}

batchpb.Job:
  TaskGroups[0].TaskSpec.Runnables[0].Container.ImageUri: "alpine:latest"
  TaskGroups[0].TaskSpec.ComputeResource: {CpuMilli: 4000, MemoryMib: 4096}
  AllocationPolicy.Instances[0].Policy.ProvisioningModel: STANDARD
  AllocationPolicy.ServiceAccount: null (uses default)
  LogsPolicy.Destination: CLOUD_LOGGING
```

### Full Configuration

```
User Input:
  image_uri: "gcr.io/my-project/ml-app:v1"
  resource_profile: "large"
  resource_override: {memory_mib: 12288}
  machine_type: "n1-standard-8"
  boot_disk_size_gb: 100
  use_spot_vms: true
  service_account: "ml-sa@my-project.iam.gserviceaccount.com"
  commands: ["python", "train.py"]
  env_vars: {CUDA_VISIBLE_DEVICES: "0"}

JobConfig:
  ImageURI: "gcr.io/my-project/ml-app:v1"
  Commands: ["python", "train.py"]
  EnvVars: {CUDA_VISIBLE_DEVICES: "0"}
  Resources: {CPUMillis: 8000, MemoryMiB: 12288, MaxRunDurationSeconds: 7200} // large profile + override
  MachineType: "n1-standard-8"
  BootDiskSizeGb: 100
  UseSpotVMs: true
  ServiceAccount: "ml-sa@my-project.iam.gserviceaccount.com"
  TaskGroup: {TaskCount: 1}

batchpb.Job:
  TaskGroups[0].TaskSpec.Runnables[0].Container:
    ImageUri: "gcr.io/my-project/ml-app:v1"
    Commands: ["python", "train.py"]
    Environment: {Variables: {CUDA_VISIBLE_DEVICES: "0"}}
  TaskGroups[0].TaskSpec.ComputeResource: {CpuMilli: 8000, MemoryMib: 12288, BootDiskMib: 102400}
  TaskGroups[0].TaskSpec.MaxRunDuration: 7200s
  AllocationPolicy.Instances[0].Policy:
    MachineType: "n1-standard-8"
    BootDisk: {Type: "pd-standard", SizeGb: 100}
    ProvisioningModel: SPOT
  AllocationPolicy.ServiceAccount:
    Email: "ml-sa@my-project.iam.gserviceaccount.com"
    Scopes: ["https://www.googleapis.com/auth/cloud-platform"]
  LogsPolicy.Destination: CLOUD_LOGGING
```

---

## Validation & Constraints

### GCP Batch API Constraints

| Field          | Min | Max           | Type                   |
| -------------- | --- | ------------- | ---------------------- |
| JobId          | 1   | 63 chars      | Alphanumeric + hyphens |
| CpuMilli       | 0   | 106 (1M)      | Integer                |
| MemoryMib      | 0   | 1048576 (1TB) | Integer                |
| BootDiskSizeGb | 10  | 65536         | Integer                |
| MaxRunDuration | 0   | unlimited     | Duration               |
| TaskCount      | 1   | 1000000       | Integer                |
| Priority       | 0   | 100           | Integer                |

### Jennah Validation

| Field          | Validation       | Error                       |
| -------------- | ---------------- | --------------------------- |
| ImageUri       | Non-empty        | INVALID_ARGUMENT            |
| BootDiskSizeGb | ≥10 if specified | INVALID_ARGUMENT            |
| MachineType    | Valid GCP type   | INVALID_ARGUMENT (from GCP) |
| ServiceAccount | Valid format     | INVALID_ARGUMENT (from GCP) |

---

## Error Handling

### Common GCP Batch Errors

| Error                                               | Cause                      | Solution                              |
| --------------------------------------------------- | -------------------------- | ------------------------------------- |
| `INVALID_ARGUMENT: invalid machine type`            | MachineType not in region  | Choose valid type for asia-northeast1 |
| `PERMISSION_DENIED: service account cannot be used` | SA doesn't have permission | Grant required IAM roles              |
| `RESOURCE_EXHAUSTED: quota exceeded`                | Region quota hit           | Wait or use different region          |
| `DEADLINE_EXCEEDED: request timeout`                | Large job takes too long   | Increase timeout or split job         |

---

## Performance Characteristics

### Job Submission Latency

- Typical: 1-5 seconds from submit to running
- GCP Batch scheduling: 10-30 seconds
- VM startup: 30-120 seconds (depending on image size)

### Resource Efficiency

- JobConfig size: ~2 KB
- Serialization: Proto binary (~500 bytes)
- GCP API request size: ~2-5 KB

---

## References

- [GCP Batch API - batchpb.Job](https://cloud.google.com/batch/docs/reference/rpc/google.cloud.batch.v1#job)
- [GCP Batch API - CreateJobRequest](https://cloud.google.com/batch/docs/reference/rpc/google.cloud.batch.v1#createjobrequest)
- [GCP Compute Engine Machine Types](https://cloud.google.com/compute/docs/machine-types)
- [GCP Batch Job Configuration](https://cloud.google.com/batch/docs/create-job)
- [Jennah BackendField Reference](./backend-field-reference.md)

---
