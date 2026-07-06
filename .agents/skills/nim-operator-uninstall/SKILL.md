---
name: nim-operator-uninstall
description: Safely uninstall NVIDIA NIM Operator from Kubernetes with inventory checks, explicit approval gates for destructive actions, optional custom resource cleanup, optional CRD removal, and post-uninstall validation. Use when a customer wants to remove or clean up the NIM Operator itself, not the GPU Operator or unrelated cluster dependencies.
---

# NVIDIA NIM Operator Uninstall

Use this skill to remove the NVIDIA NIM Operator Helm release from a Kubernetes cluster. This skill is intentionally separate from install because uninstall is destructive and needs stronger confirmation.

By default, uninstall only removes the NIM Operator Helm release. Do not delete NIM custom resources, CRDs, namespaces, persistent volumes, secrets, GPU Operator, cert-manager, KServe, or Dynamo dependencies unless the user explicitly approves that specific action.

## Workspace Root

Assume commands run from the root of the `k8s-nim-operator` repository unless the user gives another working directory. Before using repo-relative paths such as `.agents/skills/...`, verify the current directory:

```sh
pwd
test -f .agents/skills/nim-operator-uninstall/SKILL.md
```

If this check fails, ask for the correct repository root or `cd` to it before continuing.

## Safety Contract

Run read-only inventory before proposing any destructive command. Before running each destructive step, print the exact command, summarize what will be removed, and ask for confirmation.

Read-only examples: `kubectl get`, `kubectl describe`, `helm list`, `helm status`, `helm get values`.

Destructive examples: `helm uninstall`, `kubectl delete`, namespace deletion, CRD deletion.

## Defaults

- NIM Operator release: `nim-operator`
- NIM Operator namespace: `nim-operator`
- Keep CRDs by default
- Keep custom resources by default
- Keep namespace by default
- Keep GPU Operator by default
- Keep cert-manager and KServe by default

## References

- For validation levels, inventory checks, and evidence format, read `references/validation.md`.
- For a read-only validation helper from the repository root, run `.agents/skills/nim-operator-uninstall/scripts/validate-nim-operator-uninstall.sh`.

## How To Ask For This Skill

End users do not need to know the internal file layout. They should ask the agent for the cleanup outcome they want. Recognize and support these prompt patterns:

Inventory only:

```text
Use the NIM Operator uninstall skill to inventory the current installation. Do not delete anything.
```

Safe default uninstall:

```text
Use the NIM Operator uninstall skill to uninstall the NIM Operator Helm release. Preserve CRDs, custom resources, namespace, GPU Operator, cert-manager, and KServe unless I explicitly approve deleting them.
```

Uninstall a specific release or namespace:

```text
Use the NIM Operator uninstall skill to remove release <release> from namespace <namespace>. Inventory resources first and ask before uninstalling.
```

Full API cleanup:

```text
Use the NIM Operator uninstall skill to remove the Helm release and then ask me whether to delete NIM Operator CRDs. Show existing custom resources before deleting any CRDs.
```

Validate after uninstall:

```text
Use the NIM Operator uninstall skill to validate that the operator release and controller pods are gone. Tell me which CRDs and custom resources remain.
```

Remote cluster through SSH:

```text
Use the NIM Operator uninstall skill against my remote Kubernetes host <user>@<host>. Run commands over SSH, show every command before running it, and do not delete anything until I approve.
```

## Manual CLI Usage

This section is for humans, CI jobs, and reviewers who want to run the same workflow without an agent. Run local commands from the repository root and ensure `kubectl` points at the target cluster before running `helm uninstall`.

Inventory only:

```sh
.agents/skills/nim-operator-uninstall/scripts/validate-nim-operator-uninstall.sh
```

Inventory with overrides:

```sh
NIM_OPERATOR_RELEASE=nim-operator \
NIM_OPERATOR_NAMESPACE=nim-operator \
.agents/skills/nim-operator-uninstall/scripts/validate-nim-operator-uninstall.sh
```

Safe default uninstall. This removes only the Helm release and preserves CRDs, custom resources, namespace, GPU Operator, cert-manager, KServe, and Dynamo dependencies:

```sh
.agents/skills/nim-operator-uninstall/scripts/validate-nim-operator-uninstall.sh
helm uninstall nim-operator -n nim-operator
.agents/skills/nim-operator-uninstall/scripts/validate-nim-operator-uninstall.sh
helm list -n nim-operator
kubectl get pods -n nim-operator
```

Remote SSH usage if the skill folder has been copied to the remote host:

```sh
ssh <user>@<host> '~/.agents/skills/nim-operator-uninstall/scripts/validate-nim-operator-uninstall.sh'
ssh <user>@<host> 'helm uninstall nim-operator -n nim-operator'
ssh <user>@<host> '~/.agents/skills/nim-operator-uninstall/scripts/validate-nim-operator-uninstall.sh'
ssh <user>@<host> 'helm list -n nim-operator'
ssh <user>@<host> 'kubectl get pods -n nim-operator'
```

## Initial Questions

Ask only for missing choices that materially affect removal:

1. Which release and namespace should be uninstalled?
2. Should existing NIM and NeMo custom resources be deleted first, or preserved?
3. Should NIM Operator CRDs be deleted after Helm uninstall, or preserved?
4. Should the namespace be deleted after cleanup, or preserved?

If the user wants a quick default uninstall, uninstall only the `nim-operator` Helm release from namespace `nim-operator` and preserve CRDs, custom resources, namespace, GPU Operator, cert-manager, and KServe.

## Inventory Checks

Start inventory by calling the bundled validation helper:

```sh
.agents/skills/nim-operator-uninstall/scripts/validate-nim-operator-uninstall.sh
```

This is the canonical pre-uninstall call site for the skill. It checks client tools, cluster access, the Helm release, operator namespace resources, NIM Operator CRDs, and any NIM/NeMo custom resources.

If the helper is unavailable or a narrower manual check is needed, run these read-only checks before proposing uninstall commands:

```sh
kubectl config current-context
kubectl cluster-info
helm list -A | grep nim-operator
helm status nim-operator -n nim-operator
helm get values nim-operator -n nim-operator
kubectl get pods -n nim-operator
kubectl get deployment -n nim-operator -l app.kubernetes.io/instance=nim-operator,app.kubernetes.io/name=k8s-nim-operator
kubectl get crd | grep -E 'apps.nvidia.com'
```

Inventory NIM and NeMo custom resources across all namespaces:

```sh
kubectl get nimservices.apps.nvidia.com -A
kubectl get nimcaches.apps.nvidia.com -A
kubectl get nimpipelines.apps.nvidia.com -A
kubectl get nimbuilds.apps.nvidia.com -A
kubectl get nemodatastores.apps.nvidia.com -A
kubectl get nemoentitystores.apps.nvidia.com -A
kubectl get nemocustomizers.apps.nvidia.com -A
kubectl get nemoevaluators.apps.nvidia.com -A
kubectl get nemoguardrails.apps.nvidia.com -A
```

If any custom resources exist, warn that deleting CRDs will delete or orphan API access to those resources. Ask whether the user wants to delete custom resources first.

## Uninstall Helm Release

After user approval, uninstall only the Helm release:

```sh
helm uninstall nim-operator -n nim-operator
```

Then call the bundled validation helper again to collect post-uninstall evidence:

```sh
.agents/skills/nim-operator-uninstall/scripts/validate-nim-operator-uninstall.sh
```

Also verify the key release and controller resources directly:

```sh
helm list -n nim-operator
kubectl get pods -n nim-operator
kubectl get deployment -n nim-operator -l app.kubernetes.io/instance=nim-operator,app.kubernetes.io/name=k8s-nim-operator
```

If Helm reports the release is not found, do not treat that as success automatically. Check whether operator resources still exist in the namespace.

## Optional Custom Resource Cleanup

Only if the user explicitly approves deleting NIM and NeMo custom resources, show and run targeted deletes. Prefer deleting specific resources the user selected. If the user approves deleting all NIM Operator custom resources, use:

```sh
kubectl delete nimservices.apps.nvidia.com --all -A
kubectl delete nimcaches.apps.nvidia.com --all -A
kubectl delete nimpipelines.apps.nvidia.com --all -A
kubectl delete nimbuilds.apps.nvidia.com --all -A
kubectl delete nemodatastores.apps.nvidia.com --all -A
kubectl delete nemoentitystores.apps.nvidia.com --all -A
kubectl delete nemocustomizers.apps.nvidia.com --all -A
kubectl delete nemoevaluators.apps.nvidia.com --all -A
kubectl delete nemoguardrails.apps.nvidia.com --all -A
```

Warn that this may remove model-serving workloads, caches, jobs, and service state owned by those custom resources.

## Optional CRD Cleanup

Keep CRDs by default. Delete CRDs only if the user explicitly approves full API cleanup.

```sh
kubectl delete crd \
  nimservices.apps.nvidia.com \
  nimcaches.apps.nvidia.com \
  nimpipelines.apps.nvidia.com \
  nimbuilds.apps.nvidia.com \
  nemodatastores.apps.nvidia.com \
  nemoentitystores.apps.nvidia.com \
  nemocustomizers.apps.nvidia.com \
  nemoevaluators.apps.nvidia.com \
  nemoguardrails.apps.nvidia.com
```

Before deleting CRDs, re-run custom resource inventory. If custom resources still exist, ask again before proceeding.

## Optional Namespace Cleanup

Keep the namespace by default. Delete it only if the user explicitly approves and it contains no resources the user wants to preserve:

```sh
kubectl get all -n nim-operator
kubectl delete namespace nim-operator
```

## Do Not Remove These By Default

Do not uninstall these from this skill unless the user explicitly asks for a broader cluster cleanup workflow:

- NVIDIA GPU Operator
- cert-manager
- KServe
- Dynamo dependencies that may be shared
- image pull secrets
- NGC API secrets
- persistent volumes or storage classes

## Post-Uninstall Validation

Run the bundled validation helper:

```sh
.agents/skills/nim-operator-uninstall/scripts/validate-nim-operator-uninstall.sh
```

If a manual spot-check is needed, run:

```sh
helm list -n nim-operator
kubectl get pods -n nim-operator
kubectl get deployment -n nim-operator -l app.kubernetes.io/instance=nim-operator,app.kubernetes.io/name=k8s-nim-operator
kubectl get crd | grep -E 'apps.nvidia.com'
```

Report:

- whether the Helm release is gone
- whether operator pods/deployments are gone
- whether CRDs were preserved or deleted
- whether custom resources remain
- what dependencies remain intentionally installed
