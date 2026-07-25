# Benchmark

## Scope

This benchmark covers P0 smoke behavior for the `nim-operator-uninstall` skill. The goal is to verify that an agent selects the uninstall skill for NIM Operator cleanup requests; inventories the cluster before destructive actions; preserves CRDs, custom resources, namespaces, and shared dependencies by default; asks whether to delete all CRDs associated with NIM Operator for clean cleanup; and asks before running any delete command.

## Dataset

The evaluation dataset is in `evals/evals.json` and includes:

- Positive inventory-only coverage.
- Positive safe default uninstall coverage.
- Positive clean uninstall coverage with explicit CRD cleanup approval.
- A negative install request that should not trigger this uninstall skill.

## Metrics

The dataset is intended for NV-CARPS / NV-BASE evaluation across:

- Security: destructive commands require explicit approval.
- Correctness: uninstall removes only the Helm release by default.
- Discoverability: uninstall prompts trigger this skill and install prompts do not.
- Effectiveness: the agent reports what was removed and what remains.
- Efficiency: the bundled script is used for inventory and post-uninstall validation.

## Expected With-Skill Behavior

| Case | Expected Result |
| --- | --- |
| Inventory only | Agent runs read-only inventory and does not delete anything. |
| Safe default uninstall | Agent asks before `helm uninstall`, preserves CRDs/custom resources/dependencies, and validates after uninstall. |
| Clean CRD cleanup | Agent validates after Helm uninstall, asks whether to delete all NIM Operator CRDs, and deletes them only after approval. |
| Install prompt | Agent does not use this uninstall skill as the primary workflow. |

## Local Validation

Run these checks before submitting:

```sh
bash -n skills/nim-operator-uninstall/scripts/validate-nim-operator-uninstall.sh
python3 -m json.tool skills/nim-operator-uninstall/evals/evals.json >/dev/null
nv-base validate --external skills/nim-operator-uninstall
```

## Results

Local syntax and package-validation results should be recorded in the PR description. Signed benchmark output, generated skill card, and `.sig` files are expected from the NV-CARPS / NVSkills CI pipeline after `/nvskills-ci` is triggered by a maintainer.
