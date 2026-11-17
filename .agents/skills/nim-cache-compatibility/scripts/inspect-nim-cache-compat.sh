#!/usr/bin/env bash
set -euo pipefail

# Detach stdin so kubectl cannot block on an interactive credential prompt.
exec </dev/null

usage() {
  cat >&2 <<'EOF'
Usage: inspect-nim-cache-compat.sh -n NAMESPACE -c NIMCACHE -i IMAGE [-p PROFILE] [options]

Reports a NIMCache's provenance and whether it holds every model artifact a NIM
service image needs. Prints a JSON report to stdout; progress goes to stderr.

  -n, --namespace NS       Namespace of the NIMCache
  -c, --cache NAME         NIMCache to inspect
  -i, --image IMAGE        Service image, e.g. nvcr.io/nim/meta/llama-3.2-1b-instruct:1.10.1
  -p, --profile ID         Profile the service will use (without it the result is Unknown)
      --pull-secret NAME   Image pull secret (defaults to the NIMCache's)
      --skip-volume        Do not mount the cache volume (use when it is attached elsewhere)
  -o, --output FILE        Also write the JSON report to FILE

Exit codes: 0 Compatible, 1 Incompatible, 2 Unknown, 3 error.
Environment: POD_TIMEOUT_SECONDS (default 600), CRANE_IMAGE, KUBECTL_REQUEST_TIMEOUT (default 15s).
EOF
}

die() { printf 'error: %s\n' "$*" >&2; exit 3; }
log() { printf '%s\n' "$*" >&2; }

namespace="" cache="" image="" profile="" pull_secret="" inspect_volume=true output=""
pod_timeout="${POD_TIMEOUT_SECONDS:-600}"
crane_image="${CRANE_IMAGE:-gcr.io/go-containerregistry/crane:debug}"

while [ $# -gt 0 ]; do
  case "$1" in
    -n|--namespace) namespace="$2"; shift 2 ;;
    -c|--cache) cache="$2"; shift 2 ;;
    -i|--image) image="$2"; shift 2 ;;
    -p|--profile) profile="$2"; shift 2 ;;
    --pull-secret) pull_secret="$2"; shift 2 ;;
    --skip-volume) inspect_volume=false; shift ;;
    -o|--output) output="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage; die "unknown argument: $1" ;;
  esac
done

kubectl() {
  command kubectl --request-timeout="${KUBECTL_REQUEST_TIMEOUT:-15s}" "$@"
}

command -v kubectl >/dev/null || die "kubectl not found"
command -v python3 >/dev/null || die "python3 not found"
python3 -c 'import yaml' 2>/dev/null || die "PyYAML is required: pip install pyyaml"
[ -n "$namespace" ] && [ -n "$cache" ] && [ -n "$image" ] \
  || { usage; die "--namespace, --cache and --image are required"; }

work="$(mktemp -d)"
run_id="$(date +%s)-$$"
label_key="nim-cache-compat/run"
cleanup() {
  kubectl delete pod -n "$namespace" -l "$label_key=$run_id" --wait=false >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

jp() { kubectl get "$1" "$2" -n "$namespace" -o jsonpath="$3"; }

kubectl get nimcache "$cache" -n "$namespace" -o json >"$work/cache.json" \
  || die "NIMCache $cache not found in $namespace"
cache_image="$(jp nimcache "$cache" '{.spec.source.ngc.modelPuller}')"
pvc="$(jp nimcache "$cache" '{.status.pvc}')"
[ -n "$pull_secret" ] || pull_secret="$(jp nimcache "$cache" '{.spec.source.ngc.pullSecret}')"

log "NIMCache:      $namespace/$cache (source image: ${cache_image:-none}, volume: ${pvc:-none})"
log "Service image: $image"
log "Profile:       ${profile:-<not given>}"

# pod_header NAME: shared Pod metadata; the label lets cleanup find every pod from this run.
pod_header() {
  cat <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: $1
  labels: {$label_key: "$run_id"}
spec:
  restartPolicy: Never
EOF
  if [ -n "$pull_secret" ]; then
    printf '  imagePullSecrets: [{name: %s}]\n' "$pull_secret"
  fi
}

# run_pod NAME TIMEOUT OUTFILE < pod-yaml: runs a one-shot pod and saves its logs.
run_pod() {
  local name="$1" timeout="$2" out="$3" phase reason waited=0
  kubectl apply -n "$namespace" -f - >/dev/null || return 1
  while :; do
    phase="$(kubectl get pod "$name" -n "$namespace" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
    reason="$(kubectl get pod "$name" -n "$namespace" \
      -o jsonpath='{.status.containerStatuses[0].state.waiting.reason}' 2>/dev/null || true)"
    case "$phase" in
      Succeeded) kubectl logs "$name" -n "$namespace" >"$out"; break ;;
      Failed) log "  pod $name failed:"; kubectl logs "$name" -n "$namespace" 2>&1 | tail -5 >&2; break ;;
    esac
    case "$reason" in
      ErrImagePull|ImagePullBackOff|InvalidImageName|CreateContainerConfigError)
        log "  pod $name cannot start: $reason"; phase="Failed"; break ;;
    esac
    if [ "$phase" = "Pending" ] && [ "$waited" -ge 30 ]; then
      reason="$(kubectl get events -n "$namespace" --field-selector "involvedObject.name=$name" \
        -o jsonpath='{range .items[*]}{.reason}{"\n"}{end}' 2>/dev/null \
        | grep -m1 -E '^(FailedMount|FailedAttachVolume|FailedScheduling)$' || true)"
      if [ -n "$reason" ]; then
        log "  pod $name cannot start: $reason"; phase="Blocked"; break
      fi
    fi
    if [ "$waited" -ge "$timeout" ]; then
      log "  pod $name still ${phase:-Pending} after ${timeout}s"
      phase="Timeout"; break
    fi
    sleep 5; waited=$((waited + 5))
  done
  kubectl delete pod "$name" -n "$namespace" --wait=false >/dev/null 2>&1 || true
  case "$phase" in
    Succeeded) return 0 ;;
    Blocked|Timeout) return 2 ;;
    *) return 1 ;;
  esac
}

log "Resolving image digests..."
digest_cmd="echo service=\$(crane digest $image)"
[ -n "$cache_image" ] && digest_cmd="echo cache=\$(crane digest $cache_image); $digest_cmd"
{ pod_header "compat-digests-$run_id"; cat <<EOF
  containers:
  - name: crane
    image: $crane_image
    command: [sh, -c, "$digest_cmd"]
EOF
  if [ -n "$pull_secret" ]; then cat <<EOF
    env: [{name: DOCKER_CONFIG, value: /docker}]
    volumeMounts: [{name: cfg, mountPath: /docker}]
  volumes:
  - name: cfg
    secret: {secretName: $pull_secret, items: [{key: .dockerconfigjson, path: config.json}]}
EOF
  fi; } | run_pod "compat-digests-$run_id" "$pod_timeout" "$work/digests.txt" \
  || log "  could not resolve digests; continuing without them"
cache_digest="$(sed -n 's/^cache=//p' "$work/digests.txt" 2>/dev/null || true)"
service_digest="$(sed -n 's/^service=//p' "$work/digests.txt" 2>/dev/null || true)"

manifest_cmd="cat /opt/nim/etc/default/model_manifest.yaml 2>/dev/null || cat /etc/nim/config/model_manifest.yaml 2>/dev/null || echo 'model_manifest.yaml not found'"
files_begin="===NIM-CACHE-COMPAT-FILES==="
files_end="===NIM-CACHE-COMPAT-FILES-COMPLETE==="
# The end marker is printed only if find finishes cleanly, so a partial listing is never trusted.
list_cmd="echo $files_begin; cd /cache && find ngc -path '*/snapshots/*' ! -type d && echo $files_end; true"

# manifest_pod NAME IMAGE OUTFILE [PVC]: prints the image's manifest and, with a PVC, the cache file list.
manifest_pod() {
  { pod_header "$1"
    if [ -n "${4:-}" ]; then cat <<EOF
  containers:
  - name: manifest
    image: $2
    command: [sh, -c, "$manifest_cmd; $list_cmd"]
    volumeMounts: [{name: cache, mountPath: /cache, readOnly: true}]
  volumes:
  - name: cache
    persistentVolumeClaim: {claimName: $4, readOnly: true}
EOF
    else cat <<EOF
  containers:
  - name: manifest
    image: $2
    command: [sh, -c, "$manifest_cmd"]
EOF
    fi; } | run_pod "$1" "$pod_timeout" "$3"
}

svc_pvc=""
if $inspect_volume && [ -n "$pvc" ]; then
  svc_pvc="$pvc"
  log "Reading the service image's model manifest and listing files on volume $pvc..."
else
  log "Reading the service image's model manifest..."
fi
svc_rc=0
manifest_pod "compat-svc-manifest-$run_id" "$image" "$work/service-output.txt" "$svc_pvc" || svc_rc=$?
if [ "$svc_rc" -ne 0 ]; then
  if [ -n "$svc_pvc" ] && [ "$svc_rc" -eq 2 ]; then
    log "  retrying without the cache volume; the result will rely on hash and digest evidence"
    manifest_pod "compat-svc-manifest-nv-$run_id" "$image" "$work/service-output.txt" \
      || log "  could not read the service manifest"
  else
    log "  could not read the service manifest"
  fi
fi
if [ -f "$work/service-output.txt" ]; then
  : >"$work/cache-files.partial"
  if awk -v begin="$files_begin" -v end="$files_end" -v files="$work/cache-files.partial" \
      -v manifest="$work/service-manifest.yaml" '
      $0 == begin { in_files = 1; next }
      $0 == end { complete = 1; next }
      in_files { print > files; next }
      { print > manifest }
      END { exit complete ? 0 : 1 }' "$work/service-output.txt"; then
    mv "$work/cache-files.partial" "$work/cache-files.txt"
  elif grep -qx "$files_begin" "$work/service-output.txt"; then
    log "  file listing on volume $pvc was incomplete; ignoring it"
  fi
fi

if [ -n "$cache_image" ]; then
  if [ "$cache_image" = "$image" ]; then
    cp "$work/service-manifest.yaml" "$work/cache-manifest.yaml" 2>/dev/null || true
  else
    log "Reading the cache source image's model manifest..."
    manifest_pod "compat-cache-manifest-$run_id" "$cache_image" "$work/cache-manifest.yaml" \
      || log "  could not read the cache manifest"
  fi
fi

rc=0
WORK="$work" SERVICE_IMAGE="$image" PROFILE="$profile" \
CACHE_DIGEST="$cache_digest" SERVICE_DIGEST="$service_digest" \
python3 - >"$work/report.json" <<'PY' || rc=$?
"""Compares the gathered NIMCache data with the service image's manifest."""
import hashlib
import json
import os
import re
import sys
import urllib.parse

import yaml

EXIT = {"Compatible": 0, "Incompatible": 1, "Unknown": 2}
WORK = os.environ["WORK"]


class TagTolerantLoader(yaml.SafeLoader):
    """Loads v1 manifests, which use custom tags such as !workspace and !name."""


def _construct_any(loader, _suffix, node):
    if isinstance(node, yaml.MappingNode):
        return loader.construct_mapping(node, deep=True)
    if isinstance(node, yaml.SequenceNode):
        return loader.construct_sequence(node, deep=True)
    return loader.construct_scalar(node)


TagTolerantLoader.add_multi_constructor("!", _construct_any)


def read_text(name):
    try:
        with open(os.path.join(WORK, name), encoding="utf-8") as f:
            return f.read()
    except FileNotFoundError:
        return None


def load_manifest(name):
    """Returns (schema, top-level model, {profile_id: profile}, sha256) or None."""
    text = read_text(name)
    if not text or "model_manifest.yaml not found" in text:
        return None
    data = yaml.load(text, Loader=TagTolerantLoader)
    if not isinstance(data, dict):
        return None
    digest = hashlib.sha256(text.encode()).hexdigest()
    if isinstance(data.get("profiles"), list):
        profiles = {p["id"]: p for p in data["profiles"] if isinstance(p, dict) and "id" in p}
        return "v2", data.get("model"), profiles, digest
    profiles = {k: v for k, v in data.items() if isinstance(v, dict)}
    models = {p.get("model") for p in profiles.values() if p.get("model")}
    return "v1", (models.pop() if len(models) == 1 else None), profiles, digest


def parse_ngc_uri(uri):
    """ngc://<repo>:<version>?file=<name> -> (repo, version, filename or None)."""
    u = urllib.parse.urlparse(uri)
    if u.scheme != "ngc":
        return None
    repo, sep, version = (u.netloc + u.path).partition(":")
    if not sep:
        return None
    return repo, version, urllib.parse.parse_qs(u.query).get("file", [None])[0]


def ngc_cache_path(repo, version, filename):
    return f"ngc/hub/models--{repo.replace('/', '--')}/snapshots/{version}/{filename}"


def required_files(profile):
    """Returns ([(cache_path, source_uri)], [unverifiable_uri], {model_version})."""
    workspace = profile.get("workspace") or {}
    needed, unverifiable, versions = [], [], set()
    for f in (workspace.get("files") or {}).values():  # v2
        uri = f.get("uri", "") if isinstance(f, dict) else str(f)
        parsed = parse_ngc_uri(uri)
        if not parsed or not parsed[2]:
            unverifiable.append(uri)
            continue
        repo, version, filename = parsed
        versions.add(f"{repo}:{version}")
        needed.append((ngc_cache_path(repo, version, filename), uri))
    for comp in workspace.get("components") or []:  # v1
        src = comp.get("src") or {}
        parsed = parse_ngc_uri(src.get("repo_id", ""))
        names = []
        for entry in src.get("files") or []:
            names.extend(entry.keys() if isinstance(entry, dict) else [entry])
        if not parsed:
            unverifiable.extend(f"{src.get('repo_id', '<no repo_id>')}/{n}" for n in names)
            continue
        repo, version, _ = parsed
        versions.add(f"{repo}:{version}")
        needed.extend((ngc_cache_path(repo, version, n), f"{src['repo_id']}/{n}") for n in names)
    return needed, unverifiable, versions


def cache_source(cache):
    spec = cache.get("spec", {}).get("source", {})
    if cache.get("status", {}).get("type") == "native":
        return "native", {"modelPuller": (spec.get("ngc") or {}).get("modelPuller")}
    for key, kind in (("hf", "huggingface"), ("dataStore", "datastore")):
        if spec.get(key):
            src = spec[key]
            return kind, {k: src.get(k) for k in ("endpoint", "namespace", "modelName", "datasetName", "revision")}
    ngc = spec.get("ngc") or {}
    if ngc.get("modelEndpoint"):
        return "universal", {"modelEndpoint": ngc["modelEndpoint"], "modelPuller": ngc.get("modelPuller")}
    return "ngc", {"modelPuller": ngc.get("modelPuller")}


cache = json.loads(read_text("cache.json"))
status = cache.get("status", {})
source_type, source_identity = cache_source(cache)
cached_profiles = [
    {"id": p.get("name"), "workspaceHash": (p.get("config") or {}).get("nim_workspace_hash_v1")}
    for p in status.get("profiles") or []
]
cache_manifest = load_manifest("cache-manifest.yaml")
svc_manifest = load_manifest("service-manifest.yaml")
cache_files = read_text("cache-files.txt")
profile = os.environ.get("PROFILE", "")
cache_digest = os.environ.get("CACHE_DIGEST") or None
service_digest = os.environ.get("SERVICE_DIGEST") or None

report = {
    "schemaVersion": "nim-cache-compat/v1",
    "cache": {
        "name": cache.get("metadata", {}).get("name"),
        "namespace": cache.get("metadata", {}).get("namespace"),
        "state": status.get("state"),
        "pvc": status.get("pvc"),
        "sourceType": source_type,
        "source": source_identity,
        "sourceImageDigest": cache_digest,
        "manifestSchema": cache_manifest[0] if cache_manifest else None,
        "manifestSha256": cache_manifest[3] if cache_manifest else None,
        "model": cache_manifest[1] if cache_manifest else None,
        "cachedProfiles": cached_profiles,
        "volumeInspected": cache_files is not None,
    },
    "service": {
        "image": os.environ["SERVICE_IMAGE"],
        "imageDigest": service_digest,
        "manifestSchema": svc_manifest[0] if svc_manifest else None,
        "manifestSha256": svc_manifest[3] if svc_manifest else None,
        "model": svc_manifest[1] if svc_manifest else None,
        "profile": profile or None,
    },
    "evidence": [],
    "missingFiles": [],
    "unverifiableFiles": [],
}
ev = report["evidence"]


def finish(result, reason, message):
    report.update(result=result, reason=reason, message=message)
    print(json.dumps(report, indent=2))
    sys.exit(EXIT[result])


if cache_digest and service_digest:
    ev.append("SameImageDigest" if cache_digest == service_digest else "DifferentImageDigest")

if status.get("state") != "Ready":
    finish("Unknown", "CacheNotReady", f"NIMCache state is {status.get('state')!r}, not Ready.")
if source_type != "ngc":
    finish("Unknown", "NoModelProfiles",
           f"{source_type} caches have no model profiles to compare. Confirm the service serves the same "
           f"model and revision as cache.source; artifact completeness cannot be verified.")
if svc_manifest is None:
    finish("Unknown", "ServiceManifestUnavailable",
           "Could not read model_manifest.yaml from the service image: the image or pull secret may be "
           "wrong, or the NIM does not use model profiles.")
cache_model, svc_model = report["cache"]["model"], report["service"]["model"]
if cache_model and svc_model and cache_model.rsplit("/", 1)[-1] != svc_model.rsplit("/", 1)[-1]:
    finish("Incompatible", "ModelMismatch",
           f"Cache holds {cache_model!r} but the service image serves {svc_model!r}.")
if not profile:
    finish("Unknown", "NoProfileSelected",
           "No profile given. The NIM chooses one at startup, so pass the profile the service will use.")
if profile not in svc_manifest[2]:
    finish("Incompatible", "ProfileNotInServiceManifest",
           f"Profile {profile} does not exist in the service image's manifest.")

svc_profile = svc_manifest[2][profile]
needed, unverifiable, versions = required_files(svc_profile)
svc_hash = (svc_profile.get("tags") or {}).get("nim_workspace_hash_v1")
report["service"].update(modelVersions=sorted(versions), requiredFiles=len(needed), workspaceHash=svc_hash)
report["unverifiableFiles"] = unverifiable
if profile in {p["id"] for p in cached_profiles}:
    ev.append("ProfileIdCached")
if svc_hash and svc_hash in {p["workspaceHash"] for p in cached_profiles if p["workspaceHash"]}:
    ev.append("WorkspaceHashMatch")

if cache_files is not None:
    on_disk = {line.strip() for line in cache_files.splitlines() if line.strip()}
    missing = [{"path": p, "source": uri} for p, uri in needed if p not in on_disk]
    report["missingFiles"] = missing
    if missing:
        finish("Incompatible", "ArtifactsMissing",
               f"{len(missing)} of {len(needed)} files required by profile {profile} are not on the cache "
               f"volume; the NIM would download them at startup.")
    if unverifiable:
        finish("Unknown", "UnsupportedSourceScheme",
               f"{len(unverifiable)} required files use a source that cannot be mapped to the cache.")
    if not needed:
        finish("Unknown", "NoArtifactList", f"Profile {profile} lists no workspace files.")
    ev.append("AllRequiredFilesOnVolume")
    finish("Compatible", "AllArtifactsPresent",
           f"All {len(needed)} files required by profile {profile} are on the cache volume.")

if "WorkspaceHashMatch" in ev:
    finish("Compatible", "WorkspaceHashMatch",
           "Volume not inspected; the service profile's workspace hash matches a cached profile.")
if "SameImageDigest" in ev and "ProfileIdCached" in ev:
    finish("Compatible", "SameImageAndProfileCached",
           "Volume not inspected; same image digest and the profile is recorded as cached.")
finish("Unknown", "VolumeNotInspected",
       "The cache volume was not inspected and no hash or digest evidence proves compatibility.")
PY

cat "$work/report.json"
[ -n "$output" ] && cp "$work/report.json" "$output"
exit "$rc"
