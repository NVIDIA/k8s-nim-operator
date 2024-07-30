# The NVIDIA NIM Operator
An Operator for deployment and maintenance of various NeMo microservices along with NVIDIA Inferencing Microservices (NIMs) in a Kubernetes environment.

## Description
The NVIDIA NIM Operator is a Kubernetes operator designed to facilitate the deployment, management, and scaling of NVIDIA NIMs and related NeMo microservices. The NIM Operator streamlines the integration of these powerful AI capabilities into cloud-native environments such as Kubernetes, leveraging NVIDIA GPUs.

## Getting Started

### Prerequisites
- Access to a Kubernetes v1.28+ cluster with supported NVIDIA GPUs

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=nvcr.io/nvidia/cloud-native/k8s-nim-operator:v0.1.0
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=nvcr.io/nvidia/cloud-native/k8s-nim-operator:v0.1.0
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

>**NOTE**: Ensure that the samples has default values to test it out.

### To Deploy a sample `NIMCache` and `NIMService` instance

Follow the guides [here](https://github.com/NVIDIA/k8s-nim-operator/-/tree/main/docs?ref_type=heads) to deploy sample CR instances.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=nvcr.io/nvidia/cloud-native/k8s-nim-operator:v0.1.0
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/k8s-nim-operator/<tag or branch>/dist/install.yaml
```

## Contributing
NVIDIA is willing to work with partners for adding platform support for the NIM Operator. The NIM Operator is open-source and permissively licensed under the Apache 2.0 license with only minimal requirements for source code [contributions](#signing).

To get started with building the NIM Operator, follow these steps:

```shell
$ git clone <nim-operator-repo>
$ cd k8s-nim-operator
$ make IMG=<image-name> docker-build
```

## <a name="signing"></a>Signing your work

Want to hack on the NVIDIA NIM Operator? Awesome!
We only require you to sign your work, the below section describes this!

The sign-off is a simple line at the end of the explanation for the patch. Your
signature certifies that you wrote the patch or otherwise have the right to pass
it on as an open-source patch. The rules are pretty simple: if you can certify
the below (from [developercertificate.org](http://developercertificate.org/)):

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.
1 Letterman Drive
Suite D4700
San Francisco, CA, 94129

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.

Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

Then you just add a line to every git commit message:

    Signed-off-by: Joe Smith <joe.smith@email.com>

Use your real name (sorry, no pseudonyms or anonymous contributions.)

If you set your `user.name` and `user.email` git configs, you can sign your
commit automatically with `git commit -s`.

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
