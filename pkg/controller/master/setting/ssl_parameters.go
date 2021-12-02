package setting

import (
	"encoding/json"

	"github.com/rancher/wrangler/pkg/data"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

func (h *Handler) syncSSLParameters(setting *harvesterv1.Setting) error {
	sslParameter := &settings.SSLParameter{}
	value := setting.Value
	if value == "" {
		value = setting.Default
	}
	if err := json.Unmarshal([]byte(value), sslParameter); err != nil {
		return err
	}

	return h.updateSSLParameters(sslParameter)
}

func (h *Handler) updateSSLParameters(sslParameter *settings.SSLParameter) error {
	logrus.Infof("Update SSL Parameters: Ciphers: %s, Protocols: %s", sslParameter.Ciphers, sslParameter.Protocols)

	helmChartConfig, err := h.helmChartConfigCache.Get(util.KubeSystemNamespace, util.Rke2IngressNginxAppName)
	if err != nil {
		return err
	}
	toUpdateHelmChartConfig := helmChartConfig.DeepCopy()

	var values = make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(helmChartConfig.Spec.ValuesContent), &values); err != nil {
		return err
	}

	if sslParameter.Ciphers == "" {
		data.RemoveValue(values, "controller", "config", "ssl-ciphers")
	} else {
		data.PutValue(values, sslParameter.Ciphers, "controller", "config", "ssl-ciphers")
	}
	if sslParameter.Protocols == "" {
		data.RemoveValue(values, "controller", "config", "ssl-protocols")
	} else {
		data.PutValue(values, sslParameter.Protocols, "controller", "config", "ssl-protocols")
	}

	newValuesContent, err := yaml.Marshal(values)
	if err != nil {
		return err
	}

	toUpdateHelmChartConfig.Spec.ValuesContent = string(newValuesContent)
	if _, err := h.helmChartConfigs.Update(toUpdateHelmChartConfig); err != nil {
		return err
	}

	return nil
}
