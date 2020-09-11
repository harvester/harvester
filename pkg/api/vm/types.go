package vm

import "github.com/rancher/wrangler/pkg/condition"

var (
	vmReady   condition.Cond = "Ready"
	vmiPaused condition.Cond = "Paused"
)

type EjectCdRomActionInput struct {
	DiskNames []string `json:"diskNames,omitempty"`
}
