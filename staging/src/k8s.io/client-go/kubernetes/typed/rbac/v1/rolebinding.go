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

package v1

import (
	"context"
	json "encoding/json"
	"fmt"
	"time"

	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rbacv1 "k8s.io/client-go/applyconfigurations/rbac/v1"
	scheme "k8s.io/client-go/kubernetes/scheme"
	rest "k8s.io/client-go/rest"
)

// RoleBindingsGetter has a method to return a RoleBindingInterface.
// A group's client should implement this interface.
type RoleBindingsGetter interface {
	RoleBindings(namespace string) RoleBindingInterface
}

// RoleBindingInterface has methods to work with RoleBinding resources.
type RoleBindingInterface interface {
	Create(ctx context.Context, roleBinding *v1.RoleBinding, opts metav1.CreateOptions) (*v1.RoleBinding, error)
	Update(ctx context.Context, roleBinding *v1.RoleBinding, opts metav1.UpdateOptions) (*v1.RoleBinding, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.RoleBinding, error)
	List(ctx context.Context, opts metav1.ListOptions) (*v1.RoleBindingList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.RoleBinding, err error)
	Apply(ctx context.Context, roleBinding *rbacv1.RoleBindingApplyConfiguration, opts metav1.ApplyOptions) (result *v1.RoleBinding, err error)
	RoleBindingExpansion
}

// roleBindings implements RoleBindingInterface
type roleBindings struct {
	client rest.Interface
	ns     string
}

// newRoleBindings returns a RoleBindings
func newRoleBindings(c *RbacV1Client, namespace string) *roleBindings {
	return &roleBindings{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the roleBinding, and returns the corresponding roleBinding object, and an error if there is any.
func (c *roleBindings) Get(ctx context.Context, name string, options metav1.GetOptions) (result *v1.RoleBinding, err error) {
	result = &v1.RoleBinding{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("rolebindings").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of RoleBindings that match those selectors.
func (c *roleBindings) List(ctx context.Context, opts metav1.ListOptions) (result *v1.RoleBindingList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1.RoleBindingList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("rolebindings").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested roleBindings.
func (c *roleBindings) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	client := c.client.Get().Namespace(c.ns).Resource("rolebindings").VersionedParams(&opts, scheme.ParameterCodec)
	if opts.TimeoutSeconds != nil {
		client.Timeout(time.Duration(*opts.TimeoutSeconds) * time.Second)
	}
	return client.Watch(ctx)
}

// Create takes the representation of a roleBinding and creates it.  Returns the server's representation of the roleBinding, and an error, if there is any.
func (c *roleBindings) Create(ctx context.Context, roleBinding *v1.RoleBinding, opts metav1.CreateOptions) (result *v1.RoleBinding, err error) {
	result = &v1.RoleBinding{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("rolebindings").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(roleBinding).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a roleBinding and updates it. Returns the server's representation of the roleBinding, and an error, if there is any.
func (c *roleBindings) Update(ctx context.Context, roleBinding *v1.RoleBinding, opts metav1.UpdateOptions) (result *v1.RoleBinding, err error) {
	result = &v1.RoleBinding{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("rolebindings").
		Name(roleBinding.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(roleBinding).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the roleBinding and deletes it. Returns an error if one occurs.
func (c *roleBindings) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("rolebindings").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *roleBindings) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("rolebindings").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched roleBinding.
func (c *roleBindings) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.RoleBinding, err error) {
	result = &v1.RoleBinding{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("rolebindings").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}

// Apply takes the given apply declarative configuration, applies it and returns the applied roleBinding.
func (c *roleBindings) Apply(ctx context.Context, roleBinding *rbacv1.RoleBindingApplyConfiguration, opts metav1.ApplyOptions) (result *v1.RoleBinding, err error) {
	if roleBinding == nil {
		return nil, fmt.Errorf("roleBinding provided to Apply must not be nil")
	}
	patchOpts := opts.ToPatchOptions()
	data, err := json.Marshal(roleBinding)
	if err != nil {
		return nil, err
	}
	name := roleBinding.Name
	if name == nil {
		return nil, fmt.Errorf("roleBinding.Name must be provided to Apply")
	}
	result = &v1.RoleBinding{}
	err = c.client.Patch(types.ApplyPatchType).
		Namespace(c.ns).
		Resource("rolebindings").
		Name(*name).
		VersionedParams(&patchOpts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
