package setting

import (
	"encoding/json"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

func (h *Handler) syncHTTPProxy(setting *harvesterv1.Setting) error {
	// Add envs to the backup secret used by Longhorn backups
	var httpProxyConfig util.HTTPProxyConfig
	if err := json.Unmarshal([]byte(setting.Value), &httpProxyConfig); err != nil {
		return err
	}
	backupConfig := map[string]string{
		"HTTP_PROXY":  httpProxyConfig.HTTPProxy,
		"HTTPS_PROXY": httpProxyConfig.HTTPSProxy,
		"NO_PROXY":    util.AddBuiltInNoProxy(httpProxyConfig.NoProxy),
	}
	if err := h.updateBackupSecret(backupConfig); err != nil {
		return err
	}

	//redeploy system services. The proxy envs will be injected by the mutation webhook.
	return h.redeployDeployment(h.namespace, "harvester")
}
