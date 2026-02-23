# Gateway Service

API gateway for Jennah deployment platform. Handles OAuth authentication, tenant management, and request routing to worker nodes.

## Architecture

- Authentication: OAuth headers from oauth2-proxy
- Tenant Management: Spanner database with in-memory caching
- Routing: Consistent hashing to distribute tenants across workers
- Database: Spanner (labs-169405/alphaus-dev/main)

## Local Development

### Prerequisites

- Go 1.23+
- Docker
- GCP credentials for database access

### Build

Build binary:

cd cmd/gateway
go build -o bin/gateway main.go

Run locally:

./bin/gateway serve

### Configuration Flags

--port (default: 8080)
  Server port

--worker-ips (default: 10.128.0.1,10.128.0.2,10.128.0.3)
  Comma-separated list of worker IP addresses

--gcp-project (default: labs-169405)
  GCP project ID

--spanner-instance (default: alphaus-dev)
  Spanner instance name

--spanner-database (default: main)
  Spanner database name

### Environment Variables

GOOGLE_APPLICATION_CREDENTIALS
  Path to service account JSON key file

## Docker

### Build Image

From project root:

docker build -f Dockerfile.gateway -t jennah-gateway:latest .

Using Make:

make gw-docker-build

### Run Locally

Without credentials:

docker run --rm -p 8080:8080 jennah-gateway:latest

With service account credentials:

docker run --rm -p 8080:8080 \
  -v $HOME/jennah-key.json:/key.json:ro \
  -e GOOGLE_APPLICATION_CREDENTIALS=/key.json \
  jennah-gateway:latest

### Push to Registry

docker tag jennah-gateway:latest gcr.io/labs-169405/jennah-gateway:latest
docker push gcr.io/labs-169405/jennah-gateway:latest

Using Make:

make gw-docker-push

### Image Details

- Base: scratch (minimal runtime)
- Size: approximately 28MB
- Architecture: linux/amd64
- Binary: statically linked

## API Endpoints

### GetCurrentTenant

Retrieve tenant information for authenticated user.

curl -X POST http://localhost:8080/jennah.v1.DeploymentService/GetCurrentTenant \
  -H "Content-Type: application/json" \
  -H "X-OAuth-Email: user@example.com" \
  -H "X-OAuth-UserId: oauth-user-123" \
  -H "X-OAuth-Provider: google" \
  -d '{}'

Response:

{
  "tenantId": "uuid",
  "userEmail": "user@example.com",
  "oauthProvider": "google",
  "createdAt": "2026-02-12T10:00:00Z"
}

### SubmitJob

Submit deployment job to worker.

curl -X POST http://localhost:8080/jennah.v1.DeploymentService/SubmitJob \
  -H "Content-Type: application/json" \
  -H "X-OAuth-Email: user@example.com" \
  -H "X-OAuth-UserId: oauth-user-123" \
  -H "X-OAuth-Provider: google" \
  -d '{"imageUri": "gcr.io/project/image:tag", "envVars": {"KEY": "value"}}'

### ListJobs

List jobs for authenticated tenant (forwarded by gateway to the tenant-assigned worker).

curl -X POST http://localhost:8080/jennah.v1.DeploymentService/ListJobs \
  -H "Content-Type: application/json" \
  -H "X-OAuth-Email: user@example.com" \
  -H "X-OAuth-UserId: oauth-user-123" \
  -H "X-OAuth-Provider: google" \
  -d '{}'

### CancelJob

Cancel a tenant job in an active state (gateway forwards request to the tenant-assigned worker).

curl -X POST http://localhost:8080/jennah.v1.DeploymentService/CancelJob \
  -H "Content-Type: application/json" \
  -H "X-OAuth-Email: user@example.com" \
  -H "X-OAuth-UserId: oauth-user-123" \
  -H "X-OAuth-Provider: google" \
  -d '{"jobId": "<job-uuid>"}'

### DeleteJob

Delete a tenant job from the system (gateway forwards request to the tenant-assigned worker).

curl -X POST http://localhost:8080/jennah.v1.DeploymentService/DeleteJob \
  -H "Content-Type: application/json" \
  -H "X-OAuth-Email: user@example.com" \
  -H "X-OAuth-UserId: oauth-user-123" \
  -H "X-OAuth-Provider: google" \
  -d '{"jobId": "<job-uuid>"}'

### Health Check

curl http://localhost:8080/health

Response:

OK

## Implementation

### Tenant Management Flow

1. Extract OAuth headers from request
2. Check in-memory cache for existing tenant
3. Query database if not cached
4. Create new tenant if not found
5. Cache tenant mapping for future requests

### Request Routing

Consistent hashing based on tenant ID ensures:
- Same tenant always routes to same worker
- Even distribution across workers
- Minimal reassignment when workers scale

Gateway forwards job operations (`SubmitJob`, `ListJobs`, `CancelJob`, `DeleteJob`) to the selected worker and uses Spanner directly for tenant lifecycle data.

### Thread Safety

sync.RWMutex protects concurrent access to in-memory tenant cache.

## Database Schema

Tenants table columns:
- TenantId (primary key)
- UserEmail
- OAuthProvider
- OAuthUserId
- CreatedAt
- UpdatedAt
