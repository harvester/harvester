package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type RecurringJobType string

const (
	RecurringJobTypeSnapshot = RecurringJobType("snapshot")
	RecurringJobTypeBackup   = RecurringJobType("backup")

	RecurringJobGroupDefault = "default"
)

type VolumeRecurringJob struct {
	Name    string `json:"name"`
	IsGroup bool   `json:"isGroup"`
}

// RecurringJobSpec defines the desired state of the Longhorn recurring job
type RecurringJobSpec struct {
	// The recurring job name.
	Name string `json:"name"`
	// The recurring job group.
	Groups []string `json:"groups,omitempty"`
	// The recurring job type.
	// Can be "snapshot" or "backup".
	Task RecurringJobType `json:"task"`
	// The cron setting.
	Cron string `json:"cron"`
	// The retain count of the snapshot/backup.
	Retain int `json:"retain"`
	// The concurrency of taking the snapshot/backup.
	Concurrency int `json:"concurrency"`
	// The label of the snapshot/backup.
	Labels map[string]string `json:"labels,omitempty"`
}

// RecurringJobStatus defines the observed state of the Longhorn recurring job
type RecurringJobStatus struct {
	// The owner ID which is responsible to reconcile this recurring job CR.
	OwnerID string `json:"ownerID"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhrj
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Groups",type=string,JSONPath=`.spec.groups`,description="Sets groupings to the jobs. When set to \"default\" group will be added to the volume label when no other job label exist in volume"
// +kubebuilder:printcolumn:name="Task",type=string,JSONPath=`.spec.task`,description="Should be one of \"backup\" or \"snapshot\""
// +kubebuilder:printcolumn:name="Cron",type=string,JSONPath=`.spec.cron`,description="The cron expression represents recurring job scheduling"
// +kubebuilder:printcolumn:name="Retain",type=integer,JSONPath=`.spec.retain`,description="The number of snapshots/backups to keep for the volume"
// +kubebuilder:printcolumn:name="Concurrency",type=integer,JSONPath=`.spec.concurrency`,description="The concurrent job to run by each cron job"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Labels",type=string,JSONPath=`.spec.labels`,description="Specify the labels"

// RecurringJob is where Longhorn stores recurring job object.
type RecurringJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Spec RecurringJobSpec `json:"spec,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Status RecurringJobStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RecurringJobList is a list of RecurringJobs.
type RecurringJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RecurringJob `json:"items"`
}
