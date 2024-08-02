<!--
  SPDX-FileCopyrightText: Copyright (c) 2024 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
  SPDX-License-Identifier: Apache-2.0
-->

# Create a NIM Service

## Prerequisites

* A `NIMCache` instance in the namespace `nim-service`.

## 1. Create the NIM Service Instance

Create a file, such as `nimservice.yaml`, with contents like the following example:

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

Apply the manifest:

```sh
kubectl create -f nimservice.yaml -n nim-service
```

### 2. Check the Status of NIM Service Deployment

```sh
kubectl get nimservice -n nim-service
```

```output
NAME                      STATUS   AGE
meta-llama3-8b-instruct   Ready    115m
```

```sh
kubectl get pods -n nim-service
```

```output
NAME                                       READY   STATUS      RESTARTS   AGE
meta-llama3-8b-instruct-db9d899fd-mfmq2    1/1     Running     0          108m
meta-llama3-8b-instruct-job-xktnk          0/1     Completed   0          4m38s
```

### 3. Verify the Microservice is Running

Create a file, `verify-pod.yaml`, with contents like the following example:

```yaml
---
apiVersion: v1
kind: Pod
metadata:
  name: verify-streaming-chat
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

Apply the manifest:

```sh
kubectl create -f test-pod.yaml -n nim-service
```

Confirm the verification pod ran to completion:

```sh
kubectl get pods -n nim-service
```

```console
NAME                                              READY   STATUS      RESTARTS   AGE
meta-llama3-8b-instruct-latest-db9d899fd-mfmq2    1/1     Running     0          112m
meta-llama3-8b-instruct-latest-job-xktnk          0/1     Completed   0          8m8s
verify-streaming-chat                             0/1     Completed   0          99m
```

