# Validation

Use this reference to validate an uninstall plan and the final cluster state after NIM Operator removal.

## Package Validation

```sh
test -f .agents/skills/nim-operator-uninstall/SKILL.md
grep -q '^name: nim-operator-uninstall$' .agents/skills/nim-operator-uninstall/SKILL.md
grep -q '^description:' .agents/skills/nim-operator-uninstall/SKILL.md
test -f .agents/skills/nim-operator-uninstall/agents/openai.yaml
test -f .agents/skills/nim-operator-uninstall/references/validation.md
test -x .agents/skills/nim-operator-uninstall/scripts/validate-nim-operator-uninstall.sh
```

## Plan Validation

Before uninstalling, verify that the agent has:

- identified the current Kubernetes context
- identified the Helm release and namespace
- inventoried NIM and NeMo custom resources
- stated whether CRDs will be preserved or deleted
- stated whether the namespace will be preserved or deleted
- separated Helm uninstall from CRD deletion
- asked for approval before every destructive step

## Read-Only Inventory

```sh
kubectl config current-context
kubectl cluster-info
helm list -A | grep nim-operator
helm status nim-operator -n nim-operator
helm get values nim-operator -n nim-operator
kubectl get pods -n nim-operator
kubectl get crd | grep -E 'apps.nvidia.com'
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

## Post-Uninstall Validation

```sh
helm list -n nim-operator
kubectl get pods -n nim-operator
kubectl get deployment -n nim-operator -l app.kubernetes.io/instance=nim-operator,app.kubernetes.io/name=k8s-nim-operator
kubectl get crd | grep -E 'apps.nvidia.com'
```

Expected default result:

- Helm release is gone.
- Operator deployment and pods are gone.
- CRDs are still present unless user approved CRD deletion.
- Custom resources are still present unless user approved custom resource deletion.
- GPU Operator, cert-manager, KServe, and shared dependencies are untouched.

## Evidence Format

```text
Cluster:
Context:
Release:
Namespace:
Pre-uninstall custom resources:
Uninstall command approved:
CRD deletion approved:
Namespace deletion approved:
Post-uninstall Helm state:
Post-uninstall pods/deployments:
Remaining CRDs:
Remaining custom resources:
Dependencies intentionally preserved:
Validation date:
Residual risks:
```
