# Build the manager binary
ARG GOLANG_VERSION

FROM golang:${GOLANG_VERSION} AS builder
ARG TARGETOS
ARG TARGETARCH

ARG GOPROXY=direct
ENV GOPROXY=$GOPROXY

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

#Install Git
RUN apt-get update && apt-get install -y git

WORKDIR /utils

#Clone the k8s-operator-libs repo
RUN git clone https://github.com/NVIDIA/k8s-operator-libs.git

WORKDIR /utils/k8s-operator-libs

#Checkout the version of the k8s-operator-libs repo that is compatible with the current version of the NIM Operator
RUN git checkout c80af13d73e36e0eedbe9b712efa85cccb64213a

#Build the crd-apply-tool
RUN go build -o crd-apply-tool cmd/apply-crds/main.go

# Use distroless as minimal base image to package the manager binary
FROM nvcr.io/nvidia/distroless/go:v3.1.12

ARG VERSION="unknown"

WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /utils/k8s-operator-libs/crd-apply-tool /usr/local/bin/crd-apply-tool

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
