# Jennah CLI

A command-line interface for submitting and managing batch jobs on the Jennah platform. All commands communicate through the Jennah Gateway API.

---

## Prerequisites

- [Go 1.22+](https://go.dev/dl/)
- A Jennah account (email + Google OAuth user ID)

---

## Build

```bash
cd cmd/cli
go build -o jennah .
```

To make `jennah` available system-wide:

```bash
sudo cp jennah /usr/local/bin/jennah
```

---

## Getting Started

### 1. Log in

```bash
jennah login
```

You will be prompted for your **email** and **OAuth user ID**. Credentials are saved locally to `~/.config/jennah/config.json`.

- If you are a new user, you will be asked to confirm registration.
- If you are already logged in, the command will block with a message.

### 2. Verify login

```bash
jennah list
```

If credentials are valid, you will see your jobs (or an empty list).

### 3. Log out

```bash
jennah logout
```

Removes your locally saved credentials.

---

## Commands

### `submit`

Submit a batch job from a JSON file.

```bash
jennah submit job.json
```

Use `--wait` to stream status changes until the job completes:

```bash
jennah submit job.json --wait
```

**Example `job.json`:**

```json
{
  "image_uri": "gcr.io/google-samples/hello-app:1.0",
  "resource_profile": "default",
  "env_vars": {
    "APP_NAME": "hello-world",
    "TEST_ENV": "production"
  }
}
```

| Field | Description |
|-------|-------------|
| `image_uri` | Container image to run (must be accessible to GCP Batch) |
| `resource_profile` | Named resource preset: `small`, `medium`, `large`, `default` |
| `env_vars` | Key-value environment variables passed to the container |

**Example output:**

```
Gateway URL: https://jennah-gateway-...
Resource Profile: default

Request Payload:
{
  "envVars": { ... },
  "imageUri": "gcr.io/...",
  "resourceProfile": "default"
}

Submitting job...
HTTP Status: 200

Response:
{
  "jobId": "9e32129f-d14b-43ad-b655-7769d7c4d398",
  "status": "PENDING"
}

✅ Job submitted successfully!
Job ID: 9e32129f-d14b-43ad-b655-7769d7c4d398

Done!
```

With `--wait`:

```
Streaming status...
==============================
  [13:10:06]  PENDING
  [13:10:11]  PENDING → SCHEDULED
  [13:10:21]  SCHEDULED → RUNNING
==============================
Done!
```

---

### `list`

List all jobs under your account.

```bash
jennah list
```

---

### `get`

Get details of a specific job.

```bash
jennah get <job-id>
```

Output as JSON:

```bash
jennah get <job-id> --output json
```

---

### `delete`

Delete a specific job by ID:

```bash
jennah delete <job-id>
```

Delete all your jobs at once:

```bash
jennah delete --all
```

---

### `tenant`

Manage your tenant account.

```bash
jennah tenant --help
```

---

## Job Status Flow

Jobs transition through the following statuses:

```
PENDING → SCHEDULED → RUNNING → SUCCEEDED
                              → FAILED
                              → CANCELLED
```

---

## Configuration

Credentials are stored at:

```
~/.config/jennah/config.json
```

You can also set credentials via environment variables (useful for scripting):

```bash
export JENNAH_EMAIL=you@example.com
export JENNAH_USER_ID=your-google-oauth-id
```
