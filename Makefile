.PHONY: build gw-docker-build gw-docker-run gw-docker-push gw-deploy clean generate

PROJECT_ID = labs-169405
IMAGE_NAME = jennah-gateway
IMAGE_TAG = latest
AR_IMAGE = asia-docker.pkg.dev/$(PROJECT_ID)/asia.gcr.io/$(IMAGE_NAME):$(IMAGE_TAG)

generate:
	buf generate --exclude-path vendor/

build:
	cd cmd/gateway && go build -o ../../bin/gateway main.go

gw-docker-build:
	docker build -f Dockerfile.gateway -t $(IMAGE_NAME):$(IMAGE_TAG) .
	docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(AR_IMAGE)

gw-docker-run:
	docker run --rm -p 8080:8080 $(IMAGE_NAME):$(IMAGE_TAG)

gw-docker-push:
	gcloud auth configure-docker asia-docker.pkg.dev
	docker push $(AR_IMAGE)

gw-deploy:
	gcloud run deploy $(IMAGE_NAME) \
	  --image $(AR_IMAGE) \
	  --platform managed \
	  --region asia-northeast1 \
	  --port 8080

clean:
	rm -rf bin/
	docker rmi $(IMAGE_NAME):$(IMAGE_TAG) 2>/dev/null || true
	docker rmi $(AR_IMAGE) 2>/dev/null || true