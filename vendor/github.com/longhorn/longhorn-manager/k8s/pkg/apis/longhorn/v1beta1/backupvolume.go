package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type BackupVolumeSpec struct {
	SyncRequestedAt metav1.Time `json:"syncRequestedAt"`
}

type BackupVolumeStatus struct {
	OwnerID              string            `json:"ownerID"`
	LastModificationTime metav1.Time       `json:"lastModificationTime"`
	Size                 string            `json:"size"`
	Labels               map[string]string `json:"labels"`
	CreatedAt            string            `json:"createdAt"`
	LastBackupName       string            `json:"lastBackupName"`
	LastBackupAt         string            `json:"lastBackupAt"`
	DataStored           string            `json:"dataStored"`
	Messages             map[string]string `json:"messages"`
	BackingImageName     string            `json:"backingImageName"`
	BackingImageChecksum string            `json:"backingImageChecksum"`
	LastSyncedAt         metav1.Time       `json:"lastSyncedAt"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type BackupVolume struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              BackupVolumeSpec   `json:"spec"`
	Status            BackupVolumeStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type BackupVolumeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []BackupVolume `json:"items"`
}
