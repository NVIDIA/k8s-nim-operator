# nemo-data-flywheel-tutorials

Tutorials for NeMo Microservices (MS) Data Flywheel, which includes examples for using the NeMo MS Data Store, Entity Store, Customizer, Evaluator, Guardrails, and NVIDIA NIMs.

## Prerequisites
1. This notebook requires the machine to have 2 GPUs, one for base model inferencing and one for model fine-tuning.
2. Follow the instructions in [NeMo Microservices Prerequisites](https://sw-docs-dgx-station.nvidia.com/nim-operator/review/89/nemo-prerequisites.html) to install NeMo microservice dependencies and NeMo Training Operator.
3. Deploy the Ingress 
    a. Install ingress controller (e.g. nginx ingress controller)
    ```bash
    helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
    helm repo update
    helm install nginx-ingress ingress-nginx/ingress-nginx -n ingress-nginx
    ```
    b. Deploy the Ingress objects as configured in [manifests/ingress.yml](./manifests/ingress.yml).
4. Add the ClusterIP of `ingress-nginx-controller` Service to `/etc/hosts`.
    ```bash
    export INGRESS_IP=$(kubectl get svc -n ingress-nginx -o jsonpath='{.items[0].status.clusterIP}')
    echo -e "\n$INGRESS_IP nemo.test\n $INGRESS_IP data-store.test\n $INGRESS_IP nim.test\n" | sudo tee -a /etc/hosts
    ```
5. Deploy the NeMo microservices as defined under [./manifests](./manifests).


## 2. Update Config
Update the config.py file with the following values:
```python
NDS_URL = "http://data-store.test"
NEMO_URL = "http://nemo.test"
NIM_URL = "http://nim.test"
HF_TOKEN = "<your-huggingface-token>"
BASE_MODEL = "meta/llama-3.2-1b-instruct"
```

## 3. Bring up the Jupyter notebook
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

## 4. Access the remote Jupyter notebook

**On your client machine**

SSH tunnel to forward traffic from Jupyter server on your NIM cluster to your local machine
```bash
ssh -N -f -L localhost:8888:localhost:8888 <your-nim-cluster-username>@<your-nim-cluster-ip>
```

Access the Jupyter lab on localhost:8888 in your browser. Paste in the token from the Jupyter server output for authentication.

