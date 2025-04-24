# nemo-data-flywheel-tutorials

Tutorials for NeMo Microservices (MS) Data Flywheel, which includes examples for using the NeMo MS Data Store, Entity Store, Customizer, Evaluator, Guardrails, and NVIDIA NIMs.

## Prerequisites
1. Deploy the NIM operator and NeMo Training Operator. Create CRs for requried NeMo Microservices and the NIM pipeline using the manifests from `manifests` folder.


## 1. Update Config
Update the config.py file with the following values:
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

## 2. Bring up the Jupyter notebook
Create a virtual environment. This is recommended to isolate project dependencies.

```bash
python3 -m venv nemo_env
source nemo_env/bin/activate
```

Install the required Python packages using requirements.txt.

```bash
pip install -r requirements.txt
```

Start the Jupyter lab server on your NIM cluster.
```bash
jupyter lab --ip 0.0.0.0 --port=8888 --allow-root
```

## 3. Access the remote Jupyter notebook

**On your client machine**

SSH tunnel to forward traffic from Jupyter server on your NIM cluster to your local machine
```bash
ssh -N -f -L localhost:8888:localhost:8888 <your-nim-cluster-username>@<your-nim-cluster-ip>
```

Access the Jupyter lab on localhost:8888 in your browser. Paste in the token from the Jupyter server output for authentication.

