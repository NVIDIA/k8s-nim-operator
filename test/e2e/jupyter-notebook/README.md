# nemo-data-flywheel-tutorials

Tutorials for NeMo Microservices (MS) Data Flywheel, including examples for using the NeMo MS Data Store, Entity Store, Customizer, Evaluator, Guardrails, and NVIDIA NIMs.

## Prerequiste 
Install NIM Operator and perform [quick start deploy of all NeMo Microservices](https://docs.nvidia.com/nim-operator/latest/deploy-nemo-microservices.html)   
## Steps to run the notebook

1. Update Notebook Config
Update the `test/e2e/jupyter-notebook/config.py` file with the following values:

```python
NDS_URL = "http://nemodatastore-sample.nemo.svc.cluster.local:8000" # Data Store
ENTITY_STORE_URL = "http://nemoentitystore-sample.nemo.svc.cluster.local:8000" # Entity Store
CUSTOMIZER_URL = "http://nemocustomizer-sample.nemo.svc.cluster.local:8000" # Customizer
EVALUATOR_URL = "http://nemoevaluator-sample.nemo.svc.cluster.local:8000" # Evaluator
GUARDRAILS_URL = "http://nemoguardrails-sample.nemo.svc.cluster.local:8000" # Guardrails
NIM_URL = "http://meta-llama3-1b-instruct.nemo.svc.cluster.local:8000" # NIM
HF_TOKEN = "<your-huggingface-token>"
BASE_MODEL = "meta/llama-3.2-1b-instruct"
```


2. Access your jupyter server

Once the Ansible playbook has completed, the Jupyter server will be running in your cluster.

To access the Jupyter notebook, use `kubectl port-forward` from your local machine and launch using http://localhost:8888

```bash
kubectl port-forward svc/jupyter-service -n nemo 8888:8888
```
