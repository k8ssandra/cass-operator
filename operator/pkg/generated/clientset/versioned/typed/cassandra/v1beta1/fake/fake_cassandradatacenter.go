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

package fake

import (
	v1beta1 "github.com/k8ssandra/cass-operator/operator/pkg/apis/cassandra/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeCassandraDatacenters implements CassandraDatacenterInterface
type FakeCassandraDatacenters struct {
	Fake *FakeCassandraV1beta1
	ns   string
}

var cassandradatacentersResource = schema.GroupVersionResource{Group: "cassandra.datastax.com", Version: "v1beta1", Resource: "cassandradatacenters"}

var cassandradatacentersKind = schema.GroupVersionKind{Group: "cassandra.datastax.com", Version: "v1beta1", Kind: "CassandraDatacenter"}

// Get takes name of the cassandraDatacenter, and returns the corresponding cassandraDatacenter object, and an error if there is any.
func (c *FakeCassandraDatacenters) Get(name string, options v1.GetOptions) (result *v1beta1.CassandraDatacenter, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(cassandradatacentersResource, c.ns, name), &v1beta1.CassandraDatacenter{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.CassandraDatacenter), err
}

// List takes label and field selectors, and returns the list of CassandraDatacenters that match those selectors.
func (c *FakeCassandraDatacenters) List(opts v1.ListOptions) (result *v1beta1.CassandraDatacenterList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(cassandradatacentersResource, cassandradatacentersKind, c.ns, opts), &v1beta1.CassandraDatacenterList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1beta1.CassandraDatacenterList{ListMeta: obj.(*v1beta1.CassandraDatacenterList).ListMeta}
	for _, item := range obj.(*v1beta1.CassandraDatacenterList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested cassandraDatacenters.
func (c *FakeCassandraDatacenters) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(cassandradatacentersResource, c.ns, opts))

}

// Create takes the representation of a cassandraDatacenter and creates it.  Returns the server's representation of the cassandraDatacenter, and an error, if there is any.
func (c *FakeCassandraDatacenters) Create(cassandraDatacenter *v1beta1.CassandraDatacenter) (result *v1beta1.CassandraDatacenter, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(cassandradatacentersResource, c.ns, cassandraDatacenter), &v1beta1.CassandraDatacenter{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.CassandraDatacenter), err
}

// Update takes the representation of a cassandraDatacenter and updates it. Returns the server's representation of the cassandraDatacenter, and an error, if there is any.
func (c *FakeCassandraDatacenters) Update(cassandraDatacenter *v1beta1.CassandraDatacenter) (result *v1beta1.CassandraDatacenter, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(cassandradatacentersResource, c.ns, cassandraDatacenter), &v1beta1.CassandraDatacenter{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.CassandraDatacenter), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeCassandraDatacenters) UpdateStatus(cassandraDatacenter *v1beta1.CassandraDatacenter) (*v1beta1.CassandraDatacenter, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(cassandradatacentersResource, "status", c.ns, cassandraDatacenter), &v1beta1.CassandraDatacenter{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.CassandraDatacenter), err
}

// Delete takes name of the cassandraDatacenter and deletes it. Returns an error if one occurs.
func (c *FakeCassandraDatacenters) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(cassandradatacentersResource, c.ns, name), &v1beta1.CassandraDatacenter{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeCassandraDatacenters) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(cassandradatacentersResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1beta1.CassandraDatacenterList{})
	return err
}

// Patch applies the patch and returns the patched cassandraDatacenter.
func (c *FakeCassandraDatacenters) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.CassandraDatacenter, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(cassandradatacentersResource, c.ns, name, pt, data, subresources...), &v1beta1.CassandraDatacenter{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.CassandraDatacenter), err
}
