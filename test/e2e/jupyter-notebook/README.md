# nemo-data-flywheel-tutorials

Tutorials for NeMo Microservices (MS) Data Flywheel, including examples for using the NeMo MS Data Store, Entity Store, Customizer, Evaluator, Guardrails, and NVIDIA NIMs.

## 1. Steps to run the notebook

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

If you are already done steps in QuickStart guide, you can skip steps 2-4
2. **Install the NeMo Dependencies Ansible playbook** that deploys the Jupyter server with all required NeMo dependencies enabled in `values.yaml`.

``` yaml
install:
  customizer: yes
  datastore: yes
  entity_store: yes
  evaluator: yes
  jupyter: yes
```

3. **Deploy the NeMo Training Operator**

```bash
kubectl create ns nemo-operator
kubectl create secret -n nemo-operator docker-registry ngc-secret \
  --docker-server=nvcr.io \
  --docker-username='$oauthtoken' \
  --docker-password=<ngc-api-key>
```

```bash
helm fetch https://helm.ngc.nvidia.com/nvidia/nemo-microservices/charts/nemo-operator-25.4.0.tgz --username='$oauthtoken' --password=<YOUR NGC API KEY>
helm install nemo-operator-25.4.0.tgz -n nemo-operator --set imagePullSecrets[0].name=ngc-secret --set controllerManager.manager.scheduler=volcano
```

4. **Create Custom Resources (CRs) for all NeMo and NIM samples** from `config/samples/nemo/latest` folder.

```bash
kubectl apply -f config/samples/nemo/latest
```

5. Access your jupyter server

Once the Ansible playbook has completed, the Jupyter server will be running in your cluster.

To access the Jupyter notebook, use `kubectl port-forward` from your local machine and launch using http://localhost:8888. The token is "token"

```bash
kubectl port-forward svc/jupyter-service -n nemo 8888:8888
```
5. Access your notebook

The notebook is under work directory.
