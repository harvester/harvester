package setting

import (
	"github.com/sirupsen/logrus"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

// Handler updates the log level on setting changes
type Handler struct {
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
