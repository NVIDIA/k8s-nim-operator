---
name: nim-cache-compatibility
description: Reports NIMCache provenance and whether the cache holds every model artifact a NIM service image needs, as machine-readable JSON. Covers model identity and NGC model version, source image digests, manifest hashes, cached profiles, and a Compatible, Incompatible or Unknown verdict with a reason and the exact missing files. Use for NIMCache/NIMService compatibility, cache misses, unexpected model downloads, or checking a NIM image upgrade against an existing cache.
---

# NIM Cache Compatibility

Use this skill to report where a `NIMCache` came from and whether a NIM service image can run entirely from it. The NIM Operator does not record this or compare the cache with the service image; when files are missing, the NIM downloads them at startup without warning.

This skill only produces the report. It does not change Operator behavior, raise conditions or events, or block deployments.

## Workspace Root

Run commands from the root of the `k8s-nim-operator` repository and verify:

```sh
test -x .agents/skills/nim-cache-compatibility/scripts/inspect-nim-cache-compat.sh
```

## Safety Contract

- Read-only commands (`kubectl get`, `kubectl describe`) may run directly.
- The script creates up to three short-lived pods in the target namespace: one resolves image digests, one runs the service image with the cache volume mounted read-only to read its manifest and list the cached files, and one reads the cache image's manifest when it differs from the service image. They pull the images (often several GB) and delete themselves. Before the first run in a namespace, tell the user this and ask for confirmation.
- Never read, print or copy secret values. The script passes the image pull secret to pods by name only.

## Requirements

`kubectl` pointing at the cluster, `python3` with PyYAML, and cluster access to pull `gcr.io/go-containerregistry/crane:debug`. On air-gapped clusters, set `CRANE_IMAGE` to a mirrored copy.

## Run the Report

```sh
.agents/skills/nim-cache-compatibility/scripts/inspect-nim-cache-compat.sh \
  -n <namespace> -c <nimcache> -i <service-image> -p <profile-id> [-o report.json]
```

- `-p` is the profile the service will use: `spec.storage.nimCache.profile` on the `NIMService`. Without it, the result is `Unknown` / `NoProfileSelected`, because the NIM chooses a profile at startup.
- If the cache volume cannot be mounted (for example, it is attached to a pod on another node), the script notices within about 30 seconds and retries without it. Add `--skip-volume` to skip the volume from the start.
- The JSON report goes to stdout and progress to stderr. Exit codes: `0` Compatible, `1` Incompatible, `2` Unknown, `3` script error.

## How the Verdict Is Decided

Use these rules when explaining a result.

- **The volume's file list decides.** Each file the profile needs (`ngc://<repo>:<version>?file=<name>` in the service image's manifest) must exist at `ngc/hub/models--<repo with / as -->/snapshots/<version>/<name>` on the cache volume.
- **Tags are not compared.** Different tags can be the same image, and different releases can share model files.
- **Profile IDs change between releases.** A profile ID missing from `status.profiles` does not mean its files are missing.
- **A matching digest is not enough.** The same image can use a profile that was never cached.
- **Without the volume** (`--skip-volume` or a timeout): `Compatible` only if the profile's `nim_workspace_hash_v1` matches a cached profile, or the digest matches and the profile ID is recorded as cached. Otherwise `Unknown` / `VolumeNotInspected`.
- **Caches without model profiles** (Hugging Face, DataStore, universal and native) return `Unknown` / `NoModelProfiles`, with the source model and revision in `cache.source`.
- **The image's own manifest is used.** The `<nimcache>-manifest` ConfigMap has its file lists stripped.

## Reporting

Lead with the verdict, then the provenance:

```markdown
**Verdict:** <Compatible | Incompatible | Unknown> (<reason>)
<message from the report>

- Cache: <namespace>/<name>, built from <cache.source.modelPuller> (<cache.sourceImageDigest>), manifest <cache.manifestSha256>
- Service: <service.image> (<service.imageDigest>), manifest <service.manifestSha256>, profile <service.profile>
- Model: <service.model>, NGC model version <service.modelVersions>
- Missing files: <count, plus the first few paths, or "none">
```

## Additional Resources

- Report fields, reason codes and known limitations: [references/report-reference.md](references/report-reference.md)
