# Report Reference

The script prints one JSON object with `schemaVersion: nim-cache-compat/v1`.

## Fields

| Field | Meaning |
|---|---|
| `result` | `Compatible`, `Incompatible` or `Unknown` |
| `reason` | Reason code; see the table below |
| `message` | Human-readable explanation |
| `evidence` | Supporting signals: `SameImageDigest`, `DifferentImageDigest`, `ProfileIdCached`, `WorkspaceHashMatch`, `AllRequiredFilesOnVolume` |
| `missingFiles[]` | `{path, source}` for each required file absent from the cache volume |
| `unverifiableFiles[]` | Required files whose source is not `ngc://` and cannot be mapped to the volume |
| `cache.sourceType` | `ngc`, `universal`, `huggingface`, `datastore` or `native` |
| `cache.source` | Source identity from the NIMCache spec (image, endpoint, model name, revision) |
| `cache.sourceImageDigest` | Digest the cache's source image tag resolves to now |
| `cache.manifestSchema`, `cache.manifestSha256`, `cache.model` | Format, hash and model name of the cache image's full manifest |
| `cache.cachedProfiles[]` | `{id, workspaceHash}` from `NIMCache.status.profiles` |
| `cache.volumeInspected` | Whether the cache volume's file list was read |
| `service.image`, `service.imageDigest` | Service image and its resolved digest |
| `service.manifestSchema`, `service.manifestSha256`, `service.model` | Format, hash and model name of the service image's manifest |
| `service.profile` | Profile checked |
| `service.modelVersions` | NGC model versions the profile's files come from |
| `service.requiredFiles`, `service.workspaceHash` | File count and `nim_workspace_hash_v1` tag of the profile |

## Reason Codes

| Result | Reason | Meaning |
|---|---|---|
| Compatible | `AllArtifactsPresent` | Every required file is on the volume. |
| Compatible | `WorkspaceHashMatch` | Volume not inspected; the profile's workspace hash matches a cached profile. |
| Compatible | `SameImageAndProfileCached` | Volume not inspected; same image digest and the profile is recorded as cached. |
| Incompatible | `ArtifactsMissing` | Required files are missing; the NIM would download them. |
| Incompatible | `ProfileNotInServiceManifest` | The profile does not exist in the service image. |
| Incompatible | `ModelMismatch` | The cache and image serve different models. |
| Unknown | `NoProfileSelected` | No profile given; the NIM chooses one at startup. |
| Unknown | `NoModelProfiles` | Hugging Face, DataStore, universal or native cache; compare `cache.source` manually. |
| Unknown | `CacheNotReady` | The NIMCache is not `Ready`. |
| Unknown | `ServiceManifestUnavailable` | The image could not be pulled or has no manifest. |
| Unknown | `UnsupportedSourceScheme` | Some required files are not `ngc://` and cannot be checked. |
| Unknown | `NoArtifactList` | The profile lists no files. |
| Unknown | `VolumeNotInspected` | The volume was skipped or unreachable, and no hash or digest evidence proves compatibility. |

## Known Limitations

- Digests reflect what tags point to now, not when the cache was built. The Operator does not record the cache's build-time digest.
- v1 manifests are parsed, and their `repo_id` is mapped the same way as v2 file URIs. This mapping has not been tested against a real v1 cache.
- A `ReadWriteOnce` volume attached to a running pod on another node cannot be mounted. The script detects the mount or scheduling failure and retries without the volume, so the result relies on hash and digest evidence.
- The service image must contain `sh`, `cat` and `find`, which current NIM images do.

## Validation

Package check:

```sh
test -f .agents/skills/nim-cache-compatibility/SKILL.md
grep -q '^name: nim-cache-compatibility$' .agents/skills/nim-cache-compatibility/SKILL.md
test -x .agents/skills/nim-cache-compatibility/scripts/inspect-nim-cache-compat.sh
bash -n .agents/skills/nim-cache-compatibility/scripts/inspect-nim-cache-compat.sh
```

Live check against a cluster with a Ready NIMCache, comparing the cache with its own image and first cached profile. It should return `Compatible` / `AllArtifactsPresent`:

```sh
.agents/skills/nim-cache-compatibility/scripts/inspect-nim-cache-compat.sh -n <ns> -c <nimcache> \
  -i "$(kubectl get nimcache <nimcache> -n <ns> -o jsonpath='{.spec.source.ngc.modelPuller}')" \
  -p "$(kubectl get nimcache <nimcache> -n <ns> -o jsonpath='{.status.profiles[0].name}')"
```
