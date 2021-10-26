package setting

import (
	"github.com/sirupsen/logrus"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

// setLogLevel updates the log level on setting changes
func (h *Handler) setLogLevel(setting *harvesterv1.Setting) error {
	level, err := logrus.ParseLevel(setting.Value)
	if err != nil {
		return err
	}

	logrus.Infof("set log level to %s", level)
	logrus.SetLevel(level)
	return nil
}
