package manager

import (
	"github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/longhorn/longhorn-manager/types"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func (m *VolumeManager) GetSettingValueExisted(sName types.SettingName) (string, error) {
	return m.ds.GetSettingValueExisted(sName)
}

func (m *VolumeManager) GetSetting(sName types.SettingName) (*longhorn.Setting, error) {
	return m.ds.GetSetting(sName)
}

func (m *VolumeManager) ListSettings() (map[types.SettingName]*longhorn.Setting, error) {
	return m.ds.ListSettings()
}

func (m *VolumeManager) ListSettingsSorted() ([]*longhorn.Setting, error) {
	settingMap, err := m.ListSettings()
	if err != nil {
		return []*longhorn.Setting{}, err
	}

	settings := make([]*longhorn.Setting, len(settingMap))
	settingNames, err := sortKeys(settingMap)
	if err != nil {
		return []*longhorn.Setting{}, err
	}
	for i, settingName := range settingNames {
		settings[i] = settingMap[types.SettingName(settingName)]
	}
	return settings, nil
}

func (m *VolumeManager) CreateOrUpdateSetting(s *longhorn.Setting) (*longhorn.Setting, error) {
	err := m.ds.ValidateSetting(s.Name, s.Value)
	if err != nil {
		return nil, err
	}
	setting, err := m.ds.UpdateSetting(s)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return m.ds.CreateSetting(s)
		}
		return nil, err
	}
	logrus.Debugf("Updated setting %v to %v", s.Name, setting.Value)
	return setting, nil
}
