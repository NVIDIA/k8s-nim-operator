package client

import (
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	nimclientset "github.com/NVIDIA/k8s-nim-operator/api/versioned"
)

type Client interface {
	KubernetesClient() kubernetes.Interface
	NIMClient() nimclientset.Interface
}

type k8sClient struct {
	kubeClient kubernetes.Interface
	nimClient  nimclientset.Interface
}

func NewClient(factory cmdutil.Factory) (Client, error) {
	kubeClient, err := factory.KubernetesClientSet()
	if err != nil {
		return nil, err
	}
	restConfig, err := factory.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	nimClient, err := nimclientset.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	return &k8sClient{
		kubeClient: kubeClient,
		nimClient:  nimClient,
	}, nil
}

func (c *k8sClient) KubernetesClient() kubernetes.Interface {
	return c.kubeClient
}

func (c *k8sClient) NIMClient() nimclientset.Interface {
	return c.nimClient
}
