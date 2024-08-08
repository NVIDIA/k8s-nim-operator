<!--
  SPDX-FileCopyrightText: Copyright (c) 2024 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
  SPDX-License-Identifier: Apache-2.0
-->

# NVIDIA NIM Operator

NVIDIA NIM Operator is a Kubernetes Operator that is designed to facilitate the deployment, management, and scaling of NVIDIA NIM microservices on Kubernetes clusters.

NVIDIA NIM microservices deliver AI foundation models as accelerated inference microservices that are portable across data center, workstation, and cloud, accelerating flexible generative AI development, deployment and time to value.

## Getting Started

### Prerequisites

- Kubernetes v1.28 and higher.
- NVIDIA GPUs that are supported by the NIM microservices to deploy.

### Deploying the Operator on the Cluster

**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<your-private-registry>/k8s-nim-operator:<tag>
```

> Publish the image to a personal registry.
> You must be able to pull the image from the working environment.
> Make sure you have the proper permission to the registry if the preceding commands result in an error.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<your-private-registry>/k8s-nim-operator:<tag>
```

> If you encounter RBAC errors, you might need to grant yourself cluster-admin
privileges or be logged in as admin.
> Ensure that the samples have default values.

### Deploying Sample `NIMCache` and `NIMService` Resources

Follow the guides in the [docs](./docs) directory to deploy sample CR instances.

### Uninstalling the Operator

**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs (CRDs) from the cluster:**

```sh
make uninstall
```

**Undeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Perform the following steps to build the installation manifests and distribute this project to users.

1. Build the manifests for the image built and published in the registry:

   ```sh
   make build-installer IMG=<your-private-registry>/k8s-nim-operator:<tag>
   ```

   The preceding Makefile target generates a `dist/install.yaml` file.
   This file is built with Kustomize and contains the manifests for the CRDs and resources that are necessary to install this project without
   its dependencies.

2. Run the installer:

   ```sh
   kubectl apply -f https://raw.githubusercontent.com/<org>/k8s-nim-operator/<tag or branch>/dist/install.yaml
   ```

## Contributing

NVIDIA can work with partners to add platform support for the NIM Operator.
The NIM Operator is open-source and permissively [licensed](LICENSE.md) with only minimal requirements for source code [contributions](CONTRIBUTING.md).

To get started with building the NIM Operator, follow these steps:

```shell
git clone git@github.com:NVIDIA/k8s-nim-operator.git
cd k8s-nim-operator
make IMG=<image-name> docker-build
```

Run `make help` for more information about additional `make` targets.

More information can be found in the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html).
