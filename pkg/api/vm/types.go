package vm

import "github.com/rancher/wrangler/pkg/condition"

var (
	vmReady   condition.Cond = "Ready"
	vmiPaused condition.Cond = "Paused"
)

type EjectCdRomActionInput struct {
	DiskNames []string `json:"diskNames" validate:"required,gt=0"`
}

type BackupInput struct {
	Name string `json:"name" validate:"required"`
}

type RestoreInput struct {
	Name       string `json:"name" validate:"required"`
	BackupName string `json:"backupName" validate:"required"`
}

type MigrateInput struct {
	NodeName string `json:"nodeName" validate:"required"`
}

type CreateTemplateInput struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description,omitempty"`
}

type AddVolumeInput struct {
	DiskName         string `json:"diskName" validate:"required"`
	VolumeSourceName string `json:"volumeSourceName" validate:"required"`
}

type RemoveVolumeInput struct {
	DiskName string `json:"diskName" validate:"required"`
}

type ExportVolumeInput struct {
	DisplayName string `json:"displayName" validate:"required"`
	Namespace   string `json:"namespace" validate:"required"`
}
