## Overview

`jennah` is an opinionated, workload deployment platform for the cloud. Internal to Alphaus.


### Development guidelines

#### Generate code from proto files:
```bash
   $ make generate
```

#### Build the gateway binary and run:
```bash
  $ make gw-build
  $ ./bin/gateway -h
```
#### Build gateway Docker image:
```bash
   $ make gw-docker-build
```
#### Run gateway container:
```bash
   $ make gw-docker-run
```
#### Push gateway to registry:
```bash
   $ make gw-docker-push
```

#### Deploy gateway to Cloud Run:
```bash
   $ make gw-deploy
```

#### Get gateway service URL:
```bash
   $  make gw-url
```

#### Test gateway health:
```bash
   $ make gw-test-health
```
#### Notes:
Make sure to run the command below everytime you have made changes especially when adding/removing packages.
```bash
  $ go mod vendor && go mod tidy
```
 