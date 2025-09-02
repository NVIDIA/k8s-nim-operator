# NIM Sample: LLM NIMCache from Datastore
This sample demonstrates how to use a NIMCache to pull models from the NeMo Datastore.
The examples in this directory assumes that the model named `llama-3-1b-instruct` is uploaded to the NeMo Datastore endpoint in `default` namespace.
The secret `hf-auth` is used to authenticate with the NeMo Datastore. The secret is expected to have a key named `HF_TOKEN` that contains the authentication token.