# Validation

Use this reference when validating the skill package, a dry-run plan, or a live NIM Operator installation.

## Validation Levels

### Package Validation

Run before submitting to NVCARPS or another shared registry:

```sh
test -f .agents/skills/nim-operator-install/SKILL.md
grep -q '^name: nim-operator-install$' .agents/skills/nim-operator-install/SKILL.md
grep -q '^description:' .agents/skills/nim-operator-install/SKILL.md
test -f .agents/skills/nim-operator-install/agents/openai.yaml
test -f .agents/skills/nim-operator-install/references/validation.md
test -x .agents/skills/nim-operator-install/scripts/validate-nim-operator-install.sh
```

Review the skill content for:

- No hardcoded customer IP addresses, hostnames, tokens, usernames, passwords, kubeconfigs, or NGC API keys.
- No commands that mutate a cluster without an explicit ask-before-run instruction.
- No Codex-only discovery assumption in the canonical workflow.
- Public and local chart paths both represented.
- GPU Operator, cert-manager, Dynamo, and KServe decisions documented.

### Command Safety Validation

For any generated install plan, separate commands into read-only and mutating groups. The agent may run read-only commands directly. The agent must show mutating commands and wait for confirmation.

Read-only commands include:

```sh
kubectl config current-context
kubectl cluster-info
kubectl auth can-i ...
kubectl get ...
kubectl describe ...
helm version
helm list
helm status
helm get values
helm search repo
```

Mutating commands include:

```sh
helm repo add
helm repo update
helm dependency update
helm upgrade --install
kubectl create
kubectl apply
kubectl patch
kubectl delete
```

### Dry-Run Workflow Validation

Validate that the skill can produce an install plan without changing the cluster:

```sh
kubectl config current-context
kubectl cluster-info
kubectl auth can-i create customresourcedefinitions.apiextensions.k8s.io
kubectl auth can-i create clusterroles.rbac.authorization.k8s.io
helm version
kubectl get nodes
kubectl get ns gpu-operator
kubectl get clusterpolicies.nvidia.com
kubectl get ns cert-manager
kubectl get crd inferenceservices.serving.kserve.io
helm search repo nvidia/k8s-nim-operator --versions
```

Expected dry-run output:

- Cluster context and API server identified.
- Cluster-scoped permission check summarized.
- GPU Operator state summarized as present, missing, or unhealthy.
- cert-manager state summarized.
- KServe state summarized only if requested.
- Available NIM Operator chart versions summarized.
- The agent asks the user to accept the latest chart version or provide a specific version.
- The chosen version appears in `helm search repo nvidia/k8s-nim-operator --versions`.
- Exact proposed Helm commands shown before execution.

### Helm Render and Dry-Run Validation

Use this when the user wants to see what would happen without installing:

```sh
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

Expected result:

- `<selected-version>` has been replaced with the user-approved chart version.
- `<approved-values>` has been replaced or removed; no placeholder remains in the command.
- `helm template` renders Kubernetes YAML without applying it.
- `helm upgrade --install --dry-run --debug` simulates the release without installing it.
- The agent explains that the skill files are instructions, while Helm renders and applies the Kubernetes resources.

### GPU Operator Validation

When GPU Operator is present or installed by the workflow, verify:

```sh
helm status gpu-operator -n gpu-operator
kubectl get pods -n gpu-operator
kubectl get clusterpolicies.nvidia.com
kubectl get nodes -o custom-columns=NAME:.metadata.name,GPUS:.status.allocatable.nvidia\.com/gpu
kubectl describe node <gpu-node-name>
```

Expected result:

- GPU Operator release is deployed, if installed by Helm.
- GPU Operator pods are running or completed as appropriate.
- `clusterpolicies.nvidia.com` reports ready.
- At least one target node reports `nvidia.com/gpu` capacity and allocatable resources for GPU-backed NIM workloads.

### NIM Operator Live Validation

After installing or upgrading NIM Operator, verify:

```sh
helm status nim-operator -n nim-operator
helm get values nim-operator -n nim-operator
helm list -n nim-operator
kubectl get deployment -n nim-operator -l app.kubernetes.io/instance=nim-operator,app.kubernetes.io/name=k8s-nim-operator
kubectl rollout status deployment/<deployment-name> -n nim-operator --timeout=180s
kubectl get pods -n nim-operator
kubectl get crd nimservices.apps.nvidia.com
kubectl get crd nimcaches.apps.nvidia.com
kubectl get crd nimpipelines.apps.nvidia.com
kubectl get crd nimbuilds.apps.nvidia.com
kubectl get crd nemodatastores.apps.nvidia.com
kubectl get crd nemoentitystores.apps.nvidia.com
kubectl get crd nemocustomizers.apps.nvidia.com
kubectl get crd nemoevaluators.apps.nvidia.com
kubectl get crd nemoguardrails.apps.nvidia.com
```

Expected result:

- Helm release status is `deployed`.
- Controller deployment rolls out successfully.
- Controller pod is `Running` and ready.
- All expected NIM and NeMo CRDs exist.
- User-supplied values match the approved plan, for example `operator.admissionController.enabled=false` only when approved.
- Installed chart version matches the user-approved version.

### Optional Dynamo Validation

When Dynamo is enabled, verify:

```sh
helm get values nim-operator -n nim-operator
kubectl get crd | grep -i dynamo
kubectl get pods -n nim-operator
```

Expected result:

- `dynamo.enabled=true` appears in Helm values.
- Dynamo CRDs are present.
- Related pods are running or completed according to the chart behavior.

### Optional KServe Validation

When KServe compatibility is requested, verify only:

```sh
kubectl get crd inferenceservices.serving.kserve.io
kubectl get pods -n kserve
```

Expected result:

- KServe CRD exists.
- KServe controller pods are present and healthy.

Do not install KServe from this skill.

## Evidence Format

When reporting validation, include:

```text
Cluster:
Context:
Kubernetes API:
GPU Operator:
GPU capacity:
cert-manager:
KServe:
NIM Operator release:
NIM Operator deployment:
NIM Operator pod:
CRDs:
Approved non-default values:
Validation date:
Residual risks:
```

## Additional Validation Ideas

These are useful when the environment or release process is stricter:

- `helm template` render validation for public and local chart values before install.
- `helm lint` on the local chart.
- Schema validation of rendered manifests with a Kubernetes policy tool.
- Server-side dry-run for rendered manifests where feasible.
- Image pull validation for private or air-gapped registries.
- RBAC review for cluster-scoped permissions.
- Network/proxy validation for Helm repos, NGC, and image registries.
- Upgrade validation from an existing NIM Operator release.
- Uninstall validation that confirms CRD retention or deletion behavior is intentional.
- Negative validation showing the skill stops when cert-manager is missing and admission controller is still enabled.
