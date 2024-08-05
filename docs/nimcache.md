<!--
  SPDX-FileCopyrightText: Copyright (c) 2024 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
  SPDX-License-Identifier: Apache-2.0
-->

# Caching NIM Models

Follow these steps to cache NIM models in a persistent volume.

## Prerequisites

* NVIDIA GPU Operator is installed.
* NVIDIA NIM Operator is installed.
* You must have an active subscription to an NVIDIA AI Enterprise product or be an NVIDIA Developer Program
  [member](https://build.nvidia.com/explore/discover?integrate_nim=true&developer_enroll=true&self_hosted_api=true&signin=true).
  Access to the containers and models for NVIDIA NIM microservices is restricted.

* A persistent volume provisioner is installed.

  The Local Path Provisioner from Rancher is acceptable for development on a single-node cluster.

## 1. Create a Namespace for Running NIM Microservices

```sh
kubectl create ns nim-service
```

### 2. Create an Image Pull Secret for the NIM Container

Replace <ngc-cli-api-key> with your NGC CLI API key.

```sh
kubectl create secret -n nim-service docker-registry ngc-secret \
    --docker-server=nvcr.io \
    --docker-username='$oauthtoken' \
    --docker-password=<ngc-cli-api-key>
```

## 3. Create the NIM Cache Instance and Enable Model Auto-Detection

Update the `NIMCache` custom resource (CR) with appropriate values for model selection.
These include `model.precision`, `model.engine`, `model.qosProfile`, `model.gpu.product` and `model.gpu.ids`.
With these, the NIM Operator can extract the supported profiles and use that for caching.

Alternatively, if you specify `model.profiles`, then the model puller downloads and caches that particular model profile.

```yaml
apiVersion: apps.nvidia.com/v1alpha1
kind: NIMCache
metadata:
  labels:
    app.kubernetes.io/name: k8s-nim-operator
    app.kubernetes.io/managed-by: kustomize
  name: meta-llama3-8b-instruct
spec:
  source:
    ngc:
      modelPuller: nvcr.io/nim/meta/llama3-8b-instruct:1.0.0
      pullSecret: ngc-secret
      authSecret: ngc-api-secret
      model:
        profiles: []
        autoDetect: true
        precision: "fp8"
        engine: "tensorrt_llm"
        qosProfile: "throughput"
        gpu:
          product: "l40s"
          ids:
            - "26b5"
        tensorParallelism: "1"
  storage:
    pvc:
      create: true
      storageClass: "local-path"
      size: "50Gi"
      volumeAccessMode: ReadWriteOnce
```

### 4. Create the CR

```sh
kubectl create -f nimcache.yaml -n nim-service
```

### 5. Verify the Progress of NIM Model Caching

Verify that the NIM Operator has initiated the caching job and track status via the CR.

```sh
kubectl get nimcache -n nim-service -o wide
```

```output
NAME                             STATUS   PVC                                  AGE
meta-llama3-8b-instruct   ready    meta-llama3-8b-instruct-pvc   2024-07-04T23:22:13Z
```

Get the NIM cache so you can view the status:

```sh
kubectl get nimcache -n nim-service -o yaml
```

```output
apiVersion: apps.nvidia.com/v1alpha1
kind: NIMCache
metadata:
  annotations:
    nvidia.com/selected-profiles: '["09e2f8e68f78ce94bf79d15b40a21333cea5d09dbe01ede63f6c957f4fcfab7b"]'
  creationTimestamp: "2024-07-04T23:22:13Z"
  finalizers:
  - finalizer.nimcache.apps.nvidia.com
  generation: 2
  labels:
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: k8s-nim-operator
  name: meta-llama3-8b-instruct
  namespace: nim-cache
  resourceVersion: "16539047"
  uid: 81bda896-5ce2-4d63-b082-27c9a963250a
spec:
  source:
    ngc:
      authSecret: ngc-api-secret
      model:
        autoDetect: true
        engine: tensorrt_llm
        gpu:
          ids:
          - 26b5
          product: l40s
        precision: fp8
        qosProfile: throughput
        tensorParallelism: "1"
      modelPuller: nvcr.io/nvidian/nim-llm-dev/meta-llama3-8b-instruct:1.0.0
      pullSecret: ngc-secret
  storage:
    pvc:
      create: true
      size: 50Gi
      storageClass: local-path
      volumeAccessMode: ReadWriteOnce
status:
  conditions:
  - lastTransitionTime: "2024-07-04T23:22:13Z"
    message: The PVC has been created for caching NIM
    reason: PVCCreated
    status: "True"
    type: NIM_CACHE_PVC_CREATED
  - lastTransitionTime: "2024-07-05T22:13:11Z"
    message: The Job to cache NIM has been created
    reason: JobCreated
    status: "True"
    type: NIM_CACHE_JOB_CREATED
  - lastTransitionTime: "2024-07-05T22:13:27Z"
    message: The Job to cache NIM is in pending state
    reason: JobPending
    status: "True"
    type: NIM_CACHE_JOB_PENDING
  - lastTransitionTime: "2024-07-05T22:13:27Z"
    message: The Job to cache NIM has successfully completed
    reason: JobCompleted
    status: "True"
    type: NIM_CACHE_JOB_COMPLETED
  profiles:
    - model: meta/llama3-8b-instruct
      release: 1.0.0
      tags:
        feat_lora: "false"
        gpu: A100
        gpu_device: 20b2:10de
        llm_engine: tensorrt_llm
        pp: "1"
        precision: fp16
        profile: latency
        tp: "2"
  pvc: meta-llama3-8b-instruct-pvc
  state: ready
```
