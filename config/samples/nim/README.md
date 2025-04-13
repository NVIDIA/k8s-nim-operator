# NIM Samples: Embedding, Reranking, Guardrail and LLM

This repository includes sample deployments for NVIDIA NIMs targeting **Embedding**, **Reranking**, **Guardrail** and **LLM** use cases.

## GPU Requirements

Each sample NIM uses **one GPU resource**. Please ensure that your Kubernetes cluster nodes have sufficient GPU resources available to support these workloads concurrently. It is also possible to run **Embedding**, **Reranking** and **Guardrail** NIMs on shared GPUs. Please refer to GPU sharing techniques like `MIG` and `TimeSlicing` from the GPU Operator [here](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/gpu-operator-mig.html).

## Persistent Volume Configuration

By default, these samples are configured to use a `ReadWriteOnce` (RWO) persistent volume claim (PVC). This means the volume can only be mounted by a single node, which may cause scheduling issues in multi-node clusters where volume binding locks pods to specific nodes.

### Recommended: Use RWX Storage (if available)

If your cluster supports `ReadWriteMany` (RWX) volumes (e.g., through NFS), update the PVC configuration to allow seamless scheduling across nodes.

Modify the `storage` section in your `NIMCache` configuration as shown below:

```yaml
storage:
  pvc:
    create: true
    storageClass: "<your-rwx-storage-class>"
    size: "50Gi"
    volumeAccessMode: ReadWriteMany
```