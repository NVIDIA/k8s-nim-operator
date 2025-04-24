# (Required) NeMo Microservices URLs
NDS_URL = "http://nemodatastore-sample.nemo.svc.cluster.local:8000" # Data Store
ENTITY_STORE_URL = "http://nemoentitystore-sample.nemo.svc.cluster.local:8000" # Entity Store
CUSTOMIZER_URL = "http://nemocustomizer-sample.nemo.svc.cluster.local:8000" # Customizer
EVALUATOR_URL = "http://nemoevaluator-sample.nemo.svc.cluster.local:8000" # Evaluator
GUARDRAILS_URL = "http://nemoguardrails-sample.nemo.svc.cluster.local:8000" # Guardrails
NIM_URL = "http://meta-llama3-1b-instruct.nemo.svc.cluster.local:8000" # NIM

# (Required) Hugging Face Token
HF_TOKEN = ""

# (Optional) To observe training with WandB
WANDB_API_KEY = ""

# (Optional) Modify if you've configured a NeMo Data Store token
NDS_TOKEN = "token"

# (Optional) Use a dedicated namespace and dataset name for tutorial assets
NMS_NAMESPACE = "xlam-tutorial-ns"
DATASET_NAME = "xlam-ft-dataset"

# (Optional) Configure the base model. Must be one supported by the NeMo Customizer deployment!
BASE_MODEL = "meta/llama-3.2-1b-instruct"
