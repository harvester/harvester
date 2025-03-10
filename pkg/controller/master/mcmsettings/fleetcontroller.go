package mcmsettings

import (
	"encoding/base64"
	"fmt"

	mgmtv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/wrangler/v3/pkg/relatedresource"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/harvester/harvester/pkg/util"
)

// watchFleetControllerConfigMap will watch the configmap for fleet-controller setting and requeue internal-server-url and internal-cacerts to trigger reconcile of associated settings to ensure correct values are applied
func (h *mcmSettingsHandler) watchFleetControllerConfigMap(_, _ string, obj runtime.Object) ([]relatedresource.Key, error) {
	if cm, ok := obj.(*corev1.ConfigMap); ok {
		if cm.Name == util.DefaultFleetControllerConfigMapName && cm.Namespace == util.DefaultFleetControllerConfigMapNamespace {
			return []relatedresource.Key{
				{
					Name: util.RancherInternalCASetting,
				},
				{
					Name: util.RancherInternalServerURLSetting,
				},
			}, nil
		}
	}
	return nil, nil
}

func (h *mcmSettingsHandler) watchInternalEndpointSettings(_ string, setting *mgmtv3.Setting) (*mgmtv3.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil || (setting.Name != util.RancherInternalCASetting && setting.Name != util.RancherInternalServerURLSetting) {
		return setting, nil
	}

	logrus.Debugf("reconcilling fleet-controller config for setting %s", setting.Name)

	fleetControllerConfig, err := h.configMapCache.Get(util.DefaultFleetControllerConfigMapNamespace, util.DefaultFleetControllerConfigMapName)
	if err != nil {
		return setting, fmt.Errorf("error fetching fleet-controller configmap: %v", err)
	}

	if setting.Name == util.RancherInternalServerURLSetting && len(setting.Value) != 0 {
		return h.reconcileInternalServerURLSetting(setting, fleetControllerConfig)
	}

	if setting.Name == util.RancherInternalCASetting && len(setting.Value) != 0 {
		return h.reconcileInternalCASetting(setting, fleetControllerConfig)
	}

	return setting, nil
}

func (h *mcmSettingsHandler) reconcileInternalCASetting(setting *mgmtv3.Setting, fleetControllerConfig *corev1.ConfigMap) (*mgmtv3.Setting, error) {

	encodedCA := base64.StdEncoding.EncodeToString([]byte(setting.Value))
	currentVal, err := getConfigMayKey(fleetControllerConfig, util.APIServerCAKey)
	if err != nil {
		return setting, err
	}

	var patchedConfigMap *corev1.ConfigMap
	if currentVal != encodedCA {
		logrus.Infof("patch fleet-controller configmap %v from %v to %v", util.APIServerCAKey, currentVal, encodedCA)
		patchedConfigMap, err = patchConfigMap(fleetControllerConfig, util.APIServerCAKey, encodedCA)
		if err != nil {
			return setting, err
		}

		_, err = h.configMapClient.Update(patchedConfigMap)
		return setting, err
	}

	return setting, err
}

func (h *mcmSettingsHandler) reconcileInternalServerURLSetting(setting *mgmtv3.Setting, fleetControllerConfig *corev1.ConfigMap) (*mgmtv3.Setting, error) {
	apiServerURL := setting.Value
	currentVal, err := getConfigMayKey(fleetControllerConfig, util.APIServerURLKey)
	if err != nil {
		return setting, err
	}

	var patchedConfigMap *corev1.ConfigMap
	if currentVal != apiServerURL {
		logrus.Infof("patch fleet-controller configmap %v from %v to %v", util.APIServerURLKey, currentVal, apiServerURL)
		patchedConfigMap, err = patchConfigMap(fleetControllerConfig, util.APIServerURLKey, apiServerURL)
		if err != nil {
			return setting, err
		}

		_, err = h.configMapClient.Update(patchedConfigMap)
		return setting, err
	}

	return setting, nil
}

func patchConfigMap(cm *corev1.ConfigMap, key string, value string) (*corev1.ConfigMap, error) {
	stringConfig := cm.Data["config"]
	parsedVals := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(stringConfig), &parsedVals); err != nil {
		return cm, fmt.Errorf("error unmarshalling cm config: %v", err)
	}

	parsedVals[key] = value
	newConfig, err := yaml.Marshal(parsedVals)
	if err != nil {
		return cm, err
	}
	cm.Data["config"] = string(newConfig)
	return cm, nil
}

func getConfigMayKey(cm *corev1.ConfigMap, key string) (string, error) {
	stringConfig := cm.Data["config"]
	parsedVals := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(stringConfig), &parsedVals); err != nil {
		return "", fmt.Errorf("error unmarshalling cm config: %v", err)
	}

	val, ok := parsedVals[key]
	if !ok {
		return "", nil
	}

	stringVal, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("unable to assert val %v for key %s to string", val, key)
	}

	return stringVal, nil
}
