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
	"time"

	v1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	scheme "github.com/NVIDIA/k8s-nim-operator/api/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// NIMPipelinesGetter has a method to return a NIMPipelineInterface.
// A group's client should implement this interface.
type NIMPipelinesGetter interface {
	NIMPipelines(namespace string) NIMPipelineInterface
}

// NIMPipelineInterface has methods to work with NIMPipeline resources.
type NIMPipelineInterface interface {
	Create(ctx context.Context, nIMPipeline *v1alpha1.NIMPipeline, opts v1.CreateOptions) (*v1alpha1.NIMPipeline, error)
	Update(ctx context.Context, nIMPipeline *v1alpha1.NIMPipeline, opts v1.UpdateOptions) (*v1alpha1.NIMPipeline, error)
	UpdateStatus(ctx context.Context, nIMPipeline *v1alpha1.NIMPipeline, opts v1.UpdateOptions) (*v1alpha1.NIMPipeline, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.NIMPipeline, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1alpha1.NIMPipelineList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.NIMPipeline, err error)
	NIMPipelineExpansion
}

// nIMPipelines implements NIMPipelineInterface
type nIMPipelines struct {
	client rest.Interface
	ns     string
}

// newNIMPipelines returns a NIMPipelines
func newNIMPipelines(c *AppsV1alpha1Client, namespace string) *nIMPipelines {
	return &nIMPipelines{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the nIMPipeline, and returns the corresponding nIMPipeline object, and an error if there is any.
func (c *nIMPipelines) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.NIMPipeline, err error) {
	result = &v1alpha1.NIMPipeline{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("nimpipelines").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of NIMPipelines that match those selectors.
func (c *nIMPipelines) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.NIMPipelineList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha1.NIMPipelineList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("nimpipelines").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested nIMPipelines.
func (c *nIMPipelines) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("nimpipelines").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a nIMPipeline and creates it.  Returns the server's representation of the nIMPipeline, and an error, if there is any.
func (c *nIMPipelines) Create(ctx context.Context, nIMPipeline *v1alpha1.NIMPipeline, opts v1.CreateOptions) (result *v1alpha1.NIMPipeline, err error) {
	result = &v1alpha1.NIMPipeline{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("nimpipelines").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(nIMPipeline).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a nIMPipeline and updates it. Returns the server's representation of the nIMPipeline, and an error, if there is any.
func (c *nIMPipelines) Update(ctx context.Context, nIMPipeline *v1alpha1.NIMPipeline, opts v1.UpdateOptions) (result *v1alpha1.NIMPipeline, err error) {
	result = &v1alpha1.NIMPipeline{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("nimpipelines").
		Name(nIMPipeline.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(nIMPipeline).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *nIMPipelines) UpdateStatus(ctx context.Context, nIMPipeline *v1alpha1.NIMPipeline, opts v1.UpdateOptions) (result *v1alpha1.NIMPipeline, err error) {
	result = &v1alpha1.NIMPipeline{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("nimpipelines").
		Name(nIMPipeline.Name).
		SubResource("status").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(nIMPipeline).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the nIMPipeline and deletes it. Returns an error if one occurs.
func (c *nIMPipelines) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("nimpipelines").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *nIMPipelines) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("nimpipelines").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched nIMPipeline.
func (c *nIMPipelines) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.NIMPipeline, err error) {
	result = &v1alpha1.NIMPipeline{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("nimpipelines").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}