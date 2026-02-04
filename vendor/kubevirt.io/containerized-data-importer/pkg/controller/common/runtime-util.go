/*
Copyright 2022 The CDI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
limitations under the License.
See the License for the specific language governing permissions and
*/

package common

import (
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"kubevirt.io/containerized-data-importer/pkg/common"
)

// MakeEmptyCDIConfigSpec creates cdi config manifest
func MakeEmptyCDIConfigSpec(name string) *cdiv1.CDIConfig {
	return &cdiv1.CDIConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CDIConfig",
			APIVersion: "cdi.kubevirt.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				common.CDILabelKey:       common.CDILabelValue,
				common.CDIComponentLabel: "",
			},
		},
	}
}

// MakeEmptyCDICR creates CDI CustomResouce manifest
func MakeEmptyCDICR() *cdiv1.CDI {
	return &cdiv1.CDI{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CDI",
			APIVersion: "cdis.cdi.kubevirt.io",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "cdi",
		},
	}
}

// IgnoreNotFound returns nil if the error is a NotFound error.
// We generally want to ignore (not requeue) NotFound errors, since we'll get a reconciliation request once the
// object exists, and requeuing in the meantime won't help.
func IgnoreNotFound(err error) error {
	if errors.IsNotFound(err) {
		return nil
	}
	return err
}

// IgnoreIsNoMatchError returns nil if the error is a IsNoMatchError.
// We will want to ignore this error for optional CRDs, if it is not found, just ignore it.
func IgnoreIsNoMatchError(err error) error {
	if meta.IsNoMatchError(err) {
		return nil
	}
	return err
}
