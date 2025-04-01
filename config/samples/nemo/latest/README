# NeMo Custom Resources for 25.04

These CRs are designed to deploy NeMo microservices using the NIM Operator.

## Compatible NIM Operator Version

- **NIM Operator v2.0.0**

>  Using these CRs with any other version may lead to validation or runtime errors.

## Notes

- The CR schema and fields in this version match the capabilities of NIM Operator v2.0.0.

## Upgrade Notes

If upgrading from a previous NeMo service version (e.g., `25.04`) using the existing operator version:
- Check for renamed or deprecated fields.
- Review updated model config parameters.
- Revalidate against the new CR using:  
  ```bash
  kubectl apply --server-dry-run=client -f appsv1_v1alpha1_nemo*.yaml```
