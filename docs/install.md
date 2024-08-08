<!--
  SPDX-FileCopyrightText: Copyright (c) 2024 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
  SPDX-License-Identifier: Apache-2.0
-->

# Installing NIM Operator for Kubernetes using Helm

### Pre-requisites

* A Kubernetes cluster with supported GPUs (H100, A100, L40S)
* NVIDIA GPU Operator have to be installed
* Access to following NGC NVAIE repositories required

Follow these steps to install the NIM Operator using Helm:

### 1. Clone and Repository

```sh
git clone git@github.com:NVIDIA/k8s-nim-operator.git
cd k8s-nim-operator
```

### 2. Create a Namespace for Installation

```sh
kubectl create ns nim-operator
```
### 3. Export NGC CLI API KEY

Refer to get [NGC CLI API Key](https://docs.nvidia.com/ngc/gpu-cloud/ngc-private-registry-user-guide/index.html#ngc-api-keys).

```sh
export NGC_API_KEY=<ngc-cli-api-key>
```

### 4. Create an Image Pull Secret

```sh
kubectl create secret -n nim-operator docker-registry ngc-secret \
    --docker-server=nvcr.io \
    --docker-username='$oauthtoken' \
    --docker-password=$NGC_API_KEY
```

### 5. Install the NIM Operator

Install the NIM Operator using Helm.

```sh
helm install nim-operator deployments/helm/k8s-nim-operator -n nim-operator \
  --set operator.image.repository=<your-private-registry> \
  --set operator.image.tag=<tag>
```

### 6. Verify Installation
Verify that the NIM Operator has been installed successfully by listing the Helm releases and checking the pods in the nim-operator namespace.

```sh
helm ls -n nim-operator
kubectl get pods -n nim-operator
```

```console
NAME                                             READY   STATUS    RESTARTS   AGE
nim-operator-k8s-nim-operator-567484cffb-p7zr2   2/2     Running   0          22h
```
