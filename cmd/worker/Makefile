.PHONY: worker-build worker-docker-build worker-docker-push worker-clean

PROJECT_ID = labs-169405
IMAGE_NAME = jennah-worker
IMAGE_TAG = latest
AR_IMAGE = asia-docker.pkg.dev/$(PROJECT_ID)/asia.gcr.io/$(IMAGE_NAME):$(IMAGE_TAG)
REGION = asia-northeast1

# Build worker binary
worker-build:
	go build -o ../../bin/worker main.go

# Build the worker Docker image
worker-docker-build:
	docker build -f ../../Dockerfile.worker -t $(IMAGE_NAME):$(IMAGE_TAG) ../..
	docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(AR_IMAGE)

# Push the worker Docker image to Artifact Registry
worker-docker-push:
	gcloud auth configure-docker asia-docker.pkg.dev
	docker push $(AR_IMAGE)

# Clean up
worker-clean:
	rm -f ../../bin/worker
	docker rmi $(IMAGE_NAME):$(IMAGE_TAG) 2>/dev/null || true
	docker rmi $(AR_IMAGE) 2>/dev/null || true