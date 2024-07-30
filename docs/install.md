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

### 3. Create an Image Pull Secret

Replace <ngc-cli-api-key> with your NGC CLI API key.

```sh
kubectl create secret -n nim-operator docker-registry ngc-secret \
    --docker-server=nvcr.io \
    --docker-username='$oauthtoken' \
    --docker-password=<ngc-cli-api-key>
```

### 4. Install the NIM Operator
Install the NIM Operator using the Helm chart located in the helm/k8s-nim-operator directory.

```sh
helm install nim-operator helm/k8s-nim-operator -n nim-operator
```

### 5. Verify Installation
Verify that the NIM Operator has been installed successfully by listing the Helm releases and checking the pods in the nim-operator namespace.

```sh
helm ls -n nim-operator
kubectl get pods -n nim-operator
```

```console
NAME                                             READY   STATUS    RESTARTS   AGE
nim-operator-k8s-nim-operator-567484cffb-p7zr2   2/2     Running   0          22h
```
