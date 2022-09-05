package setting

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"

	ctlhelmv1 "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	catalogv1api "github.com/rancher/rancher/pkg/apis/catalog.cattle.io/v1"
	catalogv1 "github.com/rancher/rancher/pkg/generated/controllers/catalog.cattle.io/v1"
	mgmtv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	provisioningv1 "github.com/rancher/rancher/pkg/generated/controllers/provisioning.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/apply"
	v1 "github.com/rancher/wrangler/pkg/generated/controllers/apps/v1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/slice"
	"k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
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
	secrets              ctlcorev1.SecretClient
	secretCache          ctlcorev1.SecretCache
	deployments          v1.DeploymentClient
	deploymentCache      v1.DeploymentCache
	ingresses            networkingv1.IngressClient
	ingressCache         networkingv1.IngressCache
	longhornSettings     ctllonghornv1.SettingClient
	longhornSettingCache ctllonghornv1.SettingCache
	configmaps           ctlcorev1.ConfigMapClient
	configmapCache       ctlcorev1.ConfigMapCache
	apps                 catalogv1.AppClient
	managedCharts        mgmtv3.ManagedChartClient
	managedChartCache    mgmtv3.ManagedChartCache
	helmChartConfigs     ctlhelmv1.HelmChartConfigClient
	helmChartConfigCache ctlhelmv1.HelmChartConfigCache
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
	hash := sha256.New224()
	hash.Write([]byte(setting.Value))
	currentHash := fmt.Sprintf("%x", hash.Sum(nil))
	if currentHash == setting.Annotations[util.AnnotationHash] {
		return nil, nil
	}

	toUpdate := setting.DeepCopy()
	if toUpdate.Annotations == nil {
		toUpdate.Annotations = make(map[string]string)
	}
	toUpdate.Annotations[util.AnnotationHash] = currentHash

	var err error
	if syncer, ok := syncers[setting.Name]; ok {
		err = syncer(setting)
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

func (h *Handler) appOnChanged(_ string, app *catalogv1api.App) (*catalogv1api.App, error) {
	if app == nil || app.DeletionTimestamp != nil {
		return nil, nil
	}

	harvesterManagedChart, err := h.managedChartCache.Get(ManagedChartNamespace, HarvesterManagedChartName)
	if err != nil {
		return nil, err
	}

	if app.Namespace != harvesterManagedChart.Spec.DefaultNamespace || app.Name != harvesterManagedChart.Spec.ReleaseName {
		return nil, nil
	}

	return nil, UpdateSupportBundleImage(h.settings, h.settingCache, app)
}
