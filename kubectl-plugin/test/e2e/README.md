# E2E Tests for kubectl-nim Plugin

This directory contains end-to-end (e2e) tests for the kubectl-nim plugin, following the same pattern as the kubectl-ray plugin tests.

## Test Structure

The e2e tests are organized using the Ginkgo BDD testing framework:

- **kubectl_nim_e2e_suite_test.go** - Test suite initialization
- **support.go** - Helper functions for test setup/teardown
- **create_test.go** - Tests for `kubectl nim create` commands
- **get_test.go** - Tests for `kubectl nim get` commands
- **delete_test.go** - Tests for `kubectl nim delete` commands
- **status_test.go** - Tests for `kubectl nim status` commands
- **log_test.go** - Tests for `kubectl nim log` commands
- **testdata/** - Sample YAML configurations for testing

## Prerequisites

Before running the tests:

1. **Kubernetes Cluster**: You need access to a Kubernetes cluster
2. **NIM Operator**: The NIM operator must be deployed in the cluster
3. **kubectl-nim Plugin**: The plugin must be built and available in your PATH
4. **Ginkgo**: Install the Ginkgo test runner

```bash
go install github.com/onsi/ginkgo/v2/ginkgo@latest
```

## Running the Tests

### Using Make (Recommended):
```bash
cd /home/nvidia/k8s-nim-operator/kubectl-plugin

# Run all e2e tests
make e2e-test

# Run with verbose output
make e2e-test-verbose

# Run tests in parallel
make e2e-test-parallel

# Run specific tests
make e2e-test-focus FOCUS="get nimcache"

# Run all tests (unit + e2e)
make test
```

### Using the test runner script:
```bash
cd /home/nvidia/k8s-nim-operator/kubectl-plugin

# Run all e2e tests
./test/e2e/run-e2e-tests.sh

# Run with verbose output
./test/e2e/run-e2e-tests.sh --verbose

# Run specific tests
./test/e2e/run-e2e-tests.sh --focus "get nimcache"

# Run in parallel
./test/e2e/run-e2e-tests.sh --parallel
```

### Using Ginkgo directly:
```bash
cd /home/nvidia/k8s-nim-operator/kubectl-plugin
ginkgo test/e2e/

# Run specific test files
ginkgo test/e2e/get_test.go

# Run with verbose output
ginkgo -v test/e2e/

# Run specific test descriptions
ginkgo --focus="get nimcache" test/e2e/

# Run tests in parallel
ginkgo -p test/e2e/
```

## Test Patterns

Each test follows a consistent pattern:

1. **Setup**: Creates a unique test namespace and required secrets
2. **Test Execution**: Runs kubectl nim commands and validates output
3. **Cleanup**: Removes all test resources and namespace

### Example Test Structure:
```go
var _ = Describe("Calling kubectl nim `<command>` command", func() {
    var namespace string
    
    BeforeEach(func() {
        namespace = createTestNamespace()
        createNGCSecretForTesting(namespace)
        DeferCleanup(func() {
            cleanupTestResources(namespace)
            deleteTestNamespace(namespace)
        })
    })
    
    It("should <test scenario>", func() {
        // Test implementation
    })
})
```

## Key Testing Features

1. **Namespace Isolation**: Each test runs in its own namespace
2. **Resource Cleanup**: Automatic cleanup using DeferCleanup
3. **Real Integration**: Tests run against actual Kubernetes API
4. **Error Validation**: Tests both success and failure scenarios
5. **Output Validation**: Verifies command output formats (table, JSON, YAML)

## Common Test Scenarios

### Create Tests
- Basic resource creation
- Creation with custom parameters
- Duplicate resource handling
- Missing required parameters

### Get Tests
- Single resource retrieval
- List all resources
- Output format support (JSON, YAML)
- Non-existent resource handling

### Delete Tests
- Single resource deletion
- Multiple resource deletion
- Delete with --all flag
- Force deletion
- Grace period handling

### Status Tests
- Single resource status
- All resources status
- Wide output format
- Watch mode

### Log Tests
- Stream logs from pods
- Collect logs to directory
- Follow logs
- Tail specific number of lines
- All containers logging

## Troubleshooting

### Tests Failing with "not found"
- Ensure the NIM operator CRDs are installed
- Check that the operator is running

### Tests Timing Out
- Increase test timeouts in BeforeEach/AfterEach
- Check cluster resource availability

### Permission Errors
- Ensure kubectl has proper RBAC permissions
- Check namespace creation permissions

## Adding New Tests

When adding new tests:

1. Follow the existing test patterns
2. Use descriptive test names
3. Include both positive and negative test cases
4. Clean up all resources
5. Use Eventually() for async operations
6. Add appropriate timeouts

## CI/CD Integration

These tests can be integrated into CI/CD pipelines:

```yaml
# Example GitHub Actions step
- name: Run E2E Tests
  run: |
    go install github.com/onsi/ginkgo/v2/ginkgo@latest
    ginkgo -v --junit-report=junit.xml test/e2e/
```
