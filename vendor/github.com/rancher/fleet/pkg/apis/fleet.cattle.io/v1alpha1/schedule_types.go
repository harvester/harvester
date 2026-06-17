package v1alpha1

import (
	"github.com/rancher/wrangler/v3/pkg/genericcondition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	InternalSchemeBuilder.Register(&Schedule{}, &ScheduleList{})
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`
// +kubebuilder:printcolumn:name="Duration",type=string,JSONPath=`.spec.duration`
// +kubebuilder:printcolumn:name="Active",type=string,JSONPath=`.status.active`
// +kubebuilder:printcolumn:name="NextStart",type="string",JSONPath=`.status.nextStartTime`

// Schedule represents a schedule in which deployments are allowed or not, depending on its definition.
type Schedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScheduleSpec   `json:"spec,omitempty"`
	Status ScheduleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScheduleList contains a list of Schedule
type ScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Schedule `json:"items"`
}

type ScheduleSpec struct {
	Schedule string          `json:"schedule,omitempty"`
	Duration metav1.Duration `json:"duration,omitempty"`
	Location string          `json:"location,omitempty"`
	// Targets is a list of resources affected by this schedule
	Targets ScheduleTargets `json:"targets,omitempty"`
}

type ScheduleStatus struct {
	// Active is set to true when the Schedule is actively running
	Active bool `json:"active,omitempty"`
	// NextStartTime stores the next time this Schedule will start
	NextStartTime metav1.Time `json:"nextStartTime,omitempty"`
	// Conditions is a list of Wrangler conditions that describe the state
	// of the resource.
	Conditions []genericcondition.GenericCondition `json:"conditions,omitempty"`
	// MatchingClusters is the list of clusters targeted by the Schedule
	MatchingClusters []string `json:"matchingClusters,omitempty"`
}

type ScheduleTargets struct {
	Clusters []ScheduleTarget `json:"clusters,omitempty"`
}

// ScheduleTarget represents a resource (or group of resources) affected by a Schedule
type ScheduleTarget struct {
	// Name is the name of this target.
	// +nullable
	Name string `json:"name,omitempty"`
	// ClusterName is the name of a cluster.
	// +nullable
	ClusterName string `json:"clusterName,omitempty"`
	// ClusterSelector is a label selector to select clusters.
	// +nullable
	ClusterSelector *metav1.LabelSelector `json:"clusterSelector,omitempty"`
	// ClusterGroup is the name of a cluster group in the same namespace as the clusters.
	// +nullable
	ClusterGroup string `json:"clusterGroup,omitempty"`
	// ClusterGroupSelector is a label selector to select cluster groups.
	// +nullable
	ClusterGroupSelector *metav1.LabelSelector `json:"clusterGroupSelector,omitempty"`
}
