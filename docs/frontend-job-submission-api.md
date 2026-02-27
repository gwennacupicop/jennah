# Frontend Job Submission API Reference

## Overview

All requests go through the **Gateway** using the [ConnectRPC](https://connectrpc.com/) protocol, which is compatible with standard HTTP/1.1 and HTTP/2. The gateway handles authentication, tenant resolution, and worker routing transparently.

**Gateway base URL**: `http://<gateway-host>:8080`

---

## Authentication

Every request **must** include the following OAuth headers. These are set by the auth layer before the request reaches the gateway — the frontend is responsible for forwarding them.

| Header             | Required | Example            |
| ------------------ | -------- | ------------------ |
| `X-OAuth-Email`    | Yes      | `user@example.com` |
| `X-OAuth-UserId`   | Yes      | `1234567890`       |
| `X-OAuth-Provider` | Yes      | `google`           |

If any of these headers are missing, the gateway returns `401 Unauthenticated`.

> The gateway automatically resolves (or creates) the tenant from these headers. The frontend does not need to manage tenant IDs.

---

## Endpoints

### 1. Submit a Job

**POST** `/jennah.v1.DeploymentService/SubmitJob`

Submits a containerized workload to GCP Batch. The job is queued, and a job ID is returned immediately.

#### Request Headers

```
Content-Type: application/json
X-OAuth-Email: user@example.com
X-OAuth-UserId: 1234567890
X-OAuth-Provider: google
```

#### Request Body

```json
{
  "image_uri": "string",
  "env_vars": {
    "KEY": "VALUE"
  },
  "resource_profile": "string",
  "resource_override": {
    "cpu_millis": 0,
    "memory_mib": 0,
    "max_run_duration_seconds": 0
  }
}
```

#### Fields

| Field               | Type                  | Required | Description                                                                                                      |
| ------------------- | --------------------- | -------- | ---------------------------------------------------------------------------------------------------------------- |
| `image_uri`         | `string`              | **Yes**  | Container image to run. Must be a fully qualified URI (e.g. `gcr.io/project/image:tag`).                         |
| `env_vars`          | `map<string, string>` | No       | Environment variables injected into the container at runtime.                                                    |
| `resource_profile`  | `string`              | No       | Named resource preset. One of: `small`, `medium`, `large`, `xlarge`. Defaults to `medium` when omitted or empty. |
| `resource_override` | `object`              | No       | Fine-grained resource values. Any zero/omitted field falls back to the resolved preset. See table below.         |

#### `resource_override` Fields

| Field                      | Type    | Unit        | Description                                          |
| -------------------------- | ------- | ----------- | ---------------------------------------------------- |
| `cpu_millis`               | `int64` | milli-cores | CPU allocation. `1000` = 1 vCPU. `0` = use preset.   |
| `memory_mib`               | `int64` | MiB         | Memory allocation. `4096` = 4 GiB. `0` = use preset. |
| `max_run_duration_seconds` | `int64` | seconds     | Job timeout. `3600` = 1 hour. `0` = use preset.      |

#### Resource Presets

| Profile              | CPU            | Memory             | Max Duration   |
| -------------------- | -------------- | ------------------ | -------------- |
| `small`              | 1000m (1 vCPU) | 2048 MiB (2 GiB)   | 1800s (30 min) |
| `medium` _(default)_ | 2000m (2 vCPU) | 4096 MiB (4 GiB)   | 3600s (1 hr)   |
| `large`              | 4000m (4 vCPU) | 8192 MiB (8 GiB)   | 7200s (2 hr)   |
| `xlarge`             | 8000m (8 vCPU) | 16384 MiB (16 GiB) | 14400s (4 hr)  |

#### Override Merge Behaviour

- If only `resource_profile` is provided → all values come from the preset.
- If only `resource_override` is provided with partial fields → missing fields come from the default (`medium`) preset.
- If both are provided → preset sets the base, override fields (non-zero) take precedence.
- If neither is provided → `medium` defaults apply.

#### Example Requests

**Minimal (default resources)**

```json
{
  "image_uri": "gcr.io/my-project/my-worker:v1.2.0"
}
```

**Named preset**

```json
{
  "image_uri": "gcr.io/my-project/my-worker:v1.2.0",
  "env_vars": {
    "DB_HOST": "10.0.0.1",
    "LOG_LEVEL": "debug"
  },
  "resource_profile": "large"
}
```

**Partial override on top of a preset** (use `large` preset but cap timeout at 1 hr)

```json
{
  "image_uri": "gcr.io/my-project/my-worker:v1.2.0",
  "resource_profile": "large",
  "resource_override": {
    "max_run_duration_seconds": 3600
  }
}
```

**Full custom override** (ignores presets entirely)

```json
{
  "image_uri": "gcr.io/my-project/my-worker:v1.2.0",
  "resource_override": {
    "cpu_millis": 3000,
    "memory_mib": 6144,
    "max_run_duration_seconds": 5400
  }
}
```

#### Response Body

```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "RUNNING",
  "worker_assigned": "10.146.0.26"
}
```

| Field             | Type     | Description                                                           |
| ----------------- | -------- | --------------------------------------------------------------------- |
| `job_id`          | `string` | UUID of the created job. Use this to reference the job in `ListJobs`. |
| `status`          | `string` | Initial job status. Typically `RUNNING` or `SCHEDULED`.               |
| `worker_assigned` | `string` | Internal IP of the worker that handled the submission.                |

---

### 2. List Jobs

**POST** `/jennah.v1.DeploymentService/ListJobs`

Returns all jobs for the authenticated user's tenant.

#### Request Headers

```
Content-Type: application/json
X-OAuth-Email: user@example.com
X-OAuth-UserId: 1234567890
X-OAuth-Provider: google
```

#### Request Body

```json
{}
```

#### Response Body

```json
{
  "jobs": [
    {
      "job_id": "550e8400-e29b-41d4-a716-446655440000",
      "tenant_id": "a1b2c3d4-...",
      "image_uri": "gcr.io/my-project/my-worker:v1.2.0",
      "status": "RUNNING",
      "created_at": "2026-02-18T10:30:00Z"
    }
  ]
}
```

#### Job Status Values

| Status      | Meaning                                      |
| ----------- | -------------------------------------------- |
| `PENDING`   | Job accepted, not yet submitted to GCP Batch |
| `SCHEDULED` | GCP Batch is allocating resources            |
| `RUNNING`   | Container is actively executing              |
| `COMPLETED` | Job finished successfully                    |
| `FAILED`    | Job encountered an error                     |
| `CANCELLED` | Job was cancelled                            |

---

### 3. Get Current Tenant

**POST** `/jennah.v1.DeploymentService/GetCurrentTenant`

Returns the tenant record for the authenticated user. Useful for displaying account info or confirming the user is registered.

#### Request Headers

```
Content-Type: application/json
X-OAuth-Email: user@example.com
X-OAuth-UserId: 1234567890
X-OAuth-Provider: google
```

#### Request Body

```json
{}
```

#### Response Body

```json
{
  "tenant_id": "a1b2c3d4-e5f6-...",
  "user_email": "user@example.com",
  "oauth_provider": "google",
  "created_at": "2026-02-01T08:00:00Z"
}
```

---

## Error Responses

All errors follow ConnectRPC error format:

```json
{
  "code": "invalid_argument",
  "message": "image_uri is required"
}
```

| HTTP Status | ConnectRPC Code    | When it occurs                                         |
| ----------- | ------------------ | ------------------------------------------------------ |
| 400         | `invalid_argument` | Missing `image_uri` or malformed request               |
| 401         | `unauthenticated`  | Missing or invalid OAuth headers                       |
| 500         | `internal`         | Worker failure, database error, or no available worker |

---

## Health Check

**GET** `/health`

Returns `200 OK` with body `ok` when the gateway is running. No authentication required. Use this for liveness/readiness probes.

---

## Notes

- The gateway auto-creates a tenant on first request — no registration step is needed.
- The same user (same `X-OAuth-UserId` + `X-OAuth-Provider`) is always routed to the same worker via consistent hashing.
- `ListJobs` only returns jobs belonging to the authenticated user's tenant — users cannot see each other's jobs.
