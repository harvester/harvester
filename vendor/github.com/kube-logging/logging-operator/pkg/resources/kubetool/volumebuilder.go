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

// VolumeBuilder .
type VolumeBuilder struct {
	corev1.Volume
}

// NewVolumeBuilder .
func NewVolumeBuilder() *VolumeBuilder {
	return &VolumeBuilder{}
}

// WithName .
func (v *VolumeBuilder) WithName(name string) *VolumeBuilder {
	if v != nil {
		v.Name = name
	}
	return v
}

// WithVolumeSource .
func (v *VolumeBuilder) WithVolumeSource(volumeSource corev1.VolumeSource) *VolumeBuilder {
	if v != nil {
		v.VolumeSource = volumeSource
	}
	return v
}

// WithEmptyDir .
func (v *VolumeBuilder) WithEmptyDir(emptyDir corev1.EmptyDirVolumeSource) *VolumeBuilder {
	if v != nil {
		v.EmptyDir = &emptyDir
	}
	return v
}

// WithHostPath .
func (v *VolumeBuilder) WithHostPath(hostPath corev1.HostPathVolumeSource) *VolumeBuilder {
	if v != nil {
		v.HostPath = &hostPath
	}
	return v
}

// WithHostPathFromPath .
func (v *VolumeBuilder) WithHostPathFromPath(path string) *VolumeBuilder {
	if v != nil {
		v.WithHostPath(corev1.HostPathVolumeSource{Path: path})
	}
	return v
}
