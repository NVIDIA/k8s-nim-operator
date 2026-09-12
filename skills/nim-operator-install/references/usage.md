# Usage

Use this reference when a user asks how to invoke the install skill, wants example prompts, or needs human/CI commands without an agent.

## Example Prompts

Dry run only:

```text
Use the NIM Operator install skill to dry-run installation on my Kubernetes cluster. Show preflight checks, available chart versions, selected version, and Helm dry-run output. Do not install anything.
```

Install latest public chart:

```text
Use the NIM Operator install skill to install NIM Operator from the public NVIDIA Helm repo. Check prerequisites first, ask me which chart version to use, and do not run mutating commands until I approve.
```

Install a specific version:

```text
Use the NIM Operator install skill to install NIM Operator version <version>. Verify that version exists in the NVIDIA Helm repo before installing.
```

Install from the local chart:

```text
Use the NIM Operator install skill to install from the local chart in this repo. Show me the local chart version and ask before installing.
```

Install with Dynamo:

```text
Use the NIM Operator install skill to install NIM Operator with Dynamo enabled. Ask before enabling any Dynamo sub-options.
```

Validate an existing install:

```text
Use the NIM Operator install skill to validate the current NIM Operator installation. Run only read-only checks and summarize release, pods, CRDs, GPU Operator, cert-manager, and KServe status.
```

Upgrade:

```text
Use the NIM Operator install skill to upgrade my existing NIM Operator release. Show the current version, available versions, selected target version, preserved Helm values, and ask before upgrading.
```

Remote cluster through SSH:

```text
Use the NIM Operator install skill against my remote Kubernetes host <user>@<host>. Run commands over SSH, show every command before running it, and do not install until I approve.
```

## Manual CLI Usage

Run local commands from the repository root and ensure `kubectl` points at the target cluster before running any Helm command.

Validation only:

```sh
skills/nim-operator-install/scripts/validate-nim-operator-install.sh
```

Validation with overrides:

```sh
NIM_OPERATOR_RELEASE=nim-operator \
NIM_OPERATOR_NAMESPACE=nim-operator \
GPU_OPERATOR_NAMESPACE=gpu-operator \
skills/nim-operator-install/scripts/validate-nim-operator-install.sh
```

Public chart install or upgrade:

```sh
helm repo add nvidia https://helm.ngc.nvidia.com/nvidia
helm repo update
helm search repo nvidia/k8s-nim-operator --versions
selected_version="REPLACE_WITH_VERSION_FROM_SEARCH_OUTPUT"
skills/nim-operator-install/scripts/validate-nim-operator-install.sh
helm upgrade --install nim-operator nvidia/k8s-nim-operator \
  --namespace nim-operator \
  --create-namespace \
  --version "${selected_version}" \
  --set operator.admissionController.enabled=false
skills/nim-operator-install/scripts/validate-nim-operator-install.sh
```

To dry-run instead of installing, add `--dry-run --debug` to the `helm upgrade --install` command. To enable Dynamo, append `--set dynamo.enabled=true` and only add Dynamo sub-options if they are intentionally selected.

Local chart install or upgrade:

```sh
helm show chart deployments/helm/k8s-nim-operator
skills/nim-operator-install/scripts/validate-nim-operator-install.sh
helm upgrade --install nim-operator deployments/helm/k8s-nim-operator \
  --namespace nim-operator \
  --create-namespace
skills/nim-operator-install/scripts/validate-nim-operator-install.sh
```

Remote SSH usage if the skill folder has been copied to the remote host:

```sh
ssh <user>@<host> '~/skills/nim-operator-install/scripts/validate-nim-operator-install.sh'
ssh <user>@<host> 'helm repo add nvidia https://helm.ngc.nvidia.com/nvidia'
ssh <user>@<host> 'helm repo update'
ssh <user>@<host> 'helm search repo nvidia/k8s-nim-operator --versions'
ssh <user>@<host> 'selected_version="REPLACE_WITH_VERSION_FROM_SEARCH_OUTPUT"; helm upgrade --install nim-operator nvidia/k8s-nim-operator --namespace nim-operator --create-namespace --version "${selected_version}" --set operator.admissionController.enabled=false'
ssh <user>@<host> '~/skills/nim-operator-install/scripts/validate-nim-operator-install.sh'
```
