package rancher

import (
	"encoding/json"
	"strings"
	"time"

	rancherv3api "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	ranchersettings "github.com/rancher/rancher/pkg/settings"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/harvester/harvester/pkg/settings"
)

var UpdateRancherUISettings = map[string]string{
	"ui-pl": "Harvester",
}

func (h *Handler) RancherSettingOnChange(key string, setting *rancherv3api.Setting) (*rancherv3api.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil {
		return nil, nil
	}

	if setting.Name == caCertsSetting && setting.Default == "" {
		return nil, h.syncCACert(setting)
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

	caCertsCopy := setting.DeepCopy()
	caCertsCopy.Default = cacert
	_, err := h.RancherSettings.Update(caCertsCopy)
	return err
}
