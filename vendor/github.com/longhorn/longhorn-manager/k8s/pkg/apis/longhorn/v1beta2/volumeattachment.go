package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AttachmentTicket struct {
	// The unique ID of this attachment. Used to differentiate different attachments of the same volume.
	// +optional
	ID string `json:"id"`
	// +optional
	Type AttacherType `json:"type"`
	// The node that this attachment is requesting
	// +optional
	NodeID string `json:"nodeID"`
	// Optional additional parameter for this attachment
	// +optional
	Parameters map[string]string `json:"parameters"`
	// A sequence number representing a specific generation of the desired state.
	// Populated by the system. Read-only.
	// +optional
	Generation int64 `json:"generation"`
}

type AttachmentTicketStatus struct {
	// The unique ID of this attachment. Used to differentiate different attachments of the same volume.
	// +optional
	ID string `json:"id"`
	// Indicate whether this attachment ticket has been satisfied
	Satisfied bool `json:"satisfied"`
	// Record any error when trying to fulfill this attachment
	// +nullable
	Conditions []Condition `json:"conditions"`
	// A sequence number representing a specific generation of the desired state.
	// Populated by the system. Read-only.
	// +optional
	Generation int64 `json:"generation"`
}

type AttacherType string

const (
	AttacherTypeCSIAttacher                      = AttacherType("csi-attacher")
	AttacherTypeLonghornAPI                      = AttacherType("longhorn-api")
	AttacherTypeSnapshotController               = AttacherType("snapshot-controller")
	AttacherTypeBackupController                 = AttacherType("backup-controller")
	AttacherTypeVolumeCloneController            = AttacherType("volume-clone-controller")
	AttacherTypeSalvageController                = AttacherType("salvage-controller")
	AttacherTypeShareManagerController           = AttacherType("share-manager-controller")
	AttacherTypeVolumeRestoreController          = AttacherType("volume-restore-controller")
	AttacherTypeVolumeEvictionController         = AttacherType("volume-eviction-controller")
	AttacherTypeVolumeExpansionController        = AttacherType("volume-expansion-controller")
	AttacherTypeBackingImageDataSourceController = AttacherType("bim-ds-controller")
	AttacherTypeVolumeRebuildingController       = AttacherType("volume-rebuilding-controller")
)

const (
	AttacherPriorityLevelVolumeRestoreController          = 2000
	AttacherPriorityLevelVolumeExpansionController        = 2000
	AttacherPriorityLevelLonghornAPI                      = 1000
	AttacherPriorityLevelCSIAttacher                      = 900
	AttacherPriorityLevelSalvageController                = 900
	AttacherPriorityLevelShareManagerController           = 900
	AttacherPriorityLevelSnapshotController               = 800
	AttacherPriorityLevelBackupController                 = 800
	AttacherPriorityLevelVolumeCloneController            = 800
	AttacherPriorityLevelVolumeEvictionController         = 800
	AttacherPriorityLevelBackingImageDataSourceController = 800
	AttachedPriorityLevelVolumeRebuildingController       = 800
)

const (
	TrueValue  = "true"
	FalseValue = "false"
	AnyValue   = "any"

	AttachmentParameterDisableFrontend = "disableFrontend"
	AttachmentParameterLastAttachedBy  = "lastAttachedBy"
)

const (
	AttachmentStatusConditionTypeSatisfied = "Satisfied"

	AttachmentStatusConditionReasonAttachedWithIncompatibleParameters = "AttachedWithIncompatibleParameters"
)

func GetAttacherPriorityLevel(t AttacherType) int {
	switch t {
	case AttacherTypeCSIAttacher:
		return AttacherPriorityLevelCSIAttacher
	case AttacherTypeLonghornAPI:
		return AttacherPriorityLevelLonghornAPI
	case AttacherTypeSnapshotController:
		return AttacherPriorityLevelSnapshotController
	case AttacherTypeBackupController:
		return AttacherPriorityLevelBackupController
	case AttacherTypeVolumeCloneController:
		return AttacherPriorityLevelVolumeCloneController
	case AttacherTypeSalvageController:
		return AttacherPriorityLevelSalvageController
	case AttacherTypeShareManagerController:
		return AttacherPriorityLevelShareManagerController
	case AttacherTypeVolumeRestoreController:
		return AttacherPriorityLevelVolumeRestoreController
	case AttacherTypeVolumeRebuildingController:
		return AttachedPriorityLevelVolumeRebuildingController
	case AttacherTypeVolumeEvictionController:
		return AttacherPriorityLevelVolumeEvictionController
	case AttacherTypeVolumeExpansionController:
		return AttacherPriorityLevelVolumeExpansionController
	case AttacherTypeBackingImageDataSourceController:
		return AttacherPriorityLevelBackingImageDataSourceController
	default:
		return 0
	}
}

func GetAttachmentTicketID(attacherType AttacherType, id string) string {
	retID := string(attacherType) + "-" + id
	if len(retID) > 253 {
		return retID[:253]
	}
	return retID
}

func IsAttachmentTicketSatisfied(attachmentID string, va *VolumeAttachment) bool {
	if va == nil {
		return false
	}
	attachmentTicket, ok := va.Spec.AttachmentTickets[attachmentID]
	if !ok {
		return false
	}
	attachmentTicketStatus, ok := va.Status.AttachmentTicketStatuses[attachmentID]
	if !ok {
		return false
	}

	return attachmentTicket.Generation == attachmentTicketStatus.Generation && attachmentTicketStatus.Satisfied
}

// VolumeAttachmentSpec defines the desired state of Longhorn VolumeAttachment
type VolumeAttachmentSpec struct {
	// +optional
	AttachmentTickets map[string]*AttachmentTicket `json:"attachmentTickets"`
	// The name of Longhorn volume of this VolumeAttachment
	Volume string `json:"volume"`
}

// VolumeAttachmentStatus defines the observed state of Longhorn VolumeAttachment
type VolumeAttachmentStatus struct {
	// +optional
	AttachmentTicketStatuses map[string]*AttachmentTicketStatus `json:"attachmentTicketStatuses"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhva
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VolumeAttachment stores attachment information of a Longhorn volume
type VolumeAttachment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VolumeAttachmentSpec   `json:"spec,omitempty"`
	Status VolumeAttachmentStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeAttachmentList contains a list of VolumeAttachments
type VolumeAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VolumeAttachment `json:"items"`
}
