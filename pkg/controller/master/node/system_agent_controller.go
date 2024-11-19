package node

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/containerd"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	systemAgentControllerName = "system-agent-controller"
)

var (
	watchedSettings = map[string]string{
		settings.ContainerdRegistrySettingName: "",
		settings.HTTPProxySettingName:          "",
	}
)

type systemAgentHandler struct {
	secrets     ctlcorev1.SecretClient
	secretCache ctlcorev1.SecretCache
}

// SystemAgentRegister registers a controller to update harvester-system-agent-plan secret
func SystemAgentRegister(ctx context.Context, management *config.Management, _ config.Options) error {
	secrets := management.CoreFactory.Core().V1().Secret()
	settingController := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	handler := &systemAgentHandler{
		secrets:     secrets,
		secretCache: secrets.Cache(),
	}

	settingController.OnChange(ctx, systemAgentControllerName, handler.onSettingChange)
	return nil
}

func (h *systemAgentHandler) onSettingChange(_ string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting.DeletionTimestamp != nil {
		return nil, nil
	}
	if _, ok := watchedSettings[setting.Name]; !ok {
		return nil, nil
	}

	serverPlan := &Plan{}
	agentPlan := &Plan{}
	if err := h.setServerPlan(serverPlan); err != nil {
		return nil, err
	}
	if err := h.setAgentPlan(agentPlan); err != nil {
		return nil, err
	}

	serverPlanJson, err := json.Marshal(serverPlan)
	if err != nil {
		logrus.WithError(err).WithField("plan", serverPlan).Error("Failed to marshal server plan")
		return nil, err
	}
	agentPlanJson, err := json.Marshal(agentPlan)
	if err != nil {
		logrus.WithError(err).WithField("plan", agentPlan).Error("Failed to marshal server plan")
		return nil, err
	}

	secret, err := h.secretCache.Get(util.CattleSystemNamespaceName, util.SystemAgentPlanSecretName)
	if err != nil {
		logrus.WithError(err).
			WithFields(logrus.Fields{
				"namespace": util.CattleSystemNamespaceName,
				"name":      util.SystemAgentPlanSecretName,
			}).Error("Failed to get secret")
		return nil, err
	}

	secretCopy := secret.DeepCopy()
	secret.Data[util.SystemAgentPlanServerPlanName] = serverPlanJson
	secret.Data[util.SystemAgentPlanAgentPlanName] = agentPlanJson
	if !reflect.DeepEqual(secret.Data, secretCopy.Data) {
		if _, err = h.secrets.Update(secret); err != nil {
			logrus.WithError(err).
				WithFields(logrus.Fields{
					"namespace": util.CattleSystemNamespaceName,
					"name":      util.SystemAgentPlanSecretName,
				}).Error("Failed to update secret")
			return nil, err
		}
	}
	return nil, nil
}

func (h *systemAgentHandler) setServerPlan(plan *Plan) error {
	if err := h.setContainerdRegistries(plan); err != nil {
		return err
	}

	if err := h.setHTTPProxy(plan, false); err != nil {
		return err
	}
	return nil
}

func (h *systemAgentHandler) setAgentPlan(plan *Plan) error {
	if err := h.setContainerdRegistries(plan); err != nil {
		return err
	}

	if err := h.setHTTPProxy(plan, true); err != nil {
		return err
	}
	return nil
}

func (h *systemAgentHandler) setContainerdRegistries(plan *Plan) error {
	value := settings.ContainerdRegistry.Get()
	if value == "" || value == "{}" {
		plan.Files = append(plan.Files, File{
			Path:    "/etc/rancher/rke2/registries.yaml",
			Content: base64.StdEncoding.EncodeToString([]byte("")),
		})
		return nil
	}

	registryFromSetting := &containerd.Registry{}
	if err := json.Unmarshal([]byte(value), registryFromSetting); err != nil {
		logrus.WithError(err).
			WithFields(logrus.Fields{
				"name":  settings.ContainerdRegistrySettingName,
				"value": value,
			}).Error("Failed to unmarshal setting value")
		return err
	}

	registryFromSettingYaml, err := yaml.Marshal(registryFromSetting)
	if err != nil {
		logrus.WithError(err).
			WithFields(logrus.Fields{
				"name":     settings.ContainerdRegistrySettingName,
				"registry": registryFromSetting,
			}).Error("Failed to marshal registry")
		return err
	}

	plan.Files = append(plan.Files, File{
		Path:    "/etc/rancher/rke2/registries.yaml",
		Content: base64.StdEncoding.EncodeToString(registryFromSettingYaml),
	})
	return nil
}

func (h *systemAgentHandler) setHTTPProxy(plan *Plan, isAgent bool) error {
	instruction := OneTimeInstruction{
		CommonInstruction: CommonInstruction{
			Name: "install",
			// TODO: Get RKE2 version from setting or somewhere else
			Image: "rancher/system-agent-installer-rke2:v1.29.9+rke2r1",
			Args: []string{
				"-c",
				"run.sh",
			},
			Command: "sh",
		},
		SaveOutput: true,
	}
	if isAgent {
		instruction.Env = []string{"INSTALL_RKE2_EXEC=agent"}
	}

	value := settings.HTTPProxy.Get()
	if value == "" || value == "{}" {
		plan.OneTimeInstructions = append(plan.OneTimeInstructions, instruction)
		return nil
	}

	var httpProxyConfig util.HTTPProxyConfig
	if err := json.Unmarshal([]byte(value), &httpProxyConfig); err != nil {
		logrus.WithError(err).
			WithFields(logrus.Fields{
				"name":  settings.HTTPProxySettingName,
				"value": value,
			}).Error("Failed to unmarshal setting value")
		return err
	}

	if httpProxyConfig.HTTPProxy != "" {
		instruction.Env = append(instruction.Env, "HTTP_PROXY="+httpProxyConfig.HTTPProxy)
	}
	if httpProxyConfig.HTTPSProxy != "" {
		instruction.Env = append(instruction.Env, "HTTPS_PROXY="+httpProxyConfig.HTTPSProxy)
	}
	if httpProxyConfig.NoProxy != "" {
		instruction.Env = append(instruction.Env, "NO_PROXY="+httpProxyConfig.NoProxy)
	}
	plan.OneTimeInstructions = append(plan.OneTimeInstructions, instruction)
	return nil
}
