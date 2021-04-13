/*
Copyright The Kubernetes Authors.

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
	json "encoding/json"
	"fmt"
	"time"

	v1alpha1 "k8s.io/api/apiserverinternal/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	apiserverinternalv1alpha1 "k8s.io/client-go/applyconfigurations/apiserverinternal/v1alpha1"
	scheme "k8s.io/client-go/kubernetes/scheme"
	rest "k8s.io/client-go/rest"
)

// StorageVersionsGetter has a method to return a StorageVersionInterface.
// A group's client should implement this interface.
type StorageVersionsGetter interface {
	StorageVersions() StorageVersionInterface
}

// StorageVersionInterface has methods to work with StorageVersion resources.
type StorageVersionInterface interface {
	Create(ctx context.Context, storageVersion *v1alpha1.StorageVersion, opts v1.CreateOptions) (*v1alpha1.StorageVersion, error)
	Update(ctx context.Context, storageVersion *v1alpha1.StorageVersion, opts v1.UpdateOptions) (*v1alpha1.StorageVersion, error)
	UpdateStatus(ctx context.Context, storageVersion *v1alpha1.StorageVersion, opts v1.UpdateOptions) (*v1alpha1.StorageVersion, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.StorageVersion, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1alpha1.StorageVersionList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.StorageVersion, err error)
	Apply(ctx context.Context, storageVersion *apiserverinternalv1alpha1.StorageVersionApplyConfiguration, opts v1.ApplyOptions) (result *v1alpha1.StorageVersion, err error)
	ApplyStatus(ctx context.Context, storageVersion *apiserverinternalv1alpha1.StorageVersionApplyConfiguration, opts v1.ApplyOptions) (result *v1alpha1.StorageVersion, err error)
	StorageVersionExpansion
}

// storageVersions implements StorageVersionInterface
type storageVersions struct {
	client rest.Interface
}

// newStorageVersions returns a StorageVersions
func newStorageVersions(c *InternalV1alpha1Client) *storageVersions {
	return &storageVersions{
		client: c.RESTClient(),
	}
}

// Get takes name of the storageVersion, and returns the corresponding storageVersion object, and an error if there is any.
func (c *storageVersions) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.StorageVersion, err error) {
	result = &v1alpha1.StorageVersion{}
	err = c.client.Get().
		Resource("storageversions").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of StorageVersions that match those selectors.
func (c *storageVersions) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.StorageVersionList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha1.StorageVersionList{}
	err = c.client.Get().
		Resource("storageversions").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested storageVersions.
func (c *storageVersions) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	client := c.client.Get().Resource("storageversions").VersionedParams(&opts, scheme.ParameterCodec)
	if opts.TimeoutSeconds != nil {
		client.Timeout(time.Duration(*opts.TimeoutSeconds) * time.Second)
	}
	return client.Watch(ctx)
}

// Create takes the representation of a storageVersion and creates it.  Returns the server's representation of the storageVersion, and an error, if there is any.
func (c *storageVersions) Create(ctx context.Context, storageVersion *v1alpha1.StorageVersion, opts v1.CreateOptions) (result *v1alpha1.StorageVersion, err error) {
	result = &v1alpha1.StorageVersion{}
	err = c.client.Post().
		Resource("storageversions").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(storageVersion).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a storageVersion and updates it. Returns the server's representation of the storageVersion, and an error, if there is any.
func (c *storageVersions) Update(ctx context.Context, storageVersion *v1alpha1.StorageVersion, opts v1.UpdateOptions) (result *v1alpha1.StorageVersion, err error) {
	result = &v1alpha1.StorageVersion{}
	err = c.client.Put().
		Resource("storageversions").
		Name(storageVersion.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(storageVersion).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *storageVersions) UpdateStatus(ctx context.Context, storageVersion *v1alpha1.StorageVersion, opts v1.UpdateOptions) (result *v1alpha1.StorageVersion, err error) {
	result = &v1alpha1.StorageVersion{}
	err = c.client.Put().
		Resource("storageversions").
		Name(storageVersion.Name).
		SubResource("status").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(storageVersion).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the storageVersion and deletes it. Returns an error if one occurs.
func (c *storageVersions) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	return c.client.Delete().
		Resource("storageversions").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *storageVersions) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Resource("storageversions").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched storageVersion.
func (c *storageVersions) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.StorageVersion, err error) {
	result = &v1alpha1.StorageVersion{}
	err = c.client.Patch(pt).
		Resource("storageversions").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}

// Apply takes the given apply declarative configuration, applies it and returns the applied storageVersion.
func (c *storageVersions) Apply(ctx context.Context, storageVersion *apiserverinternalv1alpha1.StorageVersionApplyConfiguration, opts v1.ApplyOptions) (result *v1alpha1.StorageVersion, err error) {
	if storageVersion == nil {
		return nil, fmt.Errorf("storageVersion provided to Apply must not be nil")
	}
	patchOpts := opts.ToPatchOptions()
	data, err := json.Marshal(storageVersion)
	if err != nil {
		return nil, err
	}
	name := storageVersion.Name
	if name == nil {
		return nil, fmt.Errorf("storageVersion.Name must be provided to Apply")
	}
	result = &v1alpha1.StorageVersion{}
	err = c.client.Patch(types.ApplyPatchType).
		Resource("storageversions").
		Name(*name).
		VersionedParams(&patchOpts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}

// ApplyStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating ApplyStatus().
func (c *storageVersions) ApplyStatus(ctx context.Context, storageVersion *apiserverinternalv1alpha1.StorageVersionApplyConfiguration, opts v1.ApplyOptions) (result *v1alpha1.StorageVersion, err error) {
	if storageVersion == nil {
		return nil, fmt.Errorf("storageVersion provided to Apply must not be nil")
	}
	patchOpts := opts.ToPatchOptions()
	data, err := json.Marshal(storageVersion)
	if err != nil {
		return nil, err
	}

	name := storageVersion.Name
	if name == nil {
		return nil, fmt.Errorf("storageVersion.Name must be provided to Apply")
	}

	result = &v1alpha1.StorageVersion{}
	err = c.client.Patch(types.ApplyPatchType).
		Resource("storageversions").
		Name(*name).
		SubResource("status").
		VersionedParams(&patchOpts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
