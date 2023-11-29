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

package utils

import (
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type RuntimeObjects []runtime.Object
type ResourceOrder string
type ResourceScoreFunc func(o runtime.Object) int

const (
	InstallResourceOrder   ResourceOrder = "Install"
	UninstallResourceOrder ResourceOrder = "Uninstall"
)

func (os RuntimeObjects) Sort(order ResourceOrder) {
	var score ResourceScoreFunc

	switch order {
	case InstallResourceOrder:
		score = InstallObjectOrder()
	case UninstallResourceOrder:
		score = UninstallObjectOrder()
	default:
		score = func(o runtime.Object) int { return 0 }
	}

	sort.Slice(os, func(i, j int) bool {
		iScore := score(os[i])
		jScore := score(os[j])
		iGVK := os[i].GetObjectKind().GroupVersionKind()
		jGVK := os[j].GetObjectKind().GroupVersionKind()
		var iName, jName string
		if o, ok := os[i].(metav1.Object); ok {
			iName = o.GetName()
		}
		if o, ok := os[j].(metav1.Object); ok {
			jName = o.GetName()
		}
		return iScore < jScore ||
			(iScore == jScore &&
				iGVK.Group < jGVK.Group) ||
			(iScore == jScore &&
				iGVK.Group == jGVK.Group &&
				iGVK.Kind < os[j].GetObjectKind().GroupVersionKind().Kind) ||
			(iScore == jScore &&
				iGVK.Group == jGVK.Group &&
				iGVK.Kind == jGVK.Kind &&
				iName < jName)
	})
}

func InstallObjectOrder() func(o runtime.Object) int {
	var Order = []string{
		"CustomResourceDefinition",
		"Namespace",
		"ResourceQuota",
		"LimitRange",
		"PodSecurityPolicy",
		"PodDisruptionBudget",
		"Secret",
		"ConfigMap",
		"StorageClass",
		"PersistentVolume",
		"PersistentVolumeClaim",
		"ServiceAccount",
		"ClusterRole",
		"ClusterRoleList",
		"ClusterRoleBinding",
		"ClusterRoleBindingList",
		"Role",
		"RoleList",
		"RoleBinding",
		"RoleBindingList",
		"Service",
		"DaemonSet",
		"Pod",
		"ReplicationController",
		"ReplicaSet",
		"Deployment",
		"HorizontalPodAutoscaler",
		"StatefulSet",
		"Job",
		"CronJob",
		"Ingress",
		"APIService",
		"ValidatingWebhookConfiguration",
		"MutatingWebhookConfiguration",
	}

	order := make(map[string]int, len(Order))
	for i, kind := range Order {
		order[kind] = i
	}

	return func(o runtime.Object) int {
		if nr, ok := order[o.GetObjectKind().GroupVersionKind().Kind]; ok {
			return nr
		}
		return 1000
	}
}

func UninstallObjectOrder() func(o runtime.Object) int {
	var Order = []string{
		"MutatingWebhookConfiguration",
		"ValidatingWebhookConfiguration",
		"APIService",
		"Ingress",
		"Service",
		"CronJob",
		"Job",
		"StatefulSet",
		"HorizontalPodAutoscaler",
		"Deployment",
		"ReplicaSet",
		"ReplicationController",
		"Pod",
		"DaemonSet",
		"RoleBindingList",
		"RoleBinding",
		"RoleList",
		"Role",
		"ClusterRoleBindingList",
		"ClusterRoleBinding",
		"ClusterRoleList",
		"ClusterRole",
		"ServiceAccount",
		"PersistentVolumeClaim",
		"PersistentVolume",
		"StorageClass",
		"ConfigMap",
		"Secret",
		"PodDisruptionBudget",
		"PodSecurityPolicy",
		"LimitRange",
		"ResourceQuota",
		"Policy",
		"Gateway",
		"VirtualService",
		"DestinationRule",
		"Handler",
		"Instance",
		"Rule",
		"Namespace",
		"CustomResourceDefinition",
	}

	order := make(map[string]int, len(Order))
	for i, kind := range Order {
		order[kind] = i
	}

	return func(o runtime.Object) int {
		if nr, ok := order[o.GetObjectKind().GroupVersionKind().Kind]; ok {
			return nr
		}
		return 0
	}
}
