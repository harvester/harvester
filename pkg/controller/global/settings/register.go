package settings

import (
	"context"
	"fmt"
	"os"

	"github.com/rancher/steve/pkg/server"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
)

func Register(ctx context.Context, scaled *config.Scaled, _ *server.Server, _ config.Options) error {
	sp := &settingsProvider{
		context:        ctx,
		settings:       scaled.HarvesterFactory.Harvesterhci().V1beta1().Setting(),
		settingsLister: scaled.HarvesterFactory.Harvesterhci().V1beta1().Setting().Cache(),
		fallback:       map[string]string{},
	}

	return settings.SetProvider(sp)
}

type settingsProvider struct {
	context        context.Context
	settings       ctlharvesterv1.SettingClient
	settingsLister ctlharvesterv1.SettingCache
	fallback       map[string]string
}

func (s *settingsProvider) Get(name string) string {
	logrus.Debugf("Fetching setting: %s", name)

	value := os.Getenv(settings.GetEnvKey(name))
	if value != "" {
		logrus.Debugf("Setting %s found in environment variable with value: %s", name, value)
		return value
	}

	logrus.Debugf("Attempting to fetch setting %s from cache", name)
	obj, err := s.settingsLister.Get(name)
	if err != nil {
		logrus.Warnf("Failed to fetch setting %s from cache, attempting direct API call: %v", name, err)
		val, err := s.settings.Get(name, v1.GetOptions{})
		if err != nil {
			logrus.Errorf("Failed to fetch setting %s from API, falling back to default: %v", name, err)
			return s.fallback[name]
		}
		obj = val
	}

	if obj.Value == "" {
		logrus.Debugf("Setting %s has no value, using default: %s", name, obj.Default)
		return obj.Default
	}

	logrus.Debugf("Setting %s found with value: %s", name, obj.Value)
	return obj.Value
}

func (s *settingsProvider) Set(name, value string) error {
	envValue := os.Getenv(settings.GetEnvKey(name))
	if envValue != "" {
		return fmt.Errorf("setting %s can not be set because it is from environment variable", name)
	}
	obj, err := s.settings.Get(name, v1.GetOptions{})
	if err != nil {
		return err
	}

	obj.Value = value
	_, err = s.settings.Update(obj)
	return err
}

func (s *settingsProvider) SetIfUnset(name, value string) error {
	obj, err := s.settings.Get(name, v1.GetOptions{})
	if err != nil {
		return err
	}

	if obj.Value != "" {
		return nil
	}

	obj.Value = value
	_, err = s.settings.Update(obj)
	return err
}

func (s *settingsProvider) SetAll(settingsMap map[string]settings.Setting) error {
	fallback := map[string]string{}

	for name, setting := range settingsMap {
		key := settings.GetEnvKey(name)
		value := os.Getenv(key)
		defaultvaluekey := settings.GetEnvDefaultValueKey(name)
		defaultvalue := os.Getenv(defaultvaluekey)

		// override settings from ENV first
		if defaultvalue != "" && defaultvalue != setting.Default {
			logrus.WithFields(logrus.Fields{
				"name": name,
			}).Debugf("setting default %s is replaced with %s", setting.Default, defaultvalue)
			setting.Default = defaultvalue
		}

		obj, err := s.settings.Get(setting.Name, v1.GetOptions{})
		if errors.IsNotFound(err) {
			newSetting := &harvesterv1.Setting{
				ObjectMeta: v1.ObjectMeta{
					Name: setting.Name,
				},
				Default: setting.Default,
			}
			if value != "" {
				newSetting.Value = value
			}
			if newSetting.Value == "" {
				fallback[newSetting.Name] = newSetting.Default
			} else {
				fallback[newSetting.Name] = newSetting.Value
			}

			_, err := s.settings.Create(newSetting)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			update := false
			if obj.Default != setting.Default {
				obj.Default = setting.Default
				update = true
			}
			if value != "" && obj.Value != value {
				obj.Value = value
				update = true
			}
			if obj.Value == "" {
				fallback[obj.Name] = obj.Default
			} else {
				fallback[obj.Name] = obj.Value
			}
			if update {
				_, err := s.settings.Update(obj)
				if err != nil {
					return err
				}
			}
		}
	}

	s.fallback = fallback

	return nil
}
