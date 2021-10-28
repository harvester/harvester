package setting

import (
	appsv1 "github.com/rancher/wrangler/pkg/generated/controllers/apps/v1"
	corev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvctlv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	systemSetting "github.com/harvester/harvester/pkg/settings"
)

const (
	harvesterSystemNamespace = "harvester-system"
	installerSettingsCmName  = "installer-provided-settings"
)

// Handler updates the log level on setting changes
type Handler struct {
	dsClient          appsv1.DaemonSetClient
	dsCache           appsv1.DaemonSetCache
	cmClient          corev1.ConfigMapClient
	cmCache           corev1.ConfigMapCache
	settingController harvctlv1beta1.SettingController
	settingCache      harvctlv1beta1.SettingCache
}

func (h *Handler) LogLevelOnChanged(key string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil || setting.Name != "log-level" || setting.Value == "" {
		return setting, nil
	}

	level, err := logrus.ParseLevel(setting.Value)
	if err != nil {
		return setting, err
	}

	logrus.Infof("set log level to %s", level)
	logrus.SetLevel(level)
	return setting, nil
}

func (h *Handler) AutoAddDiskPathsOnChanged(key string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil || setting.Name != systemSetting.AutoAddDiskPaths.Name {
		return setting, nil
	}

	logrus.Debug("AudoAddDiskPathOnChanged")
	ds, err := h.dsCache.Get(harvesterSystemNamespace, "harvester-node-disk-manager")
	if err != nil {
		return setting, nil
	}

	dsCopy := ds.DeepCopy()

	for i, cont := range dsCopy.Spec.Template.Spec.Containers {
		if cont.Name == "harvester-node-disk-manager" {
			for j, env := range cont.Env {
				if env.Name == "NDM_AUTO_ADD_PATH" {
					logrus.Infof("update NDM_AUTO_ADD_PATH to '%s'", setting.Value)
					dsCopy.Spec.Template.Spec.Containers[i].Env[j].Value = setting.Value
				}
			}
		}
	}

	logrus.Info(dsCopy.Spec.Template.Spec.Containers)

	_, err = h.dsClient.Update(dsCopy)
	if err != nil {
		logrus.Errorf("unable to update NDM daemonset: %v", err)
		return setting, err
	}

	return nil, nil
}

func (h *Handler) LoadSettingsFromInstallerOnChanged(key string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil || setting.Name != systemSetting.LoadSettingsFromInstaller.Name {
		return setting, nil
	}

	settingCopy := setting.DeepCopy()

	var loadFromInstaller string
	if settingCopy.Value == "" {
		loadFromInstaller = settingCopy.Default
	} else {
		loadFromInstaller = settingCopy.Value
	}

	if loadFromInstaller == "true" {
		logrus.Info("load settings provided by installer")
		cm, err := h.cmCache.Get(harvesterSystemNamespace, installerSettingsCmName)
		if err != nil {
			logrus.Error("failed to get installer settings ConfigMap:", err)
			return nil, err
		}

		for settingName, settingValue := range cm.Data {
			logrus.Infof("overwrite setting '%s' with '%s'", settingName, settingValue)
			if err := h.updateSetting(settingName, settingValue); err != nil {
				if errors.IsNotFound(err) {
					logrus.Warningf("no such setting '%s', skip update", settingName)
				} else {
					logrus.Errorf("failed to update setting '%s': '%s'", settingName, err)
				}
			}
		}

		logrus.Info("installer settings loaded, disable loading")
		settingCopy.Value = "false"
		if _, err := h.settingController.Update(settingCopy); err != nil {
			logrus.Error("failed to disable loading:", err)
		}
	}

	return nil, nil
}

func (h *Handler) updateSetting(name, value string) error {
	settingToUpdate, err := h.settingCache.Get(name)
	if err != nil {
		return err
	}

	settingToUpdate = settingToUpdate.DeepCopy()
	settingToUpdate.Value = value
	if _, err := h.settingController.Update(settingToUpdate); err != nil {
		return err
	}

	return nil
}
