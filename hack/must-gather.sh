#!/usr/bin/env bash

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
