#!/usr/bin/env bash
set -euo pipefail

namespace="${NIM_OPERATOR_NAMESPACE:-nim-operator}"
release="${NIM_OPERATOR_RELEASE:-nim-operator}"

section() {
  printf '\n== %s ==\n' "$1"
}

run() {
  printf '+ %s\n' "$*"
  "$@"
}

section "Client tools"
run kubectl version --client=true
run helm version

section "Cluster"
run kubectl config current-context
run kubectl cluster-info

section "Helm release"
if helm status "${release}" -n "${namespace}" >/dev/null 2>&1; then
  run helm status "${release}" -n "${namespace}"
  run helm get values "${release}" -n "${namespace}"
else
  printf 'NIM Operator release %s not found in namespace %s\n' "${release}" "${namespace}"
fi

section "Operator namespace resources"
if kubectl get ns "${namespace}" >/dev/null 2>&1; then
  run kubectl get pods -n "${namespace}"
  run kubectl get deployment -n "${namespace}" -l "app.kubernetes.io/instance=${release},app.kubernetes.io/name=k8s-nim-operator"
else
  printf 'Namespace %s not found\n' "${namespace}"
fi

section "NIM CRDs"
for crd in \
  nimservices.apps.nvidia.com \
  nimcaches.apps.nvidia.com \
  nimpipelines.apps.nvidia.com \
  nimbuilds.apps.nvidia.com \
  nemodatastores.apps.nvidia.com \
  nemoentitystores.apps.nvidia.com \
  nemocustomizers.apps.nvidia.com \
  nemoevaluators.apps.nvidia.com \
  nemoguardrails.apps.nvidia.com; do
  if kubectl get crd "${crd}" >/dev/null 2>&1; then
    printf 'present: %s\n' "${crd}"
  else
    printf 'missing: %s\n' "${crd}"
  fi
done

section "Custom resources"
for resource in \
  nimservices.apps.nvidia.com \
  nimcaches.apps.nvidia.com \
  nimpipelines.apps.nvidia.com \
  nimbuilds.apps.nvidia.com \
  nemodatastores.apps.nvidia.com \
  nemoentitystores.apps.nvidia.com \
  nemocustomizers.apps.nvidia.com \
  nemoevaluators.apps.nvidia.com \
  nemoguardrails.apps.nvidia.com; do
  if kubectl get crd "${resource}" >/dev/null 2>&1; then
    printf '+ kubectl get %s -A\n' "${resource}"
    kubectl get "${resource}" -A || true
  else
    printf 'skip custom resource inventory; CRD missing: %s\n' "${resource}"
  fi
done
