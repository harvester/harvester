package setting

import (
	"encoding/json"
	"reflect"

	"github.com/rancher/wharfie/pkg/registries"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

func (h *Handler) syncContainerdRegistry(setting *harvesterv1.Setting) error {
	if setting.Value == "" {
		return h.clearContainerdRegistrySecret()
	}

	registryFromSetting := &registries.Registry{}
	if err := json.Unmarshal([]byte(setting.Value), registryFromSetting); err != nil {
		return err
	}

	registryFromSettingYaml, err := yaml.Marshal(registryFromSetting)
	if err != nil {
		return err
	}

	secret, err := h.secretCache.Get(util.CattleSystemNamespaceName, util.ContainerdRegistrySecretName)
	if apierrors.IsNotFound(err) {
		newSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      util.ContainerdRegistrySecretName,
				Namespace: util.CattleSystemNamespaceName,
			},
			Data: map[string][]byte{
				util.ContainerdRegistryFileName: registryFromSettingYaml,
			},
		}

		if _, err := h.secrets.Create(newSecret); err != nil {
			return err
		}
		return h.removeCredentialInSetting(setting, registryFromSetting)
	} else if err != nil {
		return err
	}

	isStale, err := h.isStaleSecret(secret, registryFromSetting)
	if err != nil {
		return err
	}

	if isStale {
		toUpdate := secret.DeepCopy()
		toUpdate.Data = map[string][]byte{
			util.ContainerdRegistryFileName: registryFromSettingYaml,
		}
		if _, err := h.secrets.Update(toUpdate); err != nil {
			return err
		}
	}

	if err = h.removeCredentialInSetting(setting, registryFromSetting); err != nil {
		return err
	}

	return nil
}

func (h *Handler) clearContainerdRegistrySecret() error {
	secret, err := h.secretCache.Get(util.CattleSystemNamespaceName, util.ContainerdRegistrySecretName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	toUpdate := secret.DeepCopy()
	toUpdate.Data = map[string][]byte{
		util.ContainerdRegistryFileName: []byte(""),
	}
	if !reflect.DeepEqual(secret, toUpdate) {
		if _, err := h.secrets.Update(toUpdate); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) isStaleSecret(secret *corev1.Secret, registryFromSetting *registries.Registry) (bool, error) {
	isStale := secret.Data == nil || string(secret.Data[util.ContainerdRegistryFileName]) == ""
	if !isStale {
		registryFromSecret := &registries.Registry{}
		if err := yaml.Unmarshal(secret.Data[util.ContainerdRegistryFileName], registryFromSecret); err != nil {
			return true, err
		}
		// We only compare mirros, because we will remove configs.
		isStale = isStale || !isSameMirrors(registryFromSetting.Mirrors, registryFromSecret.Mirrors)
	}

	return isStale, nil
}

func (h *Handler) removeCredentialInSetting(setting *harvesterv1.Setting, registry *registries.Registry) error {
	// Remove credential in configs.
	for configName, config := range registry.Configs {
		config.Auth = nil
		registry.Configs[configName] = config
	}

	registryJSON, err := json.Marshal(registry)
	if err != nil {
		return err
	}

	toUpdate := setting.DeepCopy()
	toUpdate.Value = string(registryJSON)
	if !reflect.DeepEqual(setting, toUpdate) {
		if _, err = h.settings.Update(toUpdate); err != nil {
			return err
		}
	}
	return nil
}

func isSameMirrors(fromSetting, fromSecret map[string]registries.Mirror) bool {
	if len(fromSetting) == 0 && len(fromSecret) == 0 {
		return true
	}

	if len(fromSetting) == 0 || len(fromSecret) == 0 {
		return false
	}

	// For map comparison in reflect.DeepEqual, it only same if both are nil or both content are same.
	for mirrorName, mirror := range fromSetting {
		if len(mirror.Rewrites) == 0 && mirror.Rewrites != nil {
			mirror.Rewrites = nil
			fromSetting[mirrorName] = mirror
		}
	}
	for mirrorName, mirror := range fromSecret {
		if len(mirror.Rewrites) == 0 && mirror.Rewrites != nil {
			mirror.Rewrites = nil
			fromSecret[mirrorName] = mirror
		}
	}
	return reflect.DeepEqual(fromSetting, fromSecret)
}
