# Makefile for kubectl-nim plugin

# Force remove all terminating namespaces with no pods
# for ns in $(kubectl get ns --no-headers | grep Terminating | awk '{print $1}'); do   echo "Force removing finalizers from namespace: $ns";   kubectl get namespace "$ns" -o json |     jq '.spec.finalizers = []' |     kubectl replace --raw "/api/v1/namespaces/$ns/finalize" -f -; done


# Use this before every command to use existing image pull secrets (no hf-api-secret yet though)
# E2E_USE_REAL_SECRETS=true E2E_SECRET_NAMESPACE=your-namespace

.PHONY: all build test unit-test e2e-test clean install deps

# Variables
BINARY_NAME=kubectl-nim
GO=go
GINKGO=ginkgo
MAIN_PATH=cmd/kubectl-nim.go

# Default target
all: build

# Build the plugin
build:
	@echo "Building $(BINARY_NAME)..."
	$(GO) build -o $(BINARY_NAME) $(MAIN_PATH)

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GO) mod download
	$(GO) mod tidy

# Install test dependencies
test-deps:
	@echo "Installing test dependencies..."
	$(GO) install github.com/onsi/ginkgo/v2/ginkgo@latest
	$(GO) get github.com/onsi/ginkgo/v2
	$(GO) get github.com/onsi/gomega

# Run all tests
test: unit-test e2e-test

# Run unit tests
unit-test:
	@echo "Running unit tests..."
	$(GO) test -v -race ./pkg/...

# Run e2e tests
e2e-test: build test-deps
	@echo "Running e2e tests..."
	@PATH=$(PWD):$(HOME)/go/bin:$$PATH $(GINKGO) test/e2e/

# Run e2e tests with verbose output
e2e-test-verbose: build test-deps
	@echo "Running e2e tests (verbose)..."
	@PATH=$(PWD):$(HOME)/go/bin:$$PATH $(GINKGO) -v test/e2e/

# Run e2e tests in parallel
e2e-test-parallel: build test-deps
	@echo "Running e2e tests (parallel)..."
	@PATH=$(PWD):$(HOME)/go/bin:$$PATH $(GINKGO) -p test/e2e/

# Run specific e2e test
e2e-test-focus: build test-deps
	@echo "Running focused e2e tests..."
	@if [ -z "$(FOCUS)" ]; then \
		echo "Usage: make e2e-test-focus FOCUS=\"test pattern\""; \
		exit 1; \
	fi
	@PATH=$(PWD):$(HOME)/go/bin:$$PATH $(GINKGO) --focus="$(FOCUS)" test/e2e/

# Run specific e2e test with verbose output
e2e-test-focus-verbose: build test-deps
	@echo "Running focused e2e tests..."
	@if [ -z "$(FOCUS)" ]; then \
		echo "Usage: make e2e-test-focus FOCUS=\"test pattern\""; \
		exit 1; \
	fi
	@PATH=$(PWD):$(HOME)/go/bin:$$PATH $(GINKGO) --focus="$(FOCUS)" -v test/e2e/


# Install the plugin to system PATH
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	@sudo cp $(BINARY_NAME) /usr/local/bin/
	@echo "$(BINARY_NAME) installed successfully!"

# Install to user's kubectl plugin directory
install-user: build
	@echo "Installing $(BINARY_NAME) to ~/.krew/bin..."
	@mkdir -p ~/.krew/bin
	@cp $(BINARY_NAME) ~/.krew/bin/
	@echo "$(BINARY_NAME) installed to user directory!"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@rm -rf test/e2e/testdata/*.log
	@rm -rf test/e2e/testdata/logs/

# Check prerequisites
check-prereqs:
	@echo "Checking prerequisites..."
	@command -v kubectl >/dev/null 2>&1 || { echo "kubectl is required but not installed."; exit 1; }
	@kubectl cluster-info >/dev/null 2>&1 || { echo "Cannot connect to Kubernetes cluster."; exit 1; }
	@kubectl api-resources | grep -q nimcaches || echo "WARNING: NIM operator CRDs not found"

# Help
help:
	@echo "Available targets:"
	@echo "  make build              - Build the kubectl-nim plugin"
	@echo "  make test               - Run all tests (unit + e2e)"
	@echo "  make unit-test          - Run unit tests only"
	@echo "  make e2e-test           - Run e2e tests"
	@echo "  make e2e-test-verbose   - Run e2e tests with verbose output"
	@echo "  make e2e-test-parallel  - Run e2e tests in parallel"
	@echo "  make e2e-test-focus FOCUS=\"pattern\" - Run specific e2e tests"
	@echo "  make install            - Install plugin to /usr/local/bin"
	@echo "  make install-user       - Install plugin to user directory"
	@echo "  make clean              - Clean build artifacts"
	@echo "  make check-prereqs      - Check prerequisites"
	@echo "  make deps               - Install Go dependencies"
	@echo "  make test-deps          - Install test dependencies"
