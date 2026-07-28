#
# Copyright 2024.
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

# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make build-bundle-image VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
MODULE_NAME := k8s-nim-operator
MODULE := github.com/NVIDIA/$(MODULE_NAME)

REGISTRY ?= ghcr.io/nvidia

VERSION ?= v0.0.0-main

GOLANG_VERSION ?= 1.26.5

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.30.0

# NVIDIA_DRA_DRIVER_GPU_VERSION is injected into sigs.k8s.io/dra-driver-nvidia-gpu/internal/info.version at build time.
NVIDIA_DRA_DRIVER_GPU_VERSION ?= v0.4.1

GIT_COMMIT ?= $(shell git describe --match="" --dirty --long --always 2> /dev/null || echo "")
