# E2E Testing Guide

This comprehensive guide covers configuration, usage, and advanced features of the kubectl-nim E2E test suite.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Available Make Targets](#available-make-targets)
3. [Environment Variables](#environment-variables)
4. [Test Execution Modes](#test-execution-modes)
5. [Readiness Verification](#readiness-verification)
6. [Real NGC Credentials](#real-ngc-credentials)
7. [Use Cases and Examples](#use-cases-and-examples)
8. [Troubleshooting](#troubleshooting)
9. [Implementation Details](#implementation-details)

## Quick Start

### Basic Testing (Default)
```bash
# Fast execution with dummy secrets
make e2e-test
```

### Comprehensive Testing
```bash
# Full validation with readiness verification
E2E_VERIFY_READINESS=true make e2e-test
```

### Real Secrets Testing
```bash
# Use real NGC credentials
E2E_USE_REAL_SECRETS=true make e2e-test
```

### Production-Level Testing
```bash
# Complete validation for CI/CD
E2E_VERIFY_READINESS=true E2E_USE_REAL_SECRETS=true make e2e-test
```

## Available Make Targets

The E2E test suite provides several make targets for different testing scenarios:

| Target | Description | Example |
|--------|-------------|---------|
| `make e2e-test` | Run all E2E tests | `make e2e-test` |
| `make e2e-test-verbose` | Run E2E tests with verbose output | `make e2e-test-verbose` |
| `make e2e-test-parallel` | Run E2E tests in parallel | `make e2e-test-parallel` |
| `make e2e-test-focus` | Run specific E2E tests | `make e2e-test-focus FOCUS="create"` |
| `make e2e-test-focus-verbose` | Run specific E2E tests with verbose output | `make e2e-test-focus-verbose FOCUS="nimservice"` |

### Combining Environment Variables with Make Targets

All make targets can be combined with environment variables:

```bash
# Verbose output with readiness verification
E2E_VERIFY_READINESS=true make e2e-test-verbose

# Parallel execution with real secrets
E2E_USE_REAL_SECRETS=true make e2e-test-parallel

# Focused test with custom namespace
E2E_SECRET_NAMESPACE=my-namespace make e2e-test-focus FOCUS="nimcache"
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `E2E_VERIFY_READINESS` | `false` | Enable comprehensive readiness and health verification |
| `E2E_USE_REAL_SECRETS` | `false` | Use real NGC secrets instead of dummy ones |
| `E2E_SECRET_NAMESPACE` | `default` | Namespace containing NGC secrets |

### Setting Environment Variables

```bash
# Single test run
E2E_VERIFY_READINESS=true make e2e-test

# Multiple variables
E2E_VERIFY_READINESS=true E2E_USE_REAL_SECRETS=true make e2e-test

# Export for multiple runs
export E2E_VERIFY_READINESS=true
export E2E_USE_REAL_SECRETS=true
make e2e-test
```

## Test Execution Modes

### 1. Fast Mode (Default)
- **Execution Time**: Seconds to minutes
- **Verification**: Resource creation only
- **Use Case**: Development, quick feedback
- **Command**: `make e2e-test`

**What it does:**
- Creates NIMCache/NIMService resources
- Verifies resources exist in Kubernetes
- Uses dummy NGC secrets
- No deployment health verification

### 2. Comprehensive Mode
- **Execution Time**: 10-15 minutes per resource
- **Verification**: Full deployment readiness
- **Use Case**: CI/CD, production validation
- **Command**: `E2E_VERIFY_READINESS=true make e2e-test`

**What it does:**
- Creates resources and waits for Ready status
- Verifies pod health (no ImagePullBackOff, etc.)
- Monitors deployment progress
- Provides detailed failure information

### 3. Real Secrets Mode
- **Execution Time**: 10+ minutes (model downloads)
- **Verification**: Actual model downloads and caching
- **Use Case**: End-to-end validation
- **Command**: `E2E_USE_REAL_SECRETS=true make e2e-test`

**What it does:**
- Downloads real models from NGC
- Tests actual authentication flows
- Validates storage and caching
- Requires valid NGC credentials

### 4. Production Mode (has not been fully tested)
- **Execution Time**: 15-30 minutes
- **Verification**: Complete end-to-end validation
- **Use Case**: Pre-production testing
- **Command**: `E2E_VERIFY_READINESS=true E2E_USE_REAL_SECRETS=true make e2e-test`

**What it does:**
- Combines real secrets with readiness verification
- Ensures deployments are production-ready
- Validates complete workflow
- Highest confidence level

## Readiness Verification

### Overview

Readiness verification ensures that resources not only get created but also become fully functional and healthy.

### Features

**Resource Status Monitoring:**
- Waits for NIMCache/NIMService to reach `Ready` state
- Fails immediately if resources enter `Failed` state
- Provides detailed error information on failures

**Pod Health Verification:**
- Detects ImagePullBackOff and ErrImagePull
- Identifies CrashLoopBackOff issues
- Catches CreateContainerConfigError
- Monitors for failed pods and non-zero exit codes

**Timeouts and Intervals:**
- NIMCache readiness: 10 minutes with 10-second intervals
- NIMService readiness: 15 minutes with 15-second intervals
- Pod health checks: 5 minutes with 10-second intervals
- Resource existence: 30 seconds with 2-second intervals

### Implementation

The readiness verification is controlled by the `shouldWaitForReadiness()` function:

```go
if shouldWaitForReadiness() {
    waitForNIMCacheReady(namespace, nimCacheName)
} else {
    // Basic wait for resource existence only
    Eventually(func() error {
        cmd := exec.Command("kubectl", "get", "nimcache", name, "-n", ns)
        return cmd.Run()
    }, 30*time.Second, 2*time.Second).Should(Succeed())
}
```

### Benefits

**Fast Mode Benefits:**
- Quick developer feedback
- Faster CI/CD pipelines for basic validation
- Suitable for unit-level testing

**Comprehensive Mode Benefits:**
- Production-ready validation
- Catches deployment issues early
- Ensures end-to-end functionality
- Higher confidence in deployments

## Real NGC Credentials

### Prerequisites

**1. Valid NGC API Key Secret:**
```bash
kubectl create secret generic ngc-api-secret \
  --from-literal=NGC_API_KEY=<your-real-ngc-api-key> \
  -n default
```

**2. Valid Docker Registry Secret:**
```bash
kubectl create secret docker-registry ngc-secret \
  --docker-server=nvcr.io \
  --docker-username='$oauthtoken' \
  --docker-password=<your-real-ngc-api-key> \
  -n default
```

### Considerations

- **Download Times**: Real model downloads can take 10+ minutes for large models
- **Storage**: Ensure sufficient storage for model caching
- **Network**: Requires good connectivity to NVIDIA's servers
- **Costs**: Be aware of data transfer costs

### Secret Management

**Copy from Custom Namespace:**
```bash
export E2E_SECRET_NAMESPACE=my-namespace
export E2E_USE_REAL_SECRETS=true
make e2e-test
```

**Verify Secrets Exist:**
```bash
kubectl get secrets -n default | grep ngc
kubectl get secret ngc-api-secret -n default -o yaml
kubectl get secret ngc-secret -n default -o yaml
```

## Use Cases and Examples

### Development Workflow
```bash
# Quick validation during development
make e2e-test

# Run tests in parallel for faster execution
make e2e-test-parallel

# Focus on specific functionality
make e2e-test-focus FOCUS="create"
make e2e-test-focus FOCUS="create nimservice"

# Focus on specific functionality with verbose output
make e2e-test-focus-verbose FOCUS="create"
```

### CI/CD Pipeline
```bash
# Fast smoke test
make e2e-test

# Full validation before deployment
E2E_VERIFY_READINESS=true E2E_USE_REAL_SECRETS=true make e2e-test

# Specific test with real credentials
E2E_USE_REAL_SECRETS=true make e2e-test-focus FOCUS="basic nimcache"
```

### Manual Testing
```bash
# Test with verbose output
export E2E_VERBOSE=true
export E2E_VERIFY_READINESS=true
make e2e-test

# Production-like testing
export E2E_VERIFY_READINESS=true
export E2E_USE_REAL_SECRETS=true
export E2E_SECRET_NAMESPACE=production-secrets
make e2e-test
```

### Debugging Issues
```bash
# Enable all debug options
export E2E_VERBOSE=true
export E2E_VERIFY_READINESS=true
export E2E_USE_REAL_SECRETS=true
make e2e-test
```

## Troubleshooting

### Common Issues

**1. Secret Not Found Errors:**
```bash
# Check if secrets exist
kubectl get secrets -n default | grep ngc

# Verify secret contents
kubectl get secret ngc-api-secret -n default -o jsonpath='{.data.NGC_API_KEY}' | base64 -d
```

**2. Readiness Timeouts:**
```bash
# Monitor test progress
kubectl get pods -A | grep test-nim
kubectl logs -f <pod-name> -n <test-namespace>

# Check resource status
kubectl get nimcache -A
kubectl get nimservice -A
kubectl describe nimcache <name> -n <namespace>
```

**3. Test Cleanup Hanging:**
Tests can hang during cleanup when using real secrets due to:
- PVCs with finalizers from model downloads
- Long-running model download jobs
- NIM operator finalizers on resources

```bash
# If tests hang during cleanup, check what's blocking namespace deletion
kubectl get all -n <test-namespace>
kubectl get pvc -n <test-namespace>
kubectl describe namespace <test-namespace>

# Force cleanup if needed (be careful in production!)
kubectl patch namespace <test-namespace> -p '{"metadata":{"finalizers":null}}' --type=merge
kubectl delete namespace <test-namespace> --force --timeout=30s
```

**4. Pod Health Issues:**
```bash
# Check pod status
kubectl get pods -n <test-namespace>
kubectl describe pod <pod-name> -n <test-namespace>

# Check events
kubectl get events -n <test-namespace> --sort-by=.metadata.creationTimestamp
```

**5. Image Pull Problems:**
```bash
# Verify image pull secrets
kubectl get secret ngc-secret -n <namespace> -o yaml

# Test manual image pull
kubectl run test-pod --image=nvcr.io/nim/meta/llama-3.1-8b-instruct:latest \
  --image-pull-policy=Always --restart=Never -n <namespace>
```

### Debug Commands

**Monitor Test Execution:**
```bash
# Watch all test resources
watch kubectl get nimcache,nimservice,pods -A

# Follow logs
kubectl logs -f <test-pod> -n <test-namespace>

# Check job status
kubectl get jobs -n <test-namespace>
kubectl describe job <job-name> -n <test-namespace>
```

**Resource Investigation:**
```bash
# Detailed resource status
kubectl describe nimcache <name> -n <namespace>
kubectl describe nimservice <name> -n <namespace>

# Pod-level debugging
kubectl get pods -l nimcache=<name> -n <namespace>
kubectl logs <pod-name> -n <namespace>
kubectl exec -it <pod-name> -n <namespace> -- /bin/bash
```

## Implementation Details

### Test Structure

The E2E tests are organized as follows:

**Test Files:**
- `create_test.go` - Resource creation tests with optional readiness verification
- `support.go` - Helper functions and utilities
- `delete_test.go` - Resource deletion tests
- `get_test.go` - Resource retrieval tests
- `status_test.go` - Status checking tests

**Key Functions:**
- `shouldWaitForReadiness()` - Controls readiness verification
- `waitForNIMCacheReady()` - NIMCache readiness verification
- `waitForNIMServiceReady()` - NIMService readiness verification
- `waitForPodsHealthy()` - Pod health verification
- `deployTestNIMCache()` - NIMCache deployment helper
- `deployTestNIMService()` - NIMService deployment helper

### Backwards Compatibility

The implementation maintains 100% backwards compatibility:
- Existing test commands work unchanged
- Default behavior remains fast execution
- No breaking changes to existing workflows
- All environment variables are optional

### Test Coverage

**NIMCache Tests:**
- Basic nimcache creation
- NIMCache with PVC configuration
- NIMCache with custom resources
- Duplicate resource handling
- Missing parameter validation

**NIMService Tests:**
- Basic nimservice creation
- Custom replicas configuration
- Autoscaling configuration
- Custom port and service type
- Environment variables
- Duplicate resource handling
- Invalid NIMCache references

### Performance Considerations

**Fast Mode (Default):**
- Test execution: < 5 minutes
- Resource verification: Basic existence check
- Suitable for: Development, quick feedback

**Comprehensive Mode:**
- Test execution: 15-30 minutes
- Resource verification: Full readiness and health
- Suitable for: CI/CD, production validation

**Memory and Storage:**
- Dummy mode: Minimal resource usage
- Real secrets mode: Requires storage for model downloads
- Consider cluster capacity for multiple concurrent tests

This guide provides comprehensive coverage of the E2E testing capabilities, enabling both quick development cycles and thorough production validation.
