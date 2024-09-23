/*
Copyright 2021.

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

package v1beta1

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:images

// ImageConfig is the Schema for the imageconfigs API
type ImageConfig struct {
	metav1.TypeMeta `json:",inline"`

	Images *Images `json:"images,omitempty"`

	DefaultImages *DefaultImages `json:"defaults,omitempty"`

	ImagePolicy
}

type ImagePolicy struct {
	ImageRegistry string `json:"imageRegistry,omitempty"`

	ImagePullSecret corev1.LocalObjectReference `json:"imagePullSecret,omitempty"`

	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	ImageNamespace *string `json:"imageNamespace,omitempty"`
}

//+kubebuilder:object:root=true

// Images defines (or overrides) the imageComponent:version combination for deployed container images
type Images struct {
	metav1.TypeMeta `json:",inline"`

	CassandraVersions map[string]string `json:"cassandra,omitempty"`

	DSEVersions map[string]string `json:"dse,omitempty"`

	SystemLogger string `json:"system-logger,omitempty"`

	Client string `json:"k8ssandra-client,omitempty"`

	ConfigBuilder string `json:"config-builder,omitempty"`

	Others map[string]string `json:",inline,omitempty"`
}

type _Images Images

func (i *Images) UnmarshalJSON(b []byte) error {
	var imagesTemp _Images
	if err := json.Unmarshal(b, &imagesTemp); err != nil {
		return err
	}
	*i = Images(imagesTemp)

	var otherFields map[string]interface{}
	if err := json.Unmarshal(b, &otherFields); err != nil {
		return err
	}

	delete(otherFields, CassandraImageComponent)
	delete(otherFields, DSEImageComponent)
	delete(otherFields, SystemLoggerImageComponent)
	delete(otherFields, ConfigBuilderImageComponent)
	delete(otherFields, ClientImageComponent)

	others := make(map[string]string, len(otherFields))
	for k, v := range otherFields {
		others[k] = v.(string)
	}

	i.Others = others
	return nil
}

const (
	CassandraImageComponent     string = "cassandra"
	DSEImageComponent           string = "dse"
	HCDImageComponent           string = "hcd"
	SystemLoggerImageComponent  string = "system-logger"
	ConfigBuilderImageComponent string = "config-builder"
	ClientImageComponent        string = "k8ssandra-client"
)

type ImageComponents map[string]ImageComponent

type DefaultImages struct {
	ImageComponents
}

func (d *DefaultImages) MarshalJSON() ([]byte, error) {
	// This shouldn't be required, just like it's not with ImagePolicy, but this is Go..
	return json.Marshal(d.ImageComponents)
}

func (d *DefaultImages) UnmarshalJSON(b []byte) error {
	d.ImageComponents = make(map[string]ImageComponent)
	var input map[string]json.RawMessage
	if err := json.Unmarshal(b, &input); err != nil {
		return err
	}

	for k, v := range input {
		var component ImageComponent
		if err := json.Unmarshal(v, &component); err != nil {
			return err
		}
		d.ImageComponents[k] = component
	}

	return nil
}

type ImageComponent struct {
	Repository string `json:"repository,omitempty"`
	Suffix     string `json:"suffix,omitempty"`
	ImagePolicy
}

func init() {
	SchemeBuilder.Register(&ImageConfig{}, &Images{})
}
