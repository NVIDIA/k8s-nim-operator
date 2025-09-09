# NeMo Custom Resources

These CRs are designed to deploy NeMo microservices using the NIM Operator.

## Compatible NIM Operator Version

- **NIM Operator v3.0.0**

> Using these CRs with any other version may lead to validation or runtime errors.

## Notes

- The CR schema and fields in this version match the capabilities of NIM Operator v2.0.2.

## Upgrade Notes

If upgrading from a previous NeMo service version (e.g., `25.08`) using the existing operator version:
- Check for renamed or deprecated fields.
- Review updated model config parameters.
- Revalidate against the new CR using:

  ```bash
  kubectl apply --dry-run=server -f apps_v1alpha1_nemodatastore.yaml \
    -f apps_v1alpha1_nemocustomizer.yaml \
    -f apps_v1alpha1_nemoentitystore.yaml \
    -f apps_v1alpha1_nemoguardrails.yaml \
    -f apps_v1alpha1_nemoevaluator.yaml
  ```

  ```text
  nemodatastore.apps.nvidia.com/nemodatastore-sample created (server dry run)
  nemocustomizer.apps.nvidia.com/nemocustomizer-sample created (server dry run)
  nemoentitystore.apps.nvidia.com/nemoentitystore-sample created (server dry run)
  nemoguardrail.apps.nvidia.com/nemoguardrails-sample configured (server dry run)
  nemoevaluator.apps.nvidia.com/nemoevaluator-sample created (server dry run)
  ```
