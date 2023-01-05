// Copyright © 2020 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package secret

import (
	corev1 "k8s.io/api/core/v1"
)

//nolint:unused,deadcode
// +docName:"Secret abstraction"
// Provides an abstraction to refer to secrets from other types
// SecretLoader facilitates loading the secrets from an operator.
// Leverages core types from kubernetes/api/core/v1
type _docSecret interface{}

//nolint:unused,deadcode
// +name:"Secret"
// +description:"Secret referencing abstraction"
type _metaSecret interface{}

// +kubebuilder:object:generate=true

type Secret struct {
	// Refers to a non-secret value
	Value     string     `json:"value,omitempty"`
	// Refers to a secret value to be used directly
	ValueFrom *ValueFrom `json:"valueFrom,omitempty"`
	// Refers to a secret value to be used through a volume mount
	MountFrom *ValueFrom `json:"mountFrom,omitempty"`
}

// +kubebuilder:object:generate=true

type ValueFrom struct {
	SecretKeyRef *corev1.SecretKeySelector `json:"secretKeyRef,omitempty"`
}
