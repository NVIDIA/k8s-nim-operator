# Build the manager binary
FROM golang:1.23 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
COPY vendor/ vendor/

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# Install curl
RUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*

# Install kubectl for must-gather and helm upgrades hooks
RUN if [ "$TARGETARCH" = "x86_64" ]; then OS_ARCH="amd64"; elif [ "$TARGETARCH" = "aarch64" ]; then OS_ARCH="arm64"; else OS_ARCH="$TARGETARCH"; fi \
    && curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/${OS_ARCH}/kubectl \
    && chmod +x ./kubectl

# Use distroless as minimal base image to package the manager binary
FROM nvcr.io/nvidia/distroless/go:v3.1.3

ARG VERSION="unknown"

WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/kubectl /usr/local/bin/kubectl

# Add CRD resource into the image for helm upgrades
COPY deployments/helm/k8s-nim-operator/crds/ /opt/nim-operator/crds/

USER 65532:65532

COPY manifests/ manifests/

LABEL io.k8s.display-name="NVIDIA NIM Operator"
LABEL name="NVIDIA NIM Operator"
LABEL vendor="NVIDIA"
LABEL version="${VERSION}"
LABEL release="N/A"
LABEL summary="Deployment and Management of NVIDIA NIM and NeMo Microservices"
LABEL description="See summary"

COPY LICENSE.md /licenses

ENTRYPOINT ["/manager"]
