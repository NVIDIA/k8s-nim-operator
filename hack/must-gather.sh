#!/usr/bin/env bash

# Copyright 2025 NVIDIA CORPORATION
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

###############################################################################
# NVIDIA NIM & NeMo Must-Gather Script
#
# This script collects logs and specs from:
#   - GPU node status and descriptions
#   - Kubernetes version info
#   - NIM Operator
#   - NIMPipeline/NIMService/NIMCache CRs and pods
#   - NIM Model Manifest ConfigMaps
#   - NeMo microservices CRs and pods (optional)
#
# Usage:
#   export OPERATOR_NAMESPACE=<namespace where NIM Operator is installed>
#   export NIM_NAMESPACE=<namespace where NIMService/NIMCache are deployed>
#   export NEMO_NAMESPACE=<namespace where NeMo microservices are deployed>   # Optional
#
#   ./must-gather.sh
#
# Output will be saved to:
#   ${ARTIFACT_DIR:-/tmp/nim-nemo-must-gather_<timestamp>}
###############################################################################

set -o nounset
set -o errexit
set -x

K=kubectl
if ! $K version > /dev/null 2>&1; then
    K=oc
    if ! $K version > /dev/null 2>&1; then
        echo "FATAL: neither 'kubectl' nor 'oc' appear to be working. Exiting..."
        exit 1
    fi
fi

export ARTIFACT_DIR="${ARTIFACT_DIR:-/tmp/nim-nemo-must-gather_$(date +%Y%m%d_%H%M)}"
mkdir -p "$ARTIFACT_DIR"

exec 1> >(tee "$ARTIFACT_DIR/must-gather.log")
exec 2> "$ARTIFACT_DIR/must-gather.stderr.log"

######################################
# CLUSTER INFO
######################################
mkdir -p "$ARTIFACT_DIR/cluster"

echo "Gathering Kubernetes version info"
$K version -o yaml > "$ARTIFACT_DIR/cluster/k8s_version.yaml" || true

echo "Gathering GPU node status"
$K get nodes -l nvidia.com/gpu.present=true -o wide > "$ARTIFACT_DIR/cluster/gpu_nodes.status" || true

echo "Gathering GPU node descriptions"
$K describe nodes -l nvidia.com/gpu.present=true > "$ARTIFACT_DIR/cluster/gpu_nodes.descr" || true

######################################
# NIM OPERATOR PODS
######################################
if [[ -z "${OPERATOR_NAMESPACE:-}" ]]; then
  echo "FATAL: OPERATOR_NAMESPACE env variable not set"
  exit 1
fi

mkdir -p "$ARTIFACT_DIR/operator"

echo "Gathering NIM Operator pods from $OPERATOR_NAMESPACE"
for pod in $(   ); do
  pod_name=$(basename "$pod")
  $K logs "$pod" -n "$OPERATOR_NAMESPACE" --all-containers --prefix > "$ARTIFACT_DIR/operator/${pod_name}.log" || true
  $K describe "$pod" -n "$OPERATOR_NAMESPACE" > "$ARTIFACT_DIR/operator/${pod_name}.descr" || true
done

######################################
# NIM SERVICE & CACHE
######################################
if [[ -z "${NIM_NAMESPACE:-}" ]]; then
  echo "FATAL: NIM_NAMESPACE env variable not set"
  exit 1
fi

mkdir -p "$ARTIFACT_DIR/nim"

echo "Gathering NIMPipeline, NIMService and NIMCache CRs from $NIM_NAMESPACE"
$K get nimcaches.apps.nvidia.com -n "$NIM_NAMESPACE" -oyaml > "$ARTIFACT_DIR/nim/nimcaches.yaml" || true
$K get nimpipelines.apps.nvidia.com -n "$NIM_NAMESPACE" -oyaml > "$ARTIFACT_DIR/nim/nimpipelines.yaml" || true
$K get nimservices.apps.nvidia.com -n "$NIM_NAMESPACE" -oyaml > "$ARTIFACT_DIR/nim/nimservices.yaml" || true

echo "Gathering ConfigMaps in $NIM_NAMESPACE owned by NIMCache"
mkdir -p "$ARTIFACT_DIR/nim/configmaps"

for cm in $($K get configmaps -n "$NIM_NAMESPACE" -o name); do
  # Check if the ownerReference has kind: NIMCache
  if $K get "$cm" -n "$NIM_NAMESPACE" -o yaml | grep -A 5 'ownerReferences:' | grep -q 'kind: NIMCache'; then
    cm_name=$(basename "$cm")
    $K get "$cm" -n "$NIM_NAMESPACE" -oyaml > "$ARTIFACT_DIR/nim/configmaps/${cm_name}.yaml" || true
  fi
done

echo "Gathering NIMService pods from $NIM_NAMESPACE"
for pod in $($K get pods -n "$NIM_NAMESPACE" -l "app.kubernetes.io/part-of=nim-service,app.kubernetes.io/managed-by=k8s-nim-operator" -oname); do
  pod_name=$(basename "$pod")
  $K logs "$pod" -n "$NIM_NAMESPACE" --all-containers --prefix > "$ARTIFACT_DIR/nim/${pod_name}.log" || true
  $K describe "$pod" -n "$NIM_NAMESPACE" > "$ARTIFACT_DIR/nim/${pod_name}.descr" || true
done

######################################
# NEMO MICROSERVICES
######################################
if [[ -n "${NEMO_NAMESPACE:-}" ]]; then
  mkdir -p "$ARTIFACT_DIR/nemo"

  echo "Gathering NeMo CRs from $NEMO_NAMESPACE"
  RESOURCES=(
    nemocustomizers.apps.nvidia.com
    nemodatastores.apps.nvidia.com
    nemoentitystores.apps.nvidia.com
    nemoevaluators.apps.nvidia.com
    nemoguardrails.apps.nvidia.com
  )

  for res in "${RESOURCES[@]}"; do
    $K get "$res" -n "$NEMO_NAMESPACE" -oyaml > "$ARTIFACT_DIR/nemo/${res}.yaml" || true
  done

  echo "Gathering NeMo microservice pods from $NEMO_NAMESPACE"
  for pod in $($K get pods -n "$NEMO_NAMESPACE" -l "app.kubernetes.io/managed-by=k8s-nim-operator" -oname); do
    pod_name=$(basename "$pod")
    $K logs "$pod" -n "$NEMO_NAMESPACE" --all-containers --prefix > "$ARTIFACT_DIR/nemo/${pod_name}.log" || true
    $K describe "$pod" -n "$NEMO_NAMESPACE" > "$ARTIFACT_DIR/nemo/${pod_name}.descr" || true
  done
else
  echo "Skipping NeMo microservice collection. NEMO_NAMESPACE not set."
fi

echo "Must gather logs collected successfully and saved to: $ARTIFACT_DIR"
