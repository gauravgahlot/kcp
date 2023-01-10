/*
Copyright The KCP Authors.

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

package fake

import (
	"context"
	json "encoding/json"
	"fmt"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"

	v1alpha1 "github.com/kcp-dev/kcp/sdk/apis/apis/v1alpha1"
	apisv1alpha1 "github.com/kcp-dev/kcp/sdk/client/applyconfiguration/apis/v1alpha1"
)

// FakeAPIResourceSchemas implements APIResourceSchemaInterface
type FakeAPIResourceSchemas struct {
	Fake *FakeApisV1alpha1
}

var apiresourceschemasResource = schema.GroupVersionResource{Group: "apis.kcp.io", Version: "v1alpha1", Resource: "apiresourceschemas"}

var apiresourceschemasKind = schema.GroupVersionKind{Group: "apis.kcp.io", Version: "v1alpha1", Kind: "APIResourceSchema"}

// Get takes name of the aPIResourceSchema, and returns the corresponding aPIResourceSchema object, and an error if there is any.
func (c *FakeAPIResourceSchemas) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.APIResourceSchema, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(apiresourceschemasResource, name), &v1alpha1.APIResourceSchema{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.APIResourceSchema), err
}

// List takes label and field selectors, and returns the list of APIResourceSchemas that match those selectors.
func (c *FakeAPIResourceSchemas) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.APIResourceSchemaList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(apiresourceschemasResource, apiresourceschemasKind, opts), &v1alpha1.APIResourceSchemaList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.APIResourceSchemaList{ListMeta: obj.(*v1alpha1.APIResourceSchemaList).ListMeta}
	for _, item := range obj.(*v1alpha1.APIResourceSchemaList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested aPIResourceSchemas.
func (c *FakeAPIResourceSchemas) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(apiresourceschemasResource, opts))
}

// Create takes the representation of a aPIResourceSchema and creates it.  Returns the server's representation of the aPIResourceSchema, and an error, if there is any.
func (c *FakeAPIResourceSchemas) Create(ctx context.Context, aPIResourceSchema *v1alpha1.APIResourceSchema, opts v1.CreateOptions) (result *v1alpha1.APIResourceSchema, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(apiresourceschemasResource, aPIResourceSchema), &v1alpha1.APIResourceSchema{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.APIResourceSchema), err
}

// Update takes the representation of a aPIResourceSchema and updates it. Returns the server's representation of the aPIResourceSchema, and an error, if there is any.
func (c *FakeAPIResourceSchemas) Update(ctx context.Context, aPIResourceSchema *v1alpha1.APIResourceSchema, opts v1.UpdateOptions) (result *v1alpha1.APIResourceSchema, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(apiresourceschemasResource, aPIResourceSchema), &v1alpha1.APIResourceSchema{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.APIResourceSchema), err
}

// Delete takes name of the aPIResourceSchema and deletes it. Returns an error if one occurs.
func (c *FakeAPIResourceSchemas) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteActionWithOptions(apiresourceschemasResource, name, opts), &v1alpha1.APIResourceSchema{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeAPIResourceSchemas) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(apiresourceschemasResource, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.APIResourceSchemaList{})
	return err
}

// Patch applies the patch and returns the patched aPIResourceSchema.
func (c *FakeAPIResourceSchemas) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.APIResourceSchema, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(apiresourceschemasResource, name, pt, data, subresources...), &v1alpha1.APIResourceSchema{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.APIResourceSchema), err
}

// Apply takes the given apply declarative configuration, applies it and returns the applied aPIResourceSchema.
func (c *FakeAPIResourceSchemas) Apply(ctx context.Context, aPIResourceSchema *apisv1alpha1.APIResourceSchemaApplyConfiguration, opts v1.ApplyOptions) (result *v1alpha1.APIResourceSchema, err error) {
	if aPIResourceSchema == nil {
		return nil, fmt.Errorf("aPIResourceSchema provided to Apply must not be nil")
	}
	data, err := json.Marshal(aPIResourceSchema)
	if err != nil {
		return nil, err
	}
	name := aPIResourceSchema.Name
	if name == nil {
		return nil, fmt.Errorf("aPIResourceSchema.Name must be provided to Apply")
	}
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(apiresourceschemasResource, *name, types.ApplyPatchType, data), &v1alpha1.APIResourceSchema{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.APIResourceSchema), err
}
