/*
Copyright 2024 The Kubernetes Authors.

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
)

const (
	// PodDrainLabel is the label that can be set on Pods in workload clusters to ensure a Pod is not drained.
	// The only valid values are "skip" and "wait-completed".
	// This label takes precedence over MachineDrainRules defined in the management cluster.
	PodDrainLabel = "cluster.x-k8s.io/drain"
)

// MachineDrainRuleDrainBehavior defines the drain behavior. Can be either "Drain", "Skip", or "WaitCompleted".
// +kubebuilder:validation:Enum=Drain;Skip;WaitCompleted
type MachineDrainRuleDrainBehavior string

const (
	// MachineDrainRuleDrainBehaviorDrain means a Pod should be drained.
	MachineDrainRuleDrainBehaviorDrain MachineDrainRuleDrainBehavior = "Drain"

	// MachineDrainRuleDrainBehaviorSkip means the drain for a Pod should be skipped.
	MachineDrainRuleDrainBehaviorSkip MachineDrainRuleDrainBehavior = "Skip"

	// MachineDrainRuleDrainBehaviorWaitCompleted means the Pod should not be evicted,
	// but overall drain should wait until the Pod completes.
	MachineDrainRuleDrainBehaviorWaitCompleted MachineDrainRuleDrainBehavior = "WaitCompleted"
)

// MachineDrainRuleSpec defines the spec of a MachineDrainRule.
type MachineDrainRuleSpec struct {
	// drain configures if and how Pods are drained.
	// +required
	Drain MachineDrainRuleDrainConfig `json:"drain"`

	// machines defines to which Machines this MachineDrainRule should be applied.
	//
	// If machines is not set, the MachineDrainRule applies to all Machines in the Namespace.
	// If machines contains multiple selectors, the results are ORed.
	// Within a single Machine selector the results of selector and clusterSelector are ANDed.
	// Machines will be selected from all Clusters in the Namespace unless otherwise
	// restricted with the clusterSelector.
	//
	// Example: Selects control plane Machines in all Clusters or
	//          Machines with label "os" == "linux" in Clusters with label
	//          "stage" == "production".
	//
	//  - selector:
	//      matchExpressions:
	//      - key: cluster.x-k8s.io/control-plane
	//        operator: Exists
	//  - selector:
	//      matchLabels:
	//        os: linux
	//    clusterSelector:
	//      matchExpressions:
	//      - key: stage
	//        operator: In
	//        values:
	//        - production
	//
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	Machines []MachineDrainRuleMachineSelector `json:"machines,omitempty"`

	// pods defines to which Pods this MachineDrainRule should be applied.
	//
	// If pods is not set, the MachineDrainRule applies to all Pods in all Namespaces.
	// If pods contains multiple selectors, the results are ORed.
	// Within a single Pod selector the results of selector and namespaceSelector are ANDed.
	// Pods will be selected from all Namespaces unless otherwise
	// restricted with the namespaceSelector.
	//
	// Example: Selects Pods with label "app" == "logging" in all Namespaces or
	//          Pods with label "app" == "prometheus" in the "monitoring"
	//          Namespace.
	//
	//  - selector:
	//      matchExpressions:
	//      - key: app
	//        operator: In
	//        values:
	//        - logging
	//  - selector:
	//      matchLabels:
	//        app: prometheus
	//    namespaceSelector:
	//      matchLabels:
	//        kubernetes.io/metadata.name: monitoring
	//
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	Pods []MachineDrainRulePodSelector `json:"pods,omitempty"`
}

// MachineDrainRuleDrainConfig configures if and how Pods are drained.
type MachineDrainRuleDrainConfig struct {
	// behavior defines the drain behavior.
	// Can be either "Drain", "Skip", or "WaitCompleted".
	// "Drain" means that the Pods to which this MachineDrainRule applies will be drained.
	// If behavior is set to "Drain" the order in which Pods are drained can be configured
	// with the order field. When draining Pods of a Node the Pods will be grouped by order
	// and one group after another will be drained (by increasing order). Cluster API will
	// wait until all Pods of a group are terminated / removed from the Node before starting
	// with the next group.
	// "Skip" means that the Pods to which this MachineDrainRule applies will be skipped during drain.
	// "WaitCompleted" means that the pods to which this MachineDrainRule applies will never be evicted
	// and we wait for them to be completed, it is enforced that pods marked with this behavior always have Order=0.
	// +required
	Behavior MachineDrainRuleDrainBehavior `json:"behavior"`

	// order defines the order in which Pods are drained.
	// Pods with higher order are drained after Pods with lower order.
	// order can only be set if behavior is set to "Drain".
	// If order is not set, 0 will be used.
	// Valid values for order are from -2147483648 to 2147483647 (inclusive).
	// +optional
	Order *int32 `json:"order,omitempty"`
}

// MachineDrainRuleMachineSelector defines to which Machines this MachineDrainRule should be applied.
// +kubebuilder:validation:MinProperties=1
type MachineDrainRuleMachineSelector struct {
	// selector is a label selector which selects Machines by their labels.
	// This field follows standard label selector semantics; if not present or
	// empty, it selects all Machines.
	//
	// If clusterSelector is also set, then the selector as a whole selects
	// Machines matching selector belonging to Clusters selected by clusterSelector.
	// If clusterSelector is not set, it selects all Machines matching selector in
	// all Clusters.
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// clusterSelector is a label selector which selects Machines by the labels of
	// their Clusters.
	// This field follows standard label selector semantics; if not present or
	// empty, it selects Machines of all Clusters.
	//
	// If selector is also set, then the selector as a whole selects
	// Machines matching selector belonging to Clusters selected by clusterSelector.
	// If selector is not set, it selects all Machines belonging to Clusters
	// selected by clusterSelector.
	// +optional
	ClusterSelector *metav1.LabelSelector `json:"clusterSelector,omitempty"`
}

// MachineDrainRulePodSelector defines to which Pods this MachineDrainRule should be applied.
// +kubebuilder:validation:MinProperties=1
type MachineDrainRulePodSelector struct {
	// selector is a label selector which selects Pods by their labels.
	// This field follows standard label selector semantics; if not present or
	// empty, it selects all Pods.
	//
	// If namespaceSelector is also set, then the selector as a whole selects
	// Pods matching selector in Namespaces selected by namespaceSelector.
	// If namespaceSelector is not set, it selects all Pods matching selector in
	// all Namespaces.
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// namespaceSelector is a label selector which selects Pods by the labels of
	// their Namespaces.
	// This field follows standard label selector semantics; if not present or
	// empty, it selects Pods of all Namespaces.
	//
	// If selector is also set, then the selector as a whole selects
	// Pods matching selector in Namespaces selected by namespaceSelector.
	// If selector is not set, it selects all Pods in Namespaces selected by
	// namespaceSelector.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=machinedrainrules,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Behavior",type="string",JSONPath=".spec.drain.behavior",description="Drain behavior"
// +kubebuilder:printcolumn:name="Order",type="string",JSONPath=".spec.drain.order",description="Drain order"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of the MachineDrainRule"

// MachineDrainRule is the Schema for the MachineDrainRule API.
type MachineDrainRule struct {
	metav1.TypeMeta `json:",inline"`

	// +required
	metav1.ObjectMeta `json:"metadata"`

	// spec defines the spec of a MachineDrainRule.
	// +required
	Spec MachineDrainRuleSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// MachineDrainRuleList contains a list of MachineDrainRules.
type MachineDrainRuleList struct {
	metav1.TypeMeta `json:",inline"`

	// +required
	metav1.ListMeta `json:"metadata"`

	// items contains the items of the MachineDrainRuleList.
	// +required
	Items []MachineDrainRule `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &MachineDrainRule{}, &MachineDrainRuleList{})
}
