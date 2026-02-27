# Artifact Registry Configuration

## Repository Information

**Project:** labs-169405  
**Registry:** asia.gcr.io/labs-169405  
**Format:** Docker (GCR)  
**Region:** Asia (multiple regions in Asia)  

**Note:** Using Asia registry to match GCP Batch region (asia-northeast1)

## Image Paths

### Gateway (Cloud Run)
```
asia.gcr.io/labs-169405/jennah-gateway:latest
asia.gcr.io/labs-169405/jennah-gateway:<version>
```

### Worker (GCE VM)
```
asia.gcr.io/labs-169405/jennah-worker:latest
asia.gcr.io/labs-169405/jennah-worker:<version>
```

## Usage

### Build and Push Gateway
```bash
cd cmd/gateway
docker build -t asia.gcr.io/labs-169405/jennah-gateway:latest -f ../../Dockerfile.gateway .
docker push asia.gcr.io/labs-169405/jennah-gateway:latest
```

### Build and Push Worker
```bash
cd cmd/worker
docker build -t asia.gcr.io/labs-169405/jennah-worker:latest .
docker push asia.gcr.io/labs-169405/jennah-worker:latest
```

### Authenticate Docker
```bash
gcloud auth configure-docker asia.gcr.io
```

## Notes
- Reusing existing company-wide GCR repository
- Images will be automatically accessible across GCP services (Cloud Run, GCE, GCP Batch)
- Service account `gcp-sa-dev-interns@labs-169405.iam.gserviceaccount.com` needs `roles/artifactregistry.reader` to pull images
