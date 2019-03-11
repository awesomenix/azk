
# Image URL to use all building/pushing image targets
IMG ?= quay.io/awesomenix/azkube-manager:latest
RELEASE_LABEL ?= $(git describe --tags)

all: test manager

# Run tests
test: generate fmt vet manifests
	go test ./pkg/... ./cmd/... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager github.com/awesomenix/azkube/cmd/manager

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./cmd/manager/main.go

# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kubectl apply -f config/crds
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests:
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

# Generate code
generate:
	go get -u github.com/shurcooL/vfsgen/cmd/vfsgendev
	go generate ./pkg/... ./cmd/...
	go generate -tags dev ./assets/...
	kustomize build config/default > config/deployment/azkube-deployment.yaml

# Build the docker image
docker-build: test
	docker build . -t ${IMG}

# Push the docker image
docker-push: docker-build
	docker push ${IMG}

release:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o bin/linux/azkube cmd/azkube/main.go
	tar -czf bin/azkube-linux-${RELEASE_LABEL}.tar.gz bin/linux/azkube
	CGO_ENABLED=0 GOARCH=amd64 go build -a -o bin/osx/azkube cmd/azkube/main.go
	tar -czf bin/azkube-osx-${RELEASE_LABEL}.tar.gz bin/osx/azkube