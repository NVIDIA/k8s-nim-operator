---
name: nim-operator-install
description: Install NVIDIA NIM Operator on Kubernetes with prerequisite checks, optional NVIDIA GPU Operator dependency installation, public or local Helm chart selection, optional Dynamo support, and optional KServe compatibility verification. Use when a customer wants to install or upgrade the NIM Operator itself, with or without Dynamo and KServe, but does not want to deploy a NIM inference model yet.
---

# NVIDIA NIM Operator Install

Use this skill to install or upgrade the NVIDIA NIM Operator on an existing Kubernetes cluster. The skill installs the operator and its CRDs only. Do not deploy `NIMService`, `NIMCache`, `NIMPipeline`, or NeMo service custom resources unless the user explicitly asks for that as a separate task.

This is the canonical, agent-neutral skill folder. If another agent framework needs a specific discovery path, create a thin adapter or symlink to this folder instead of duplicating the workflow.

## Workspace Root

Assume commands run from the root of the `k8s-nim-operator` repository unless the user gives another working directory. Before using repo-relative paths such as `.agents/skills/...` or `deployments/helm/k8s-nim-operator`, verify the current directory:

```sh
pwd
test -f deployments/helm/k8s-nim-operator/Chart.yaml
test -f .agents/skills/nim-operator-install/SKILL.md
```

If these checks fail, ask for the correct repository root or `cd` to it before continuing.

## Safety Contract

Run read-only discovery before proposing any cluster-changing command. Before running any mutating command, print the exact commands, summarize the expected impact, and ask for confirmation.

Read-only examples: `kubectl get`, `kubectl describe`, `kubectl auth can-i`, `helm version`, `helm list`, `helm search repo`, `helm status`, `helm get values`.

Mutating examples: `helm repo add`, `helm repo update`, `helm dependency update`, `helm upgrade --install`, `kubectl create`, `kubectl apply`, `kubectl patch`, `kubectl delete`.

## Defaults

- NIM Operator release: `nim-operator`
- NIM Operator namespace: `nim-operator`
- Public NVIDIA Helm repo name: `nvidia`
- Public NVIDIA Helm repo URL: `https://helm.ngc.nvidia.com/nvidia`
- Local NIM Operator chart path: `deployments/helm/k8s-nim-operator`
- NIM Operator chart version: latest available from the selected chart source unless the user requests a specific version
- GPU Operator: verify first; ask before installing if missing
- GPU Operator namespace: `gpu-operator`
- Dynamo: disabled unless requested
- KServe: verify only; do not install KServe from this skill

## References

- For validation levels, command safety checks, and evidence format, read `references/validation.md`.
- For a read-only validation helper from the repository root, run `.agents/skills/nim-operator-install/scripts/validate-nim-operator-install.sh`.

## How To Ask For This Skill

End users do not need to know the internal file layout. They should ask the agent for the outcome they want. Recognize and support these prompt patterns:

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

This section is for humans, CI jobs, and reviewers who want to run the same workflow without an agent. Run local commands from the repository root and ensure `kubectl` points at the target cluster before running any Helm command.

Validation only:

```sh
.agents/skills/nim-operator-install/scripts/validate-nim-operator-install.sh
```

Validation with overrides:

```sh
NIM_OPERATOR_RELEASE=nim-operator \
NIM_OPERATOR_NAMESPACE=nim-operator \
GPU_OPERATOR_NAMESPACE=gpu-operator \
.agents/skills/nim-operator-install/scripts/validate-nim-operator-install.sh
```

Public chart install or upgrade:

```sh
helm repo add nvidia https://helm.ngc.nvidia.com/nvidia
helm repo update
helm search repo nvidia/k8s-nim-operator --versions
selected_version="REPLACE_WITH_VERSION_FROM_SEARCH_OUTPUT"
.agents/skills/nim-operator-install/scripts/validate-nim-operator-install.sh
helm upgrade --install nim-operator nvidia/k8s-nim-operator \
  --namespace nim-operator \
  --create-namespace \
  --version "${selected_version}" \
  --set operator.admissionController.enabled=false
.agents/skills/nim-operator-install/scripts/validate-nim-operator-install.sh
```

To dry-run instead of installing, add `--dry-run --debug` to the `helm upgrade --install` command. To enable Dynamo, append `--set dynamo.enabled=true` and only add Dynamo sub-options if they are intentionally selected.

Local chart install or upgrade:

```sh
helm show chart deployments/helm/k8s-nim-operator
.agents/skills/nim-operator-install/scripts/validate-nim-operator-install.sh
helm upgrade --install nim-operator deployments/helm/k8s-nim-operator \
  --namespace nim-operator \
  --create-namespace
.agents/skills/nim-operator-install/scripts/validate-nim-operator-install.sh
```

Remote SSH usage if the skill folder has been copied to the remote host:

```sh
ssh <user>@<host> '~/.agents/skills/nim-operator-install/scripts/validate-nim-operator-install.sh'
ssh <user>@<host> 'helm repo add nvidia https://helm.ngc.nvidia.com/nvidia'
ssh <user>@<host> 'helm repo update'
ssh <user>@<host> 'helm search repo nvidia/k8s-nim-operator --versions'
ssh <user>@<host> 'selected_version="REPLACE_WITH_VERSION_FROM_SEARCH_OUTPUT"; helm upgrade --install nim-operator nvidia/k8s-nim-operator --namespace nim-operator --create-namespace --version "${selected_version}" --set operator.admissionController.enabled=false'
ssh <user>@<host> '~/.agents/skills/nim-operator-install/scripts/validate-nim-operator-install.sh'
```

## Initial Questions

Ask only for missing choices that materially affect the install:

1. Use the public NVIDIA Helm repo or the local chart from the current checkout?
2. Enable Dynamo? If yes, should any Dynamo sub-options be set?
3. Verify KServe compatibility? If yes, verify KServe is already installed, but do not install it.
4. If GPU Operator is absent, should this skill install it as a dependency?
5. Namespace, release name, version, or chart path overrides.

If the user wants a quick default install, use public repo, latest available chart version, namespace `nim-operator`, release `nim-operator`, Dynamo disabled, KServe verification disabled, and GPU Operator install only if the user approves after the prerequisite check. Tell the user which version will be installed and ask whether they want a different version before running Helm.

## Version Selection

For public chart installs and upgrades, never leave `<selected-version>` unresolved. Discover versions first:

```sh
helm repo add nvidia https://helm.ngc.nvidia.com/nvidia
helm repo update
helm search repo nvidia/k8s-nim-operator --versions
```

Use the first version returned by `helm search repo ... --versions` as the latest candidate, then ask:

```text
I found latest NIM Operator chart version <latest-version>. Do you want to install this version, or should I use a different version?
```

If the user accepts the latest version, set `selected_version=<latest-version>`. If the user provides another version, verify that version appears in the `helm search repo` output before using it. If it does not appear, stop and ask the user to choose one of the available versions.

For local chart installs and upgrades, inspect the local chart:

```sh
helm show chart deployments/helm/k8s-nim-operator
```

Tell the user the local chart `version` and `appVersion`, then ask whether to proceed with that local chart or switch to the public chart flow.

## Dry-Run Demo Flow

When the user asks for a dry run or demo, do not install anything. Start by calling the bundled validation helper so preflight evidence is captured before chart discovery or Helm rendering. Use this sequence:

```sh
.agents/skills/nim-operator-install/scripts/validate-nim-operator-install.sh
helm repo add nvidia https://helm.ngc.nvidia.com/nvidia
helm repo update
helm search repo nvidia/k8s-nim-operator --versions
helm template nim-operator nvidia/k8s-nim-operator \
  --namespace nim-operator \
  --version <selected-version> \
  <approved-values>
helm upgrade --install nim-operator nvidia/k8s-nim-operator \
  --namespace nim-operator \
  --create-namespace \
  --version <selected-version> \
  <approved-values> \
  --dry-run --debug
```

Before rendering or dry-running Helm, resolve `<selected-version>` through the Version Selection flow and replace `<approved-values>` with the exact values the user approved, such as `--set operator.admissionController.enabled=false` or `--set dynamo.enabled=true`.

## Prerequisite Checks

Start prerequisite discovery by calling the bundled validation helper:

```sh
.agents/skills/nim-operator-install/scripts/validate-nim-operator-install.sh
```

This is the canonical preflight call site for the skill. It checks client tools, cluster access, RBAC, nodes, GPU availability, GPU Operator status, cert-manager status, KServe presence, current NIM Operator state, and NIM Operator CRDs.

If the helper is unavailable or a narrower manual check is needed, run these read-only checks before proposing install commands:

```sh
kubectl config current-context
kubectl cluster-info
kubectl auth can-i create customresourcedefinitions.apiextensions.k8s.io
kubectl auth can-i create clusterroles.rbac.authorization.k8s.io
helm version
kubectl get nodes
kubectl get nodes -o custom-columns=NAME:.metadata.name,GPUS:.status.allocatable.nvidia\.com/gpu
kubectl get ns gpu-operator
kubectl get pods -n gpu-operator
kubectl get clusterpolicies.nvidia.com
kubectl get ns cert-manager
kubectl get pods -n cert-manager
```

Interpret the results:

- Cluster-admin or equivalent privileges are normally required because the charts install CRDs and cluster-scoped RBAC.
- NVIDIA GPU Operator is required for GPU-backed NIM workloads. If it is missing, ask whether to install it before installing NIM Operator.
- GPU allocatable resources may be absent before GPU Operator is installed or ready. After GPU Operator installation, verify `nvidia.com/gpu` capacity and allocatable resources.
- `cert-manager` is required when `operator.admissionController.enabled=true` and `operator.admissionController.tls.mode=cert-manager`, which are NIM Operator chart defaults. If cert-manager is absent, either stop and ask the user to install it, or propose `--set operator.admissionController.enabled=false` only if the customer accepts disabling the admission controller.

For KServe verification, also run:

```sh
kubectl get crd inferenceservices.serving.kserve.io
kubectl get pods -n kserve
```

If KServe is absent, do not install it. Tell the user KServe-backed `NIMService` resources will not work until KServe is installed.

## GPU Operator Dependency

If GPU Operator is already installed and `clusterpolicies.nvidia.com` reports ready, do not reinstall it.

If GPU Operator is absent, ask before installing it. Use the public NVIDIA Helm repo unless the user provides a different source:

```sh
helm repo add nvidia https://helm.ngc.nvidia.com/nvidia
helm repo update
helm upgrade --install gpu-operator nvidia/gpu-operator \
  --namespace gpu-operator \
  --create-namespace
```

After installing GPU Operator, verify:

```sh
kubectl rollout status deployment/gpu-operator -n gpu-operator --timeout=300s
kubectl get pods -n gpu-operator
kubectl get clusterpolicies.nvidia.com
kubectl describe node <gpu-node-name>
```

Treat confidential computing, MIG partitioning, DRA, driver preinstallation, proxy settings, and air-gapped installation as advanced GPU Operator scenarios. Pause and ask for the customer's desired mode before adding chart values for those cases.

## NIM Operator Install Commands

Build one command block for the selected path, then ask before executing.

### Public Helm Repo

Discover available versions first:

```sh
helm repo add nvidia https://helm.ngc.nvidia.com/nvidia
helm repo update
helm search repo nvidia/k8s-nim-operator --versions
```

Tell the user which chart version appears latest and ask whether to use it or a specific version.

```sh
helm repo add nvidia https://helm.ngc.nvidia.com/nvidia
helm repo update
helm upgrade --install nim-operator nvidia/k8s-nim-operator \
  --namespace nim-operator \
  --create-namespace \
  --version <selected-version>
```

### Local Chart

For local chart installs, read the local chart version and app version:

```sh
helm show chart deployments/helm/k8s-nim-operator
```

Tell the user the local chart version and ask whether to proceed with the local checkout or use the public chart instead.

These local chart commands assume the shell is running from the repository root.

```sh
helm upgrade --install nim-operator deployments/helm/k8s-nim-operator \
  --namespace nim-operator \
  --create-namespace
```

If local install uses Dynamo, run this first because Dynamo is a chart dependency:

```sh
helm dependency update deployments/helm/k8s-nim-operator
```

## Dynamo Options

For basic Dynamo support, append:

```sh
--set dynamo.enabled=true
```

Expose these Dynamo sub-options only when the customer asks for advanced Dynamo configuration:

```sh
--set dynamo.grove.enabled=true
--set dynamo.kai-scheduler.enabled=true
```

Do not enable Dynamo or its sub-options by default.

## Admission Controller Choice

If cert-manager is missing and the user wants to proceed without it, append:

```sh
--set operator.admissionController.enabled=false
```

Make it clear that this disables the NIM Operator admission controller. Do not silently add this flag.

## Verification

After install or upgrade, call the bundled validation helper again to collect post-change evidence:

```sh
.agents/skills/nim-operator-install/scripts/validate-nim-operator-install.sh
```

Then run rollout-specific verification:

```sh
helm status nim-operator -n nim-operator
helm get values nim-operator -n nim-operator
kubectl get deployment -n nim-operator -l app.kubernetes.io/instance=nim-operator,app.kubernetes.io/name=k8s-nim-operator
kubectl rollout status deployment/<deployment-name-from-previous-command> -n nim-operator --timeout=180s
kubectl get pods -n nim-operator
kubectl get crd | grep -E 'apps.nvidia.com|nvidia.com'
```

Do not hardcode the deployment name. Helm renders it from the release name and chart name, so the default release usually creates `nim-operator-k8s-nim-operator`.

Verify these NIM Operator CRDs are present:

- `nimservices.apps.nvidia.com`
- `nimcaches.apps.nvidia.com`
- `nimpipelines.apps.nvidia.com`
- `nimbuilds.apps.nvidia.com`
- `nemodatastores.apps.nvidia.com`
- `nemoentitystores.apps.nvidia.com`
- `nemocustomizers.apps.nvidia.com`
- `nemoevaluators.apps.nvidia.com`
- `nemoguardrails.apps.nvidia.com`

If Dynamo is enabled, also verify:

```sh
kubectl get crd | grep -i dynamo
kubectl get pods -n nim-operator
```

If KServe compatibility was requested, repeat:

```sh
kubectl get crd inferenceservices.serving.kserve.io
kubectl get pods -n kserve
```

## Upgrade Flow

Use the same `helm upgrade --install` command for both fresh installs and upgrades. Before upgrading, call the validation helper to capture the pre-upgrade baseline:

```sh
.agents/skills/nim-operator-install/scripts/validate-nim-operator-install.sh
```

Then inspect the current release and available target versions:

```sh
helm status nim-operator -n nim-operator
helm get values nim-operator -n nim-operator
helm list -n nim-operator
helm search repo nvidia/k8s-nim-operator --versions
```

Then explain:

- current installed chart/app version, if present
- target version
- values that will be preserved or changed
- whether CRDs will be upgraded by the chart
- expected rollout behavior

For public chart upgrades, include `--version <selected-version>`. For local chart upgrades, use the local chart path. Preserve user-approved values such as `--set operator.admissionController.enabled=false` or `--set dynamo.enabled=true` unless the user asks to change them.

After the upgrade, run the normal verification flow and compare the new Helm status and controller rollout with the pre-upgrade state.

## Failure Handling

- If Helm cannot find a public chart, run `helm search repo nvidia/<chart-name> --versions` after `helm repo update` and ask whether to use an available version.
- If GPU Operator is missing, ask whether to install it before NIM Operator. Do not assume CPU-only operation is acceptable for future NIM workloads.
- If GPU Operator is installed but no GPU is allocatable, inspect node labels, device plugin pods, and `kubectl describe node`; do not proceed to model deployment.
- If cert-manager is missing, explain that the default webhook TLS mode expects cert-manager. Do not silently disable the admission controller.
- If KServe is missing, do not install it. State that future `NIMService` resources using `spec.inferencePlatform: kserve` need KServe installed first.
- If CRDs already exist, prefer `helm upgrade --install` and keep `operator.upgradeCRD=true` unless the user requests otherwise.
