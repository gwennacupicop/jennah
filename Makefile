.PHONY: build gw-docker-build gw-docker-run gw-docker-push gw-deploy clean generate

PROJECT_ID = labs-169405
IMAGE_NAME = jennah-gateway
IMAGE_TAG = latest
AR_IMAGE = asia-docker.pkg.dev/$(PROJECT_ID)/asia.gcr.io/$(IMAGE_NAME):$(IMAGE_TAG)
REGION = asia-northeast1
VPC_CONNECTOR = cr-vpccon-tokyo-dev
WORKER_IP = 10.146.0.26

# Generate codes from proto changes
generate:
	buf generate --exclude-path vendor/

# Build gateway binary 
gw-build:
	cd cmd/gateway && go build -o ../../bin/gateway main.go

# Build the gateway Docker image
gw-docker-build:
	docker build -f Dockerfile.gateway -t $(IMAGE_NAME):$(IMAGE_TAG) .
	docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(AR_IMAGE)

# Run the gateway Docker container locally
gw-docker-run:
	docker run --rm -p 8080:8080 $(IMAGE_NAME):$(IMAGE_TAG)

# Push the gateway Docker image to Artifact Registry
gw-docker-push:
	gcloud auth configure-docker asia-docker.pkg.dev
	docker push $(AR_IMAGE)


# Deploy the gateway Docker image to Cloud Run with VPC egress
gw-deploy:
	gcloud run deploy $(IMAGE_NAME) \
	  --image $(AR_IMAGE) \
	  --platform managed \
	  --region $(REGION) \
	  --project $(PROJECT_ID) \
	  --port 8080 \
	  --allow-unauthenticated \
	  --vpc-egress all-traffic \
	  --vpc-connector $(VPC_CONNECTOR)


# Get Cloud Run service URL
gw-url:
	@gcloud run services describe $(IMAGE_NAME) \
	  --region $(REGION) \
	  --project $(PROJECT_ID) \
	  --format='value(status.url)'

# Test health endpoint
gw-test-health:
	@echo "Testing health endpoint..."
	@curl -s $$(gcloud run services describe $(IMAGE_NAME) \
	  --region $(REGION) \
	  --project $(PROJECT_ID) \
	  --format='value(status.url)')/health


clean:
	rm -rf bin/
	docker rmi $(IMAGE_NAME):$(IMAGE_TAG) 2>/dev/null || true
	docker rmi $(AR_IMAGE) 2>/dev/null || true