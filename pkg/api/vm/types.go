package vm

import "github.com/rancher/wrangler/v3/pkg/condition"

var (
	vmReady   condition.Cond = "Ready"
	vmiPaused condition.Cond = "Paused"
)

type EjectCdRomActionInput struct {
	DiskNames []string `json:"diskNames,omitempty"`
}

type BackupInput struct {
	Name string `json:"name"`
}

type RestoreInput struct {
	Name       string `json:"name"`
	BackupName string `json:"backupName"`
}

type MigrateInput struct {
	NodeName string `json:"nodeName"`
}

type CreateTemplateInput struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	WithData    bool   `json:"withData"`
}

type AddVolumeInput struct {
	DiskName         string `json:"diskName"`
	VolumeSourceName string `json:"volumeSourceName"`
}

type RemoveVolumeInput struct {
	DiskName string `json:"diskName"`
}

type CloneInput struct {
	TargetVM    string `json:"targetVm"`
	RunStrategy string `json:"runStrategy"`
}

type FindMigratableNodesOutput struct {
	Nodes []string `json:"nodes"`
}

type UpdateResourceQuotaInput struct {
	TotalSnapshotSizeQuota string `json:"totalSnapshotSizeQuota"`
}

type CPUAndMemoryHotplugInput struct {
	Sockets uint32 `json:"sockets"`
	Memory  string `json:"memory"`
}
