# Create a NIM Service

### Pre-requisites

* Create a namespace e.g. `nim-service`
* Create a `NIMCache` instance in the namespace `nim-service` following the guide [here](https://gitlab-master.nvidia.com/dl/container-dev/k8s-nim-operator/-/blob/51e9727929b16982a2dba6d7fccbd0474f566bf8/docs/nimcache.md).

### 1. Create the CR for NIMService

nimservice.yaml:

```yaml
apiVersion: apps.nvidia.com/v1alpha1
kind: NIMService
metadata:
  labels:
    app.kubernetes.io/name: k8s-nim-operator
    app.kubernetes.io/managed-by: kustomize
  name: meta-llama3-8b-instruct
spec:
  image:
    repository: nvcr.io/nim/meta/llama3-8b-instruct
    tag: 1.0.0
    pullPolicy: IfNotPresent
    pullSecrets:
      - ngc-secret
  authSecret: ngc-api-secret
  nimCache:
    name: meta-llama3-8b-instruct
    profile: ''
  scale:
    minReplicas: 1
  resources:
    limits:
      nvidia.com/gpu: 1
  expose:
    service:
      type: ClusterIP
      openaiPort: 8000
```

```sh
kubectl create -f nimservice.yaml -n nim-service
```

### 2. Check the status of NIMService deployment

```sh
kubectl get nimservice -n nim-service
```

```console
kubectl get nimservice -n nim-service
NAME                             STATUS   AGE
meta-llama3-8b-instruct-latest   ready    115m
```

```sh
kubectl get pods -n nim-service
```

```console
NAME                                              READY   STATUS      RESTARTS   AGE
meta-llama3-8b-instruct-latest-db9d899fd-mfmq2    1/1     Running     0          108m
meta-llama3-8b-instruct-latest-job-xktnk          0/1     Completed   0          4m38s
```

### 3. Verify with a sample pod

test-pod.yaml:

```yaml
---
apiVersion: v1
kind: Pod
metadata:
  name: test-streaming-chat
spec:
  containers:
    - name: curl
      image: curlimages/curl:8.6.0
      command: ['curl']
      args:
        - -X
        - "POST"
        - 'http://meta-llama3-8b-instruct:8000/v1/chat/completions'
        - -H
        - 'accept: application/json'
        - -H
        - 'Content-Type: application/json'
        - --fail-with-body
        - -d
        - |
            {
                "model": "meta/llama3-8b-instruct",
                "messages": [
                    {
                        "role":"user",
                        "content":"Hello there how are you?",
                        "name": "aleks"
                    },
                    {
                        "role":"assistant",
                        "content":"How may I help you?"
                    },
                    {
                        "role":"user",
                        "content":"Do something for me?"
                    }
                ],
                "top_p": 1,
                "n": 1,
                "max_tokens": 15,
                "stream": true,
                "frequency_penalty": 1.0,
                "stop": ["hello"]
            }
  restartPolicy: Never
```

```sh
kubectl create -f test-pod.yaml -n nim-service
```

```sh
kubectl get pods -n nim-service
```

```console
NAME                                              READY   STATUS      RESTARTS   AGE
meta-llama3-8b-instruct-latest-db9d899fd-mfmq2    1/1     Running     0          112m
meta-llama3-8b-instruct-latest-job-xktnk          0/1     Completed   0          8m8s
test-streaming-chat                               0/1     Completed   0          99m
```

