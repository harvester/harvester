// Copyright © 2019 Banzai Cloud
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

package kubetool

import (
	corev1 "k8s.io/api/core/v1"
)

// VolumeMountBuilder .
type VolumeMountBuilder struct {
	corev1.VolumeMount
}

// MountPropagationModeRef .
func MountPropagationModeRef(mountPropagationMode corev1.MountPropagationMode) *corev1.MountPropagationMode {
	return &mountPropagationMode
}

// NewVolumeMountBuilder .
func NewVolumeMountBuilder() *VolumeMountBuilder {
	return &VolumeMountBuilder{}
}

// WithName .
func (v *VolumeMountBuilder) WithName(name string) *VolumeMountBuilder {
	if v != nil {
		v.Name = name
	}
	return v
}

// WithMountPath .
func (v *VolumeMountBuilder) WithMountPath(path string) *VolumeMountBuilder {
	if v != nil {
		v.MountPath = path
	}
	return v
}

// WithSubPath .
func (v *VolumeMountBuilder) WithSubPath(path string) *VolumeMountBuilder {
	if v != nil {
		v.SubPath = path
	}
	return v
}

// WithSubPathExpr .
func (v *VolumeMountBuilder) WithSubPathExpr(expr string) *VolumeMountBuilder {
	if v != nil {
		v.SubPathExpr = expr
	}
	return v
}

// WithMountPropagation .
func (v *VolumeMountBuilder) WithMountPropagation(mountPropagation corev1.MountPropagationMode) *VolumeMountBuilder {
	if v != nil {
		v.MountPropagation = &mountPropagation
	}
	return v
}

// WithReadOnly .
func (v *VolumeMountBuilder) WithReadOnly(readonly bool) *VolumeMountBuilder {
	if v != nil {
		v.ReadOnly = readonly
	}
	return v
}
