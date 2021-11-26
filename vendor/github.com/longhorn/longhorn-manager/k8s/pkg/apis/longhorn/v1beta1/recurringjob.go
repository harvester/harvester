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

type RecurringJobSpec struct {
	Name        string            `json:"name"`
	Groups      []string          `json:"groups,omitempty"`
	Task        RecurringJobType  `json:"task"`
	Cron        string            `json:"cron"`
	Retain      int               `json:"retain"`
	Concurrency int               `json:"concurrency"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type RecurringJobStatus struct {
	OwnerID string `json:"ownerID"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RecurringJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              RecurringJobSpec   `json:"spec"`
	Status            RecurringJobStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RecurringJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []RecurringJob `json:"items"`
}
