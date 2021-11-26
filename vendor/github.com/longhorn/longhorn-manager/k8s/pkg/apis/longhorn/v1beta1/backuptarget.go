package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	BackupTargetConditionTypeUnavailable = "Unavailable"

	BackupTargetConditionReasonUnavailable = "Unavailable"
)

type BackupTargetSpec struct {
	BackupTargetURL  string          `json:"backupTargetURL"`
	CredentialSecret string          `json:"credentialSecret"`
	PollInterval     metav1.Duration `json:"pollInterval"`
	SyncRequestedAt  metav1.Time     `json:"syncRequestedAt"`
}

type BackupTargetStatus struct {
	OwnerID      string               `json:"ownerID"`
	Available    bool                 `json:"available"`
	Conditions   map[string]Condition `json:"conditions"`
	LastSyncedAt metav1.Time          `json:"lastSyncedAt"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type BackupTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              BackupTargetSpec   `json:"spec"`
	Status            BackupTargetStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type BackupTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []BackupTarget `json:"items"`
}
