# Native Model Download Protocol Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Select Retriever's native download-only lifecycle from an authenticated image configuration label and use the matching `/model` layout for caching and serving.

**Architecture:** A focused `internal/imageprotocol` package resolves image config labels across image-index children using Kubernetes pull secrets. NIMCache branches before legacy manifest/profile processing, while Standalone and KServe rendering use the same protocol and model-path helpers.

**Tech Stack:** Go 1.26.3, controller-runtime, Kubernetes core API, go-containerregistry, Ginkgo/Gomega, EnvTest, Docker.

## Global Constraints

- Only exact `com.nvidia.nim.model_download_protocol=native-v1` opts in.
- Successfully inspected absent or unknown values retain legacy behavior.
- Inspection failures and mixed multi-platform protocols fail reconciliation.
- Native cache Jobs force `NIM_ENGINE_MODEL_DOWNLOAD_ONLY=1` and use `/model`.
- Existing/shared PVCs use `/model/<NIMCache-name>`; created PVCs use `/model`.
- User `NIM_ENGINE_MODEL_PATH` overrides the calculated default.
- Native cache Jobs bypass legacy manifests/profiles and default GPU selection.
- No CRD schema changes.
- Produce one final commit only.

---

### Task 1: Image protocol resolver

**Files:**
- Create: `internal/imageprotocol/protocol.go`
- Create: `internal/imageprotocol/registry.go`
- Create: `internal/imageprotocol/registry_test.go`
- Modify: `go.mod`, `go.sum`, `vendor/`

**Interfaces:**
- Produces: `type Protocol string`, `const NativeV1 Protocol`, and
  `type Resolver interface { Resolve(context.Context, string, string, []string) (Protocol, error) }`.
- Produces: `NewRegistryResolver(client.Reader) Resolver`.

- [x] Write table-driven failing tests for exact, absent, unknown, mixed-index,
  malformed-secret, and registry-error behavior using a fake image reader.
- [x] Run `go test ./internal/imageprotocol -count=1` in the Go 1.26.3 container
  and verify missing types/functions fail compilation.
- [x] Implement Kubernetes Docker-config authentication, registry descriptor
  traversal, and all-platform protocol agreement.
- [x] Run the package tests and verify they pass.

### Task 2: Native cache Job and reconciliation branch

**Files:**
- Modify: `internal/controller/nimcache_controller.go`
- Modify: `internal/controller/nimcache_controller_test.go`

**Interfaces:**
- Consumes: `imageprotocol.Resolver` and `imageprotocol.NativeV1`.
- Produces: protocol-aware `reconcileNIMCache`, `reconcileJob`, and
  `constructJob` behavior.

- [x] Add failing controller specs proving native jobs have no command/args,
  force download-only, mount `/model`, calculate dedicated/shared paths,
  preserve explicit paths, and omit only the implicit GPU selector.
- [x] Run the focused Ginkgo specs and verify failures describe legacy Job
  fields or missing resolver injection.
- [x] Add resolver injection, resolve before manifest processing, bypass legacy
  manifest/profile steps, and construct the minimal native Job.
- [x] Run all `internal/controller` specs and verify legacy specs still pass.

### Task 3: Native serving layout

**Files:**
- Modify: `api/apps/v1alpha1/nimservice_types.go`
- Modify: `api/apps/v1alpha1/nimservice_types_test.go`
- Modify: `internal/controller/platform/standalone/nimservice.go`
- Modify: `internal/controller/platform/standalone/nimservice_test.go`
- Modify: `internal/controller/platform/kserve/nimservice.go`
- Modify: `internal/controller/platform/kserve/nimservice_test.go`

**Interfaces:**
- Consumes: `imageprotocol.Resolver` and the referenced NIMCache storage/env.
- Produces: parameterized model-volume mount paths without changing legacy
  `GetVolumeMounts` callers.

- [x] Add failing API tests for `/model` mount generation and controller specs
  for Standalone/KServe native env injection and protocol mismatch errors.
- [x] Run the focused packages and verify they fail for missing native layout.
- [x] Implement protocol-aware rendering, same-cache path derivation, explicit
  service override precedence, and mismatch validation.
- [x] Run API, Standalone, and KServe suites and verify both native and legacy
  cases pass.

### Task 4: Verification and single commit

**Files:**
- Verify all modified files plus this plan and its design spec.

- [x] Run `gofmt` over changed Go files in the Go 1.26.3 container.
- [x] Run focused tests with the required NVIDIA DRA linker version and EnvTest
  assets, followed by the repository's non-e2e unit suite.
- [x] Run `go vet` and confirm generated/vendor state is consistent.
- [x] Inspect the staging image label read-only with `regctl` when credentials
  are available.
- [x] Review `git diff --check`, `git diff`, and `git status` for scope,
  placeholders, secrets, and accidental generated output.
- [x] Commit all design, plan, tests, production code, and dependency metadata
  once with `feat: support native NIM model downloads`.
