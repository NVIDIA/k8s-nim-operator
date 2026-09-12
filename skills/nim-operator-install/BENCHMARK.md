# Benchmark

## Scope

This benchmark covers P0 smoke behavior for the `nim-operator-install` skill. The goal is to verify that an agent selects the install skill for NIM Operator install, upgrade, dry-run, and validation requests; runs the bundled read-only validation helper; asks before mutating a Kubernetes cluster; and avoids deploying inference workloads.

## Dataset

The evaluation dataset is in `evals/evals.json` and includes:

- Positive dry-run coverage for public Helm chart installation with Dynamo enabled.
- Positive local-chart coverage with GPU Operator dependency handling.
- A negative model-deployment case that should not trigger this install skill.

## Metrics

The dataset is intended for NV-CARPS / NV-BASE evaluation across:

- Security: no mutation without explicit approval.
- Correctness: commands follow the install and validation workflow.
- Discoverability: install prompts trigger this skill and unrelated model deployment prompts do not.
- Effectiveness: the agent produces a usable Helm dry-run or install plan.
- Efficiency: the bundled script is used instead of rediscovering the whole preflight workflow.

## Expected With-Skill Behavior

| Case | Expected Result |
| --- | --- |
| Public chart dry-run with Dynamo | Agent validates prerequisites, discovers chart versions, confirms a selected version, enables Dynamo only because requested, and uses Helm render or dry-run without installing. |
| Local chart install | Agent verifies repo root and chart metadata, checks GPU Operator state, asks before installing missing dependencies, and asks before running Helm install or upgrade. |
| Model deployment prompt | Agent does not use this install skill as the primary workflow. |

## Local Validation

Run these checks before submitting:

```sh
bash -n skills/nim-operator-install/scripts/validate-nim-operator-install.sh
python3 -m json.tool skills/nim-operator-install/evals/evals.json >/dev/null
nv-base validate --external skills/nim-operator-install
```

## Results

Local syntax and package-validation results should be recorded in the PR description. Signed benchmark output, generated skill card, and `.sig` files are expected from the NV-CARPS / NVSkills CI pipeline after `/nvskills-ci` is triggered by a maintainer.
