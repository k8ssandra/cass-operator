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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cfg "sigs.k8s.io/controller-runtime/pkg/config/v1alpha1" //nolint:staticcheck
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

//+kubebuilder:object:root=true

// OperatorConfig is the Schema for the operatorconfigs API
type OperatorConfig struct {
	metav1.TypeMeta `json:",inline"`

	// ControllerManagerConfigurationSpec returns the configurations for controllers
	cfg.ControllerManagerConfigurationSpec `json:",inline"`

	// SkipValidatingWebhook replaces the old SKIP_VALIDATING_WEBHOOK env variable. If set to true, the webhooks are not initialized
	DisableWebhooks bool `json:"disableWebhooks,omitempty"`

	// ImageConfigFile indicates the path where to load the imageConfig from
	ImageConfigFile string `json:"imageConfigFile,omitempty"`

	// SecretProvider sets where the Secrets are loaded from. If unset, loads them from Kubernetes
	SecretProvider SecretProviderConfigSpec `json:"secretProvider,omitempty"`
}

type SecretProviderConfigSpec struct {
	Source SecretProviderSource `json:",inline,omitempty"`
}

type SecretProviderSource struct {
	Filesystem *SecretProviderFilesystemSource `json:"filesystem,omitempty"`
}

type SecretProviderFilesystemSource struct {
	Path            string `json:"path"`
	NamespacedPaths bool   `json:"namespaced,omitempty"`
}

func init() {
	SchemeBuilder.Register(&OperatorConfig{})
}
