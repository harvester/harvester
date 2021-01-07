package upgrade

import (
	"github.com/sirupsen/logrus"

	apisv1alpha1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
)

// settingHandler do version syncs on server-version setting changes
type settingHandler struct {
	versionSyncer *versionSyncer
}

func (h *settingHandler) OnChanged(key string, setting *apisv1alpha1.Setting) (*apisv1alpha1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil || setting.Name != "server-version" {
		return setting, nil
	}
	if err := h.versionSyncer.sync(); err != nil {
		logrus.Errorf("failed syncing version metadata: %v", err)
	}
	return setting, nil
}
