package setting

import (
	"encoding/json"
	"strings"

	rkev1 "github.com/rancher/rancher/pkg/apis/rke.cattle.io/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	fleetLocalNamespace = "fleet-local"
	localClusterName    = "local"
)

func (h *Handler) syncHTTPProxy(setting *harvesterv1.Setting) error {
	// Add envs to the backup secret used by Longhorn backups
	var httpProxyConfig util.HTTPProxyConfig
	if err := json.Unmarshal([]byte(setting.Value), &httpProxyConfig); err != nil {
		return err
	}
	backupConfig := map[string]string{
		util.HTTPProxyEnv:  httpProxyConfig.HTTPProxy,
		util.HTTPSProxyEnv: httpProxyConfig.HTTPSProxy,
		util.NoProxyEnv:    util.AddBuiltInNoProxy(httpProxyConfig.NoProxy),
	}
	if err := h.updateBackupSecret(backupConfig); err != nil {
		return err
	}
	if err := h.syncRke2HTTPProxy(httpProxyConfig); err != nil {
		return err
	}

	//redeploy system services. The proxy envs will be injected by the mutation webhook.
	if err := h.redeployDeployment(util.CattleSystemNamespaceName, "rancher"); err != nil {
		return err
	}
	return h.redeployDeployment(h.namespace, "harvester")
}

func (h *Handler) syncRke2HTTPProxy(httpProxyConfig util.HTTPProxyConfig) error {
	localCluster, err := h.clusterCache.Get(fleetLocalNamespace, localClusterName)
	if err != nil {
		return err
	}
	toUpdate := localCluster.DeepCopy()
	var newEnvVars []rkev1.EnvVar
	for _, envVar := range toUpdate.Spec.AgentEnvVars {
		if !strings.HasSuffix(envVar.Name, "_PROXY") {
			newEnvVars = append(newEnvVars, envVar)
		}
	}
	newEnvVars = append(newEnvVars, rkev1.EnvVar{
		Name:  util.HTTPProxyEnv,
		Value: httpProxyConfig.HTTPProxy,
	}, rkev1.EnvVar{
		Name:  util.HTTPSProxyEnv,
		Value: httpProxyConfig.HTTPSProxy,
	}, rkev1.EnvVar{
		Name:  util.NoProxyEnv,
		Value: util.AddBuiltInNoProxy(httpProxyConfig.NoProxy),
	})
	toUpdate.Spec.AgentEnvVars = newEnvVars
	_, err = h.clusters.Update(toUpdate)

	return err
}
