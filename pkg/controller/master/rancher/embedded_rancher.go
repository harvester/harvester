package rancher

import (
	"encoding/json"
	"reflect"
	"strings"

	rancherv3api "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/harvester/harvester/pkg/settings"
)

var UpdateRancherUISettings = map[string]string{
	"ui-pl":    "Harvester",
	"ui-brand": "harvester",
}

func (h *Handler) RancherSettingOnChange(_ string, setting *rancherv3api.Setting) (*rancherv3api.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil {
		return nil, nil
	}

	if setting.Name == caCertsSetting && setting.Default == "" {
		return nil, h.syncCACert(setting)
	}

	for name, value := range UpdateRancherUISettings {
		if setting.Name == name && setting.Default != value {
			logrus.Debugf("Updating rancher dashboard setting %s, %s => %s", name, setting.Default, value)
			settCopy := setting.DeepCopy()
			settCopy.Default = value
			if _, err := h.RancherSettings.Update(settCopy); err != nil {
				return setting, err
			}
		}
	}
	return nil, nil
}

// syncCACert updates the cacerts setting to ca certificate of Harvester setting
// or internal-cacerts when it is not specified.
func (h *Handler) syncCACert(setting *rancherv3api.Setting) error {
	var cacert string
	sslCertificate := &settings.SSLCertificate{}
	if err := json.Unmarshal([]byte(settings.SSLCertificates.Get()), sslCertificate); err != nil {
		return err
	}
	if sslCertificate.PublicCertificate != "" && sslCertificate.PrivateKey != "" {
		cacert = sslCertificate.CA
	} else {
		internalCACerts, err := h.RancherSettingCache.Get(internalCACertsSetting)
		if err != nil {
			return err
		}
		cacert = internalCACerts.Value
	}
	// Trim spaces to avoid checksum error in rancher.
	// Ref: https://github.com/rancher/rancher/blob/a841a9e101ae601a033fa8b39342ad4800c91d85/package/run.sh#L243
	cacert = strings.TrimSpace(cacert)

	caCertsCopy := setting.DeepCopy()
	caCertsCopy.Default = cacert
	_, err := h.RancherSettings.Update(caCertsCopy)
	return err
}

// PatchCAPIDeployment is used to patch env variables for capi-deployment created by rancher.
// This ensures that capi-controller reconciles the local rancher cluster using the kubeconfig in the local-kubeconfig
// secret in fleet-local namespace. Without this patch, capi-controller identifies itself as running on the workload
// cluster and patches the config, to use in-cluster api endpoint (kubernetes default) service, which does not recognize
// the rancher generated token in the local-kubeconfig secret
func (h *Handler) PatchCAPIDeployment(_ string, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	if deployment == nil || !deployment.DeletionTimestamp.IsZero() {
		return deployment, nil
	}

	if !isCapiDeployment(deployment) {
		return deployment, nil
	}

	deploymentObjCopy := deployment.DeepCopy()
	for i := range deploymentObjCopy.Spec.Template.Spec.Containers {
		if len(deploymentObjCopy.Spec.Template.Spec.Containers[i].Env) > 0 {
			deploymentObjCopy.Spec.Template.Spec.Containers[i].Env = []corev1.EnvVar{}
		}
	}

	if !reflect.DeepEqual(deploymentObjCopy.Spec.Template.Spec.Containers, deployment.Spec.Template.Spec.Containers) {
		return h.Deployments.Update(deploymentObjCopy)
	}

	return deployment, nil
}

func isCapiDeployment(deployment *appsv1.Deployment) bool {
	if deployment.Name == capiControllerDeploymentName && deployment.Namespace == capiControllerDeploymentNamespace {
		return true
	}
	return false
}
