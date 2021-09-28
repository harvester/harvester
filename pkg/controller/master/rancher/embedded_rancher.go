package rancher

import (
	"fmt"
	"strings"
	"time"

	rancherv3api "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	ranchersettings "github.com/rancher/rancher/pkg/settings"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"
)

var UpdateRancherUISettings = map[string]string{
	"ui-pl": "Harvester",
}

func (h *Handler) RancherSettingOnChange(key string, setting *rancherv3api.Setting) (*rancherv3api.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil {
		return nil, nil
	}

	if setting.Name == caCertsSetting && setting.Default == "" {
		return nil, h.initializeTLS(setting)
	}
	if setting.Name == serverURLSetting && setting.Value == "" {
		return h.initializeServerURL(setting)
	}

	if setting.Name == systemNamespacesSetting && !strings.Contains(setting.Default, "harvester-system") {
		return h.initializeSystemNamespaces(setting)
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

func (h *Handler) initializeServerURL(setting *rancherv3api.Setting) (*rancherv3api.Setting, error) {
	vipConfig, err := h.getVipConfig()
	if err != nil {
		return nil, err
	}
	toUpdate := setting.DeepCopy()
	toUpdate.Value = fmt.Sprintf("https://%s", vipConfig.IP)
	logrus.Debugf("Updating server-url setting to %s", toUpdate.Value)
	return h.RancherSettings.Update(toUpdate)
}

func (h *Handler) initializeSystemNamespaces(setting *rancherv3api.Setting) (*rancherv3api.Setting, error) {
	sets := labels.Set{
		defaultAdminLabelKey: defaultAdminLabelValue,
	}
	users, err := h.RancherUserCache.List(sets.AsSelector())
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		// The default admin and its namespace is not created, enqueue and check later
		h.RancherSettingController.EnqueueAfter(setting.Name, 30*time.Second)
		return nil, nil
	}
	defaultAdminNamespace := users[0].Name
	harvesterDefaultSystemNamespaces := strings.Join([]string{
		ranchersettings.SystemNamespaces.Default,
		"cattle-dashboards",
		"cattle-fleet-clusters-system",
		"cattle-monitoring-system",
		"fleet-local",
		"harvester-system",
		"local",
		"longhorn-system",
		defaultAdminNamespace,
	}, ",")
	logrus.Debugf("Updating system-namespaces setting to %s", harvesterDefaultSystemNamespaces)
	toUpdate := setting.DeepCopy()
	toUpdate.Default = harvesterDefaultSystemNamespaces
	return h.RancherSettings.Update(toUpdate)
}

// initializeTLS writes internal-cacerts to cacerts value and adds the VIP to certificate SANs
// cacerts is used in generated kubeconfig files
func (h *Handler) initializeTLS(setting *rancherv3api.Setting) error {
	internalCACerts, err := h.RancherSettingCache.Get(internalCACertsSetting)
	if err != nil {
		return err
	}
	if internalCACerts.Value == "" {
		// Not initialized, enqueue
		h.RancherSettingController.EnqueueAfter(caCertsSetting, 30*time.Second)
		return nil
	}

	caCertsCopy := setting.DeepCopy()
	caCertsCopy.Default = internalCACerts.Value
	if _, err = h.RancherSettings.Update(caCertsCopy); err != nil {
		return err
	}

	if err := h.addVIPToSAN(); err != nil {
		return err
	}
	return nil
}

// addVIPToSAN writes VIP to TLS SAN of the serving cert
func (h *Handler) addVIPToSAN() error {
	vipConfig, err := h.getVipConfig()
	if err != nil {
		return err
	}
	secret, err := h.SecretCache.Get(cattleSystemNamespaceName, tlsCertName)
	if err != nil {
		return err
	}

	toAddAnnotation := tlsCNPrefix + vipConfig.IP
	if _, ok := secret.Annotations[toAddAnnotation]; ok {
		return nil
	}

	toUpdate := secret.DeepCopy()
	if toUpdate.Annotations == nil {
		toUpdate.Annotations = make(map[string]string)
	}
	//clean up other cns
	newAnnotations := map[string]string{}
	for k, v := range toUpdate.Annotations {
		if !strings.Contains(k, tlsCNPrefix) {
			newAnnotations[k] = v
		}
	}
	toUpdate.Annotations = newAnnotations
	toUpdate.Annotations[toAddAnnotation] = vipConfig.IP
	_, err = h.Secrets.Update(toUpdate)
	return err
}
