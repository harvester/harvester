package setting

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	ctlnodev1 "github.com/harvester/node-manager/pkg/generated/controllers/node.harvesterhci.io/v1beta1"
	ctlhelmv1 "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	ctlmgmtv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	provisioningv1 "github.com/rancher/rancher/pkg/generated/controllers/provisioning.cattle.io/v1"
	ctlrkev1 "github.com/rancher/rancher/pkg/generated/controllers/rke.cattle.io/v1"
	"github.com/rancher/wrangler/v3/pkg/apply"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/apps/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/slice"
	"k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	kubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	networkingv1 "github.com/harvester/harvester/pkg/generated/controllers/networking.k8s.io/v1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

type syncerFunc func(*harvesterv1.Setting) error

var (
	syncers map[string]syncerFunc
	// bootstrapSettings are the setting that syncs on bootstrap
	bootstrapSettings = []string{
		settings.SSLCertificatesSettingName,
		settings.KubeconfigDefaultTokenTTLMinutesSettingName,
		// The Longhorn storage over-provisioning percentage is set to 100, whereas Harvester uses 200.
		// This needs to be synchronized when Harvester starts.
		settings.OvercommitConfigSettingName,
		// always run this when Harvester POD starts
		settings.AdditionalGuestMemoryOverheadRatioName,
	}
	skipHashCheckSettings = []string{
		settings.AutoRotateRKE2CertsSettingName,
		settings.LogLevelSettingName,
		settings.KubeconfigDefaultTokenTTLMinutesSettingName,
		settings.AdditionalGuestMemoryOverheadRatioName,
	}
)

type Handler struct {
	namespace            string
	httpClient           http.Client
	apply                apply.Apply
	clusterCache         provisioningv1.ClusterCache
	clusters             provisioningv1.ClusterClient
	settings             v1beta1.SettingClient
	settingCache         v1beta1.SettingCache
	settingController    v1beta1.SettingController
	secrets              ctlcorev1.SecretClient
	secretCache          ctlcorev1.SecretCache
	deployments          v1.DeploymentClient
	deploymentCache      v1.DeploymentCache
	ingresses            networkingv1.IngressClient
	ingressCache         networkingv1.IngressCache
	longhornSettings     ctllhv1.SettingClient
	longhornSettingCache ctllhv1.SettingCache
	configmaps           ctlcorev1.ConfigMapClient
	configmapCache       ctlcorev1.ConfigMapCache
	endpointCache        ctlcorev1.EndpointsCache
	managedCharts        ctlmgmtv3.ManagedChartClient
	managedChartCache    ctlmgmtv3.ManagedChartCache
	helmChartConfigs     ctlhelmv1.HelmChartConfigClient
	helmChartConfigCache ctlhelmv1.HelmChartConfigCache
	nodeClient           ctlcorev1.NodeController
	nodeCache            ctlcorev1.NodeCache
	nodeConfigs          ctlnodev1.NodeConfigClient
	nodeConfigsCache     ctlnodev1.NodeConfigCache
	rkeControlPlaneCache ctlrkev1.RKEControlPlaneCache
	rancherSettings      ctlmgmtv3.SettingClient
	rancherSettingsCache ctlmgmtv3.SettingCache
	kubeVirtConfig       kubevirtv1.KubeVirtClient
	kubeVirtConfigCache  kubevirtv1.KubeVirtCache
}

func (h *Handler) settingOnChanged(_ string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil {
		return nil, nil
	}

	// The setting value hash is stored in the annotation when a setting syncer completes.
	// So that we only proceed when value is changed.
	if setting.Value == "" && setting.Annotations[util.AnnotationHash] == "" &&
		!slice.ContainsString(bootstrapSettings, setting.Name) {
		return nil, nil
	}

	toMeasure := io.MultiReader(
		strings.NewReader(setting.Value),
		strings.NewReader(setting.Annotations[util.AnnotationUpgradePatched]),
	)

	hash := sha256.New224()
	if _, err := io.Copy(hash, toMeasure); err != nil {
		return nil, fmt.Errorf("failed to calculate hash for setting %s: %w", setting.Name, err)
	}
	currentHash := fmt.Sprintf("%x", hash.Sum(nil))
	if !slice.ContainsString(skipHashCheckSettings, setting.Name) && currentHash == setting.Annotations[util.AnnotationHash] {
		return nil, nil
	}

	toUpdate := setting.DeepCopy()
	if toUpdate.Annotations == nil {
		toUpdate.Annotations = make(map[string]string)
	}

	var err error
	if syncer, ok := syncers[setting.Name]; ok {
		err = syncer(setting)
		if err == nil {
			toUpdate.Annotations[util.AnnotationHash] = currentHash
		}
		if updateErr := h.setConfiguredCondition(toUpdate, err); updateErr != nil {
			return setting, updateErr
		}
	}

	return setting, err
}

func (h *Handler) setConfiguredCondition(settingCopy *harvesterv1.Setting, err error) error {
	if err != nil && (!harvesterv1.SettingConfigured.IsFalse(settingCopy) ||
		harvesterv1.SettingConfigured.GetMessage(settingCopy) != err.Error()) {
		harvesterv1.SettingConfigured.False(settingCopy)
		harvesterv1.SettingConfigured.Message(settingCopy, err.Error())
		if _, err := h.settings.Update(settingCopy); err != nil {
			return err
		}
	} else if err == nil {
		if settingCopy.Value == "" {
			harvesterv1.SettingConfigured.False(settingCopy)
		} else {
			harvesterv1.SettingConfigured.True(settingCopy)
		}
		harvesterv1.SettingConfigured.Message(settingCopy, "")
		if _, err := h.settings.Update(settingCopy); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) updateBackupSecret(data map[string]string) error {
	secret, err := h.secretCache.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	toUpdate := secret.DeepCopy()
	if toUpdate.Data == nil {
		toUpdate.Data = make(map[string][]byte)
	}
	for key, value := range data {
		toUpdate.Data[key] = []byte(value)
	}
	_, err = h.secrets.Update(toUpdate)
	return err
}

func (h *Handler) redeployDeployment(namespace, name string) error {
	deployment, err := h.deploymentCache.Get(namespace, name)
	if err != nil {
		return err
	}
	toUpdate := deployment.DeepCopy()
	if deployment.Spec.Template.Annotations == nil {
		toUpdate.Spec.Template.Annotations = make(map[string]string)
	}
	toUpdate.Spec.Template.Annotations[util.AnnotationTimestamp] = time.Now().Format(time.RFC3339)

	_, err = h.deployments.Update(toUpdate)
	return err
}
