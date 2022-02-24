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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

//+kubebuilder:object:root=true
//+kubebuilder:subresource:images

// ImageConfig is the Schema for the imageconfigs API
type ImageConfig struct {
	metav1.TypeMeta `json:",inline"`

	Images *Images `json:"images,omitempty"`

	DefaultImages *DefaultImages `json:"defaults,omitempty"`

	ImageRegistry string `json:"imageRegistry,omitempty"`

	ImagePullSecret corev1.LocalObjectReference `json:"imagePullSecret,omitempty"`

	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
}

//+kubebuilder:object:root=true

// Images defines (or overrides) the imageComponent:version combination for deployed container images
type Images struct {
	metav1.TypeMeta `json:",inline"`

	CassandraVersions map[string]string `json:"cassandra,omitempty"`

	DSEVersions map[string]string `json:"dse,omitempty"`

	SystemLogger string `json:"system-logger"`

	ConfigBuilder string `json:"config-builder"`
}

type DefaultImages struct {
	metav1.TypeMeta `json:",inline"`

	CassandraImageComponent ImageComponent `json:"cassandra,omitempty"`

	DSEImageComponent ImageComponent `json:"dse,omitempty"`
}

type ImageComponent struct {
	Repository string `json:"repository,omitempty"`
	Suffix     string `json:"suffix,omitempty"`
}

func init() {
	SchemeBuilder.Register(&ImageConfig{}, &Images{})
}
