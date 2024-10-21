---
name: Bug report
about: Create a report to help us improve
title: Bug report template
labels: ''
assignees: slu2011, visheshtanksale, shivamerla

---

### 1. Quick Debug Information
* OS/Version(e.g. RHEL8.6, Ubuntu22.04):
* Kernel Version:
* Container Runtime Type/Version(e.g. Containerd, CRI-O, Docker):
* K8s Flavor/Version(e.g. K8s, OCP, Rancher, GKE, EKS):
* GPU Operator Version:
* NIM Operator Version:
* LLM NIM Versions:
* NeMo Service Versions:

### 2. Issue or feature description
_Briefly explain the issue in terms of expected behavior and current behavior._

### 3. Steps to reproduce the issue
_Detailed steps to reproduce the issue._

### 4. Information to attach

 - [ ] Operator pod status: 
    * `kubectl get pods -n OPERATOR_NAMESPACE`
    * `kubectl logs <operator-pod> -n OPERATOR_NAMESPACE`
 - [ ] NIM Cache status: 
    * `kubectl get nimcache -A`
    * `kubectl describe nimcache -n <namespace>`
    * `kubectl get events -n <namespace>`
    * `kubectl get logs <caching-job> -n <namespace>`
    * `kubectl get pv, pvc -n <namespace>`
 - [ ] NIM Service status: 
    * `kubectl get nimservice -A`
    * `kubectl describe nimservice -n <namespace>`
    * `kubectl get events -n <namespace>`
    * `kubectl get logs <nim-service-pod> -n <namespace>`

 - [ ] If a pod/deployment is in an error state or pending state `kubectl describe pod -n <namespace> POD_NAME`
 - [ ] If a pod/deployment is in an error state or pending state `kubectl logs -n <namespace> POD_NAME --all-containers`
 - [ ] Output from running `nvidia-smi` from the driver container deployed by the GPU Operator: `kubectl exec DRIVER_POD_NAME -n <GPU_OPERATOR_NAMESPACE> -c nvidia-driver-ctr -- nvidia-smi`
