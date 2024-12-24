/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"

	v1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	scheme "github.com/NVIDIA/k8s-nim-operator/api/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// NemoDatastoreV2sGetter has a method to return a NemoDatastoreV2Interface.
// A group's client should implement this interface.
type NemoDatastoreV2sGetter interface {
	NemoDatastoreV2s(namespace string) NemoDatastoreV2Interface
}

// NemoDatastoreV2Interface has methods to work with NemoDatastoreV2 resources.
type NemoDatastoreV2Interface interface {
	Create(ctx context.Context, nemoDatastoreV2 *v1alpha1.NemoDatastoreV2, opts v1.CreateOptions) (*v1alpha1.NemoDatastoreV2, error)
	Update(ctx context.Context, nemoDatastoreV2 *v1alpha1.NemoDatastoreV2, opts v1.UpdateOptions) (*v1alpha1.NemoDatastoreV2, error)
	// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
	UpdateStatus(ctx context.Context, nemoDatastoreV2 *v1alpha1.NemoDatastoreV2, opts v1.UpdateOptions) (*v1alpha1.NemoDatastoreV2, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.NemoDatastoreV2, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1alpha1.NemoDatastoreV2List, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.NemoDatastoreV2, err error)
	NemoDatastoreV2Expansion
}

// nemoDatastoreV2s implements NemoDatastoreV2Interface
type nemoDatastoreV2s struct {
	*gentype.ClientWithList[*v1alpha1.NemoDatastoreV2, *v1alpha1.NemoDatastoreV2List]
}

// newNemoDatastoreV2s returns a NemoDatastoreV2s
func newNemoDatastoreV2s(c *AppsV1alpha1Client, namespace string) *nemoDatastoreV2s {
	return &nemoDatastoreV2s{
		gentype.NewClientWithList[*v1alpha1.NemoDatastoreV2, *v1alpha1.NemoDatastoreV2List](
			"nemodatastorev2s",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *v1alpha1.NemoDatastoreV2 { return &v1alpha1.NemoDatastoreV2{} },
			func() *v1alpha1.NemoDatastoreV2List { return &v1alpha1.NemoDatastoreV2List{} }),
	}
}
