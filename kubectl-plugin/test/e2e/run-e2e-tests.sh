#!/bin/bash

# E2E Test Runner for kubectl-nim plugin
# This script sets up and runs the e2e tests

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
PROJECT_ROOT="$SCRIPT_DIR/../.."

echo "=== Setting up E2E tests for kubectl-nim plugin ==="

# Change to project root
cd "$PROJECT_ROOT"

# Check if ginkgo is installed
if ! command -v ginkgo &> /dev/null; then
    echo "Installing Ginkgo test framework..."
    go install github.com/onsi/ginkgo/v2/ginkgo@latest
fi

# Add test dependencies if not present
if ! grep -q "ginkgo" go.mod; then
    echo "Adding test dependencies..."
    go get github.com/onsi/ginkgo/v2
    go get github.com/onsi/gomega
fi

# Build the kubectl-nim plugin
echo "Building kubectl-nim plugin..."
go build -o kubectl-nim cmd/kubectl-nim.go

# Add to PATH if not already there
export PATH="$PROJECT_ROOT:$PATH"

# Check for required prerequisites
echo "Checking prerequisites..."

# Check kubectl
if ! command -v kubectl &> /dev/null; then
    echo "ERROR: kubectl not found in PATH"
    exit 1
fi

# Check Kubernetes cluster connection
if ! kubectl cluster-info &> /dev/null; then
    echo "ERROR: Cannot connect to Kubernetes cluster"
    echo "Please ensure you have a running cluster and proper kubeconfig"
    exit 1
fi

# Check if NIM operator CRDs are installed
if ! kubectl api-resources | grep -q "nimcaches"; then
    echo "WARNING: NIM operator CRDs not found"
    echo "Please ensure the NIM operator is installed in your cluster"
    echo "Continuing anyway..."
fi

# Run the tests
echo "=== Running E2E tests ==="

if [ "$1" == "--verbose" ] || [ "$1" == "-v" ]; then
    ginkgo -v test/e2e/
elif [ "$1" == "--focus" ] && [ -n "$2" ]; then
    ginkgo --focus="$2" test/e2e/
elif [ "$1" == "--parallel" ] || [ "$1" == "-p" ]; then
    ginkgo -p test/e2e/
elif [ -n "$1" ]; then
    # Run specific test file
    ginkgo test/e2e/$1
else
    # Run all tests
    ginkgo test/e2e/
fi

echo "=== E2E tests completed ==="
