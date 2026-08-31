# NIM Samples: Embedding, Reranking, Guardrail and LLM

This directory contains sample `NIMCache`, `NIMService`, and `NIMPipeline` manifests.

## Image versions

| Sample family | Image | Tag | Notes |
|---|---|---|---|
| LLM-specific (1-GPU demos) | `nvcr.io/nim/meta/llama-3.2-1b-instruct` | `1.12.0` | Latest published tag for this 1.x container |
| Multi-LLM (1.x) | `nvcr.io/nim/nvidia/llm-nim` | `1.15.0` | Catalog latest tag is `1.15` |
| RAG pipeline LLM | `nvcr.io/nim/meta/llama-3.1-8b-instruct` | `1.15.5` | Latest 1.x tag; 2.0 series is separate |
| RAG embedding | `nvcr.io/nim/nvidia/llama-3.2-nv-embedqa-1b-v2` | `1.10` | NGC latest tag |
| RAG reranking | `nvcr.io/nim/nvidia/llama-3.2-nv-rerankqa-1b-v2` | `1.8.0` | NGC latest tag |
| NemoGuard content safety | `nvcr.io/nim/nvidia/llama-3.1-nemoguard-8b-content-safety` | `1.10.2` | |
| NemoGuard topic control | `nvcr.io/nim/nvidia/llama-3.1-nemoguard-8b-topic-control` | `1.10.1` | |
| NemoGuard jailbreak | `nvcr.io/nim/nvidia/nemoguard-jailbreak-detect` | `1.10.1` | |
| Riva TTS | `nvcr.io/nim/nvidia/riva-tts` | `1.3.0` | Latest published tag |
| DeepSeek-R1 (MPI multi-node) | `nvcr.io/nim/deepseek-ai/deepseek-r1` | `1.7.3` | Latest tag; NGC marks the artifact end-of-support |
| Model-Free NIM 3.0 | `nvcr.io/nim/nvidia/model-free-nim` | `3.0.0` | Serves HF (or other URI) weights; 2.0.10 is latest 2.0 |
| NIM 3.0 model-specific | `nvcr.io/nim/nvidia/nemotron-3-super-120b-a12b` | `3.0.0` | Requires multiple GPUs; 2.0.10 is latest 2.0 |
| NIM 2.0+ multi-node (Ray) | `nvcr.io/nim/meta/llama-3.1-8b-instruct` | `2.0.10` | Uses `spec.multiNode.ray` |
| Retriever 2.x | `nvcr.io/nim/nvidia/llama-nemotron-embed-vl-1b-v2` | `2.3.0` | Native download; no model profiles |

Pin tags explicitly. The NIM LLM 2.0 and 3.0 series ship in parallel; 3.0 is not a drop-in replacement for every 1.x sample.

## GPU Requirements

Most samples request **one GPU**. Exceptions:

- `serving/standalone/nim-3/nemotron-3-super.yaml` — large model-specific NIM 3.0 (sample requests 4 GPUs; confirm against the support matrix)
- `serving/advanced/multi-node/` — multi-node (MPI sample uses 8 GPUs; Ray sample uses 2)

Embedding, reranking, and guardrail NIMs can share GPUs via MIG or time-slicing. See the GPU Operator [MIG documentation](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/gpu-operator-mig.html).

## Persistent Volume Configuration

By default these samples use a `ReadWriteOnce` (RWO) PVC. That can pin pods to a single node. If the cluster supports `ReadWriteMany` (for example NFS), set:

```yaml
storage:
  pvc:
    create: true
    storageClass: "<your-rwx-storage-class>"
    size: "50Gi"
    volumeAccessMode: ReadWriteMany
```

## Linking NIMCache to NIMService

A Hugging Face `NIMCache` does not serve traffic by itself. After `kubectl get nimcache` shows `Ready`, point the service at it:

```yaml
spec:
  storage:
    nimCache:
      name: <nimcache-metadata-name>
```

End-to-end examples:

- Multi-LLM 1.x: `serving/standalone/basic/multi-llm.yaml`
- Model-Free NIM 3.0: `serving/standalone/basic/model-free.yaml`
- Model-Free on KServe Standard: `serving/kserve/standard/model-free.yaml`

An NGC **LLM-specific** cache (profiles/engines) is not interchangeable with an HF **model-free** cache. Both store model artifacts on a PVC, but the on-disk layout differs. HF-shaped caches can back Multi-LLM or Model-Free serving images; NGC LLM-specific caches cannot.

## Recent functionality samples

| Path | What it shows |
|---|---|
| `caching/hf/nimcache-model-free.yaml` | HF cache using the Model-Free puller image |
| `serving/standalone/basic/model-free.yaml` | HF `NIMCache` + Model-Free `NIMService` |
| `serving/kserve/standard/model-free.yaml` | Same on KServe Standard mode |
| `serving/standalone/no-precaching/model-free-hf.yaml` | Model-Free without a pre-built cache (`NIM_MODEL_PATH`) |
| `serving/standalone/nim-3/nemotron-3-super.yaml` | NIM 3.0 model-specific cache + service |
| `serving/advanced/multi-node/multi-node-nimservice-ray.yaml` | NIM 2.0+ multi-node with Ray |
| `serving/standalone/no-precaching/llm-hostpath.yaml` | `spec.storage.hostPath` |
| `serving/standalone/sidecars/llm.yaml` | `spec.initContainers` and `spec.sidecarContainers` |
| `serving/standalone/scheduling/llm.yaml` | `spec.priorityClassName` and `spec.affinity` |
| `serving/standalone/retriever/embed.yaml` | NeMo Retriever 2.x+ native cache + service |
