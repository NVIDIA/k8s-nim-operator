#!/usr/bin/env bash

# Copyright 2024 The Kubernetes Authors.
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

set -o errexit
set -o nounset
set -o pipefail

SOURCE_DIR="$(cd "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
ROOT_DIR="$SOURCE_DIR/.."

GINKGO="$ROOT_DIR"/bin/ginkgo
GINKGO_ARGS=${GINKGO_ARGS:-}

ENABLE_NFD=${ENABLE_NFD:-true}
ENABLE_GPU_OPERATOR=${ENABLE_GPU_OPERATOR:-true}
ENABLE_LOCAL_PATH_PROVISIONER=${ENABLE_LOCAL_PATH_PROVISIONER:-true}
E2E_TIMEOUT_SECONDS=${E2E_TIMEOUT_SECONDS:-"1800"}

COLLECT_LOGS_FROM=${COLLECT_LOGS_FROM:-"default"}
LOG_ARTIFACT_DIR=${LOG_ARTIFACT_DIR:-${ROOT_DIR}/e2e_logs}
HELM_CHART=${HELM_CHART:-"${ROOT_DIR}/deployments/helm/k8s-nim-operator"}

E2E_IMAGE_REPO=${E2E_IMAGE_REPO:-"ghcr.io/nvidia/k8s-nim-operator"}
E2E_IMAGE_TAG=${E2E_IMAGE_TAG:-"main"}
E2E_IMAGE_PULL_POLICY=${E2E_IMAGE_PULL_POLICY:-"IfNotPresent"}

while getopts ":r:t:" opt; do
  case ${opt} in
    r )
      E2E_IMAGE_REPO=$OPTARG
      ;;
    t )
      E2E_IMAGE_TAG=$OPTARG
      ;;
    \? )
      echo "Usage: $0 [-r E2E_IMAGE_REPO] [-t E2E_IMAGE_TAG]"
      exit 1
      ;;
  esac
done

# check if KUBECONFIG is set
if [ -z "${KUBECONFIG:-}" ]; then
  echo "KUBECONFIG is not set"
  exit 1
fi

# Check if ginkgo binary is present 
if [ ! -f "$GINKGO" ]; then
  echo "ginkgo binary not found at $GINKGO, building it"
  make ginkgo
fi

# Set all ENV variables for e2e tests
export ENABLE_NFD ENABLE_GPU_OPERATOR ENABLE_LOCAL_PATH_PROVISIONER \
  COLLECT_LOGS_FROM LOG_ARTIFACT_DIR HELM_CHART \
  E2E_IMAGE_REPO E2E_IMAGE_TAG E2E_IMAGE_PULL_POLICY \
  E2E_TIMEOUT_SECONDS

# shellcheck disable=SC2086
$GINKGO $GINKGO_ARGS -v ./test/e2e/...
