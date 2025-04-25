# Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
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

BUILD_MULTI_ARCH_IMAGES ?= no
DOCKER ?= docker
BUILDX  =
ifeq ($(BUILD_MULTI_ARCH_IMAGES),true)
BUILDX = buildx
endif
MKDIR    ?= mkdir

##### Global variables #####
include $(CURDIR)/versions.mk

ifeq ($(IMAGE_NAME),)
IMAGE_NAME := $(REGISTRY)/$(MODULE_NAME)
endif

IMAGE_VERSION := $(VERSION)

IMAGE_TAG ?= $(IMAGE_VERSION)
IMAGE = $(IMAGE_NAME):$(IMAGE_TAG)
DOCKERFILE = $(CURDIR)/deployments/container/Dockerfile

# BUNDLE_IMAGE defines the image:tag used for the bundle.
BUNDLE_IMAGE_NAME ?= nvcr.io/nvidia/cloud-native/nim-operator-bundle
BUNDLE_DOCKERFILE = $(CURDIR)/deployments/container/bundle.Dockerfile

IMAGE_EXTRA_TAG_NAMES ?= ${BRANCH} $(VERSION)
BUNDLE_EXTRA_TAG_NAMES ?= ${BRANCH} $(VERSION)
IMAGE_EXTRA_TAGS := $(foreach tag,$(IMAGE_EXTRA_TAG_NAMES),$(IMAGE_NAME):$(tag))
BUNDLE_EXTRA_TAGS := $(foreach tag,$(BUNDLE_EXTRA_TAG_NAMES),$(BUNDLE_IMAGE_NAME):$(tag))

OUT_IMAGE_NAME ?= $(IMAGE_NAME)
OUT_IMAGE_VERSION ?= $(IMAGE_VERSION)
OUT_IMAGE_TAG = $(OUT_IMAGE_VERSION)
OUT_IMAGE = $(OUT_IMAGE_NAME):$(OUT_IMAGE_TAG)

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
ifneq ($(origin DEFAULT_CHANNEL), alpha)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

ifneq ($(BUILD_MULTI_ARCH_IMAGES),true)
include $(CURDIR)/deployments/container/native-only.mk
else
include $(CURDIR)/deployments/container/multi-arch.mk
endif

# For the default push target we also push a short tag equal to the version.
# We skip this for the development release
DEVEL_RELEASE_IMAGE_VERSION ?= devel
PUSH_MULTIPLE_TAGS ?= true
ifeq ($(strip $(OUT_IMAGE_VERSION)),$(DEVEL_RELEASE_IMAGE_VERSION))
PUSH_MULTIPLE_TAGS = false
endif
ifeq ($(PUSH_MULTIPLE_TAGS),true)
push-$(DEFAULT_PUSH_TARGET): push-short
endif

# Use a generic build target to build the relevant images
.PHONY: build build-bundle push push-short build-bundle-image push-bundle
build:
	DOCKER_BUILDKIT=1 \
		$(DOCKER) $(BUILDX) build --pull \
		$(DOCKER_BUILD_OPTIONS) \
		$(DOCKER_BUILD_PLATFORM_OPTIONS) \
		$(foreach tag,$(IMAGE_EXTRA_TAGS),-t $(tag)) \
		--build-arg GOLANG_VERSION="$(GOLANG_VERSION)" \
		--build-arg VERSION="$(VERSION)" \
		--build-arg GIT_COMMIT="$(GIT_COMMIT)" \
		-f $(DOCKERFILE) \
		$(CURDIR)

REGCTL ?= regctl
push:
	$(REGCTL) \
	        image copy \
	        $(IMAGE) $(OUT_IMAGE)

push-short:
	$(REGCTL) \
	        image copy \
	        $(IMAGE) $(OUT_IMAGE_NAME):$(OUT_IMAGE_VERSION)
build-bundle:
	DOCKER_BUILDKIT=1 \
		$(DOCKER) $(BUILDX) build --pull \
		$(DOCKER_BUILD_OPTIONS) \
		$(DOCKER_BUILD_PLATFORM_OPTIONS) \
		$(foreach tag,$(BUNDLE_EXTRA_TAGS),-t $(tag)) \
		--build-arg VERSION="$(VERSION)" \
		--build-arg DEFAULT_CHANNEL=$(DEFAULT_CHANNEL) \
		--build-arg GIT_COMMIT="$(GIT_COMMIT)" \
		-f $(BUNDLE_DOCKERFILE) \
		$(CURDIR)

# Push the bundle image.
push-bundle: build-bundle-image
	$(REGCTL) \
	        image copy \
	        $(BUNDLE_IMAGE) $(OUT_IMAGE_NAME):$(OUT_IMAGE_VERSION)
