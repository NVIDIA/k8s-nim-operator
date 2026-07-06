#!/usr/bin/env bash
set -euo pipefail

namespace="${NIM_OPERATOR_NAMESPACE:-nim-operator}"
release="${NIM_OPERATOR_RELEASE:-nim-operator}"
gpu_namespace="${GPU_OPERATOR_NAMESPACE:-gpu-operator}"

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
run kubectl auth can-i create customresourcedefinitions.apiextensions.k8s.io
run kubectl auth can-i create clusterroles.rbac.authorization.k8s.io

section "Nodes and GPUs"
run kubectl get nodes
run kubectl get nodes -o 'custom-columns=NAME:.metadata.name,GPUS:.status.allocatable.nvidia\.com/gpu'

section "GPU Operator"
if kubectl get ns "${gpu_namespace}" >/dev/null 2>&1; then
  run kubectl get pods -n "${gpu_namespace}"
else
  printf 'GPU Operator namespace %s not found\n' "${gpu_namespace}"
fi
if kubectl get clusterpolicies.nvidia.com >/dev/null 2>&1; then
  run kubectl get clusterpolicies.nvidia.com
else
  printf 'GPU Operator ClusterPolicy CRD or resources not found\n'
fi

section "cert-manager"
if kubectl get ns cert-manager >/dev/null 2>&1; then
  run kubectl get pods -n cert-manager
else
  printf 'cert-manager namespace not found\n'
fi

section "KServe"
if kubectl get crd inferenceservices.serving.kserve.io >/dev/null 2>&1; then
  run kubectl get crd inferenceservices.serving.kserve.io
  if kubectl get ns kserve >/dev/null 2>&1; then
    run kubectl get pods -n kserve
  fi
else
  printf 'KServe InferenceService CRD not found\n'
fi

section "NIM Operator"
if helm status "${release}" -n "${namespace}" >/dev/null 2>&1; then
  run helm status "${release}" -n "${namespace}"
  run helm get values "${release}" -n "${namespace}"
  run kubectl get deployment -n "${namespace}" -l "app.kubernetes.io/instance=${release},app.kubernetes.io/name=k8s-nim-operator"
  run kubectl get pods -n "${namespace}"
else
  printf 'NIM Operator release %s not found in namespace %s\n' "${release}" "${namespace}"
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
