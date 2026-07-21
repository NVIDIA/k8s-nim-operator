# Native Model Download Protocol Design

## Goal

Teach NIM Operator to recognize the image configuration label
`com.nvidia.nim.model_download_protocol=native-v1` and use a NIM image's own
download-only lifecycle instead of the legacy `download-to-cache` command.
Legacy images must retain their current behavior.

## Protocol discovery

The operator reads the model-puller image configuration from its registry. It
authenticates with the Kubernetes image pull secret already referenced by the
resource. For a multi-platform image, every runnable child image configuration
is checked while attestation and artifact descriptors are ignored. Credential
matching preserves repository paths and Docker Hub aliases instead of
broadening a path-scoped credential to its whole registry. The resolver returns
`native-v1` only when every child has the exact label value. An absent or
consistently unknown value selects legacy behavior.
Registry, authentication, malformed-secret, and inconsistent-platform errors
fail reconciliation with context identifying the image.

The registry implementation is hidden behind a small interface. Controller
tests use a fake resolver; registry-specific tests exercise protocol selection,
multi-platform consistency, and Kubernetes Docker credential parsing.

## NIMCache behavior

Protocol discovery occurs before manifest extraction. Native images bypass
the legacy temporary manifest Pod, model-manifest parsing, profile selection,
and profile arguments. Their cache Job:

- runs the image's default entrypoint and arguments;
- forces `NIM_ENGINE_MODEL_DOWNLOAD_ONLY=1`;
- mounts the model PVC at `/model`;
- sets `NIM_ENGINE_MODEL_PATH=/model` for an operator-created PVC;
- sets `NIM_ENGINE_MODEL_PATH=/model/<NIMCache-name>` for an existing/shared
  PVC;
- permits an explicit `NIM_ENGINE_MODEL_PATH` in `spec.env` to override that
  calculated default;
- passes `NIM_ENGINE_MODEL_NAME`, `NIM_ENGINE_MODEL_VARIANT`, `HF_TOKEN`, and
  `NGC_API_KEY` through the existing environment mechanisms; and
- does not add the default GPU-node selector, while preserving an explicitly
  configured node selector.

`NIM_ENGINE_MODEL_DOWNLOAD_ONLY` is operator-owned in a cache Job and cannot be
overridden through `spec.env`. A successful native Job marks the cache Ready
with an empty profile list.

Retriever chooses its own provider: `HF_TOKEN` selects Hugging Face,
`NGC_API_KEY` selects NGC, and the presence of both prefers Hugging Face.

## NIMService behavior

Standalone and KServe workloads inspect their serving image through the same
resolver. A `native-v1` workload mounts its model volume at `/model` and gets
the path calculated from its referenced NIMCache. An explicit service
`NIM_ENGINE_MODEL_PATH` remains authoritative. Serving workloads never receive
`NIM_ENGINE_MODEL_DOWNLOAD_ONLY`.

Retriever uses the same image for caching and serving. This is treated as an
invariant; if the cache and serving protocols differ, reconciliation fails
clearly instead of constructing incompatible volume layouts.

Only the ordinary NGC model-puller cache route can opt into `native-v1`.
Hugging Face, DataStore, and NGC `modelEndpoint` cache routes remain legacy, so
a native serving image referencing one of those caches fails the same protocol
compatibility check.

Legacy workload mounts and environment variables remain unchanged.

## Compatibility and testing

No CRD fields are added. Exact `native-v1` is the only opt-in; unlabeled and
unknown-label images remain on the legacy path. Unit and controller tests cover
the resolver, legacy regression behavior, dedicated and shared PVC paths,
environment precedence, manifest/profile bypass, CPU scheduling, protocol
mismatch, and both Standalone and KServe rendering. A read-only smoke check
uses `nvcr.io/nvstaging/nim/retriever-photon-od:2.0.0-478fb74dec188314` when
registry credentials are available.
