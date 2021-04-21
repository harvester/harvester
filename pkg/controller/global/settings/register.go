package settings

import (
	"context"
	"fmt"
	"os"

	"github.com/rancher/steve/pkg/server"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
)

func Register(ctx context.Context, scaled *config.Scaled, server *server.Server, options config.Options) error {
	sp := &settingsProvider{
		context:        ctx,
		settings:       scaled.HarvesterFactory.Harvesterhci().V1beta1().Setting(),
		settingsLister: scaled.HarvesterFactory.Harvesterhci().V1beta1().Setting().Cache(),
		fallback:       map[string]string{},
	}

	if err := settings.SetProvider(sp); err != nil {
		return err
	}

	return nil
}

type settingsProvider struct {
	context        context.Context
	settings       ctlharvesterv1.SettingClient
	settingsLister ctlharvesterv1.SettingCache
	fallback       map[string]string
}

func (s *settingsProvider) Get(name string) string {
	value := os.Getenv(settings.GetEnvKey(name))
	if value != "" {
		return value
	}
	obj, err := s.settingsLister.Get(name)
	if err != nil {
		val, err := s.settings.Get(name, v1.GetOptions{})
		if err != nil {
			return s.fallback[name]
		}
		obj = val
	}
	if obj.Value == "" {
		return obj.Default
	}
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
