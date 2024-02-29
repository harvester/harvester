package setting

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	provisioningv1 "github.com/rancher/rancher/pkg/apis/provisioning.cattle.io/v1"
	rkev1 "github.com/rancher/rancher/pkg/apis/rke.cattle.io/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/containerd"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	containerdRegistrySecretsNamespace = util.FleetLocalNamespaceName
	containerdRegistrySecretDataHost   = "host"
)

func (h *Handler) syncContainerdRegistry(setting *harvesterv1.Setting) error {
	if setting.Value == "" {
		if err := h.clearContainerdRegistrySecret(); err != nil {
			return err
		}
		return h.copyContainerdRegistryToRancher(nil)
	}

	registryFromSetting := &containerd.Registry{}
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

		if err := h.copyContainerdRegistryToRancher(registryFromSetting); err != nil {
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
		if secret, err = h.secrets.Update(toUpdate); err != nil {
			return err
		}
	}

	if err = h.syncContainerdRegistrySecretToRancher(secret); err != nil {
		return err
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

func (h *Handler) isStaleSecret(secret *corev1.Secret, registryFromSetting *containerd.Registry) (bool, error) {
	isStale := secret.Data == nil || string(secret.Data[util.ContainerdRegistryFileName]) == ""
	if !isStale {
		registryFromSecret := &containerd.Registry{}
		if err := yaml.Unmarshal(secret.Data[util.ContainerdRegistryFileName], registryFromSecret); err != nil {
			return true, err
		}
		// We only compare mirros, because we will remove configs.
		isStale = isStale || !isSameMirrors(registryFromSetting.Mirrors, registryFromSecret.Mirrors)
	}

	return isStale, nil
}

func (h *Handler) removeCredentialInSetting(setting *harvesterv1.Setting, registry *containerd.Registry) error {
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

func isSameMirrors(fromSetting, fromSecret map[string]containerd.Mirror) bool {
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

func (h *Handler) syncContainerdRegistrySecretToRancher(secret *corev1.Secret) error {
	var registry containerd.Registry
	if err := yaml.Unmarshal(secret.Data[util.ContainerdRegistryFileName], &registry); err != nil {
		return err
	}
	return h.copyContainerdRegistryToRancher(&registry)
}

func (h *Handler) copyContainerdRegistryToRancher(newRegistries *containerd.Registry) error {
	ll := logrus.WithFields(logrus.Fields{
		"containerd_registry_sync_namespace": containerdRegistrySecretsNamespace,
	})

	ll.Debug("Start containerd-registry sync to Rancher")

	prevConfSecrets, err := h.containerdRegistryConfigSecrets()
	if err != nil {
		return err
	}

	newConfSecrets := make(map[string]*corev1.Secret)

	var (
		empty           containerd.Registry
		clusterRegistry *rkev1.Registry
	)

	if newRegistries != nil && !reflect.DeepEqual(newRegistries, empty) {
		clusterRegistry = new(rkev1.Registry)

		if len(newRegistries.Mirrors) > 0 {
			clusterRegistry.Mirrors = make(map[string]rkev1.Mirror)
		}

		for name, mirror := range newRegistries.Mirrors {
			clusterRegistry.Mirrors[name] = rkev1.Mirror{
				Endpoints: mirror.Endpoints,
				Rewrites:  mirror.Rewrites,
			}
		}

		if len(newRegistries.Configs) > 0 {
			clusterRegistry.Configs = make(map[string]rkev1.RegistryConfig)
		} else {
			ll.Debug("No configs in registry setting")
		}

		for host, config := range newRegistries.Configs {
			c, err := h.syncContainerdRegistryConfigSecret(prevConfSecrets, newConfSecrets, host, config)
			if err != nil {
				return err
			}

			clusterRegistry.Configs[host] = c
		}
	}

	defer h.pruneUnusedContainerdRegistryConfigSecrets(prevConfSecrets, newConfSecrets)

	cluster, err := h.clusterCache.Get(util.FleetLocalNamespaceName, util.LocalClusterName)
	if err != nil {
		ll.WithFields(logrus.Fields{
			"containerd_registry_sync_cluster_namespace": util.FleetLocalNamespaceName,
			"containerd_registry_sync_cluster_name":      util.LocalClusterName,
		}).Error("clusterCache.Get")
		return err
	}

	clusterCopy := cluster.DeepCopy()
	if clusterCopy.Spec.RKEConfig == nil {
		clusterCopy.Spec.RKEConfig = new(provisioningv1.RKEConfig)
	}

	if reflect.DeepEqual(clusterCopy.Spec.RKEConfig.Registries, clusterRegistry) {
		return nil
	}

	clusterCopy.Spec.RKEConfig.Registries = clusterRegistry

	if _, err := h.clusters.Update(clusterCopy); err != nil {
		ll.WithFields(logrus.Fields{
			"containerd_registry_sync_cluster_namespace": util.FleetLocalNamespaceName,
			"containerd_registry_sync_cluster_name":      util.LocalClusterName,
		}).WithError(err).Error("clusterCache.Update")
		return err
	}

	ll.Debug("Synced containerd-registry setting to Rancher")
	return nil
}

func (h *Handler) pruneUnusedContainerdRegistryConfigSecrets(prevConfs, newConfs map[string]*corev1.Secret) {
	for host, secret := range prevConfs {
		ll := logrus.WithFields(logrus.Fields{
			"containerd_registry_sync_host": host,
		})

		_, ok := newConfs[host]
		if ok {
			ll.Debug("Not removing host because it is referenced in updated values")
			continue
		}

		err := h.secrets.Delete(secret.Namespace, secret.Name, &metav1.DeleteOptions{})
		if err != nil {
			ll.WithFields(logrus.Fields{
				"containerd_registry_sync_namespace":   secret.Namespace,
				"containerd_registry_sync_secret_name": secret.Name,
			}).WithError(err).Error("secrets.Delete")
		}
	}
}

func (h *Handler) syncContainerdRegistryConfigSecret(prevConfs, newConfs map[string]*corev1.Secret, host string, config containerd.RegistryConfig) (rkev1.RegistryConfig, error) {
	ll := logrus.WithFields(logrus.Fields{
		"containerd_registry_sync_host": host,
	})

	genSecretName := func(prefix, host string) (string, error) {
		h := sha256.New()
		_, err := io.Copy(h, strings.NewReader(host))
		if err != nil {
			return "", err
		}

		sum := h.Sum(nil)
		return fmt.Sprintf("%s%x", prefix, sum[:8]), nil
	}

	var authSecretName string
	if s, ok := prevConfs[host]; ok {
		authSecretName = s.Name
		ll.Debug("Re-using existing secret for host")
	} else {
		var err error
		authSecretName, err = genSecretName("harvester-containerd-registry-", host)
		if err != nil {
			return rkev1.RegistryConfig{}, err
		}
		ll.Debug("Generating new secret name")
	}

	authSecret, err := h.secretCache.Get(containerdRegistrySecretsNamespace, authSecretName)
	if apierrors.IsNotFound(err) {
		authSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      authSecretName,
				Namespace: containerdRegistrySecretsNamespace,
				Labels: map[string]string{
					util.LabelSetting: settings.ContainerdRegistrySettingName,
				},
			},
			Type: rkev1.AuthConfigSecretType,
		}

		ll = ll.WithFields(logrus.Fields{
			"containerd_registry_sync_secret_name":      authSecretName,
			"containerd_registry_sync_secret_namespace": containerdRegistrySecretsNamespace,
		})

		ll.Debug("Creating new secret")

		authSecret, err = h.secrets.Create(authSecret)
		if err != nil {
			ll.WithError(err).Error("secrets.Create")
			return rkev1.RegistryConfig{}, err
		}
	}
	if err != nil {
		ll.WithError(err).Error("secrets.Get")
		return rkev1.RegistryConfig{}, err
	}

	newConfig := rkev1.RegistryConfig{AuthConfigSecretName: authSecretName}

	authSecretCopy := authSecret.DeepCopy()
	authSecretCopy.Data = map[string][]byte{containerdRegistrySecretDataHost: []byte(host)}

	if config.Auth != nil {
		authSecretCopy.Data[rkev1.UsernameAuthConfigSecretKey] = []byte(config.Auth.Username)
		authSecretCopy.Data[rkev1.PasswordAuthConfigSecretKey] = []byte(config.Auth.Password)
		authSecretCopy.Data[rkev1.AuthAuthConfigSecretKey] = []byte(config.Auth.Auth)
		authSecretCopy.Data[rkev1.IdentityTokenAuthConfigSecretKey] = []byte(config.Auth.IdentityToken)
	}

	if config.TLS != nil {
		newConfig.InsecureSkipVerify = config.TLS.InsecureSkipVerify
	}

	if !reflect.DeepEqual(authSecret, authSecretCopy) {
		_, err = h.secrets.Update(authSecretCopy)
		if err != nil {
			logrus.WithError(err).Error("secrets.Update")
			return rkev1.RegistryConfig{}, err
		}
	}

	newConfs[host] = authSecretCopy

	return newConfig, nil
}

func (h *Handler) containerdRegistryConfigSecrets() (map[string]*corev1.Secret, error) {
	selector := labels.SelectorFromSet(labels.Set(
		map[string]string{util.LabelSetting: settings.ContainerdRegistrySettingName},
	))

	currentConfSecrets, err := h.secretCache.List(containerdRegistrySecretsNamespace, selector)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"containerd_registry_sync_namespace": containerdRegistrySecretsNamespace,
			"containerd_registry_sync_selector":  selector.String(),
		}).WithError(err).Error("secretCache.List")
		return nil, err
	}

	confs := make(map[string]*corev1.Secret, len(currentConfSecrets))
	for _, s := range currentConfSecrets {
		host := s.Data[containerdRegistrySecretDataHost]
		confs[string(host)] = s
		logrus.WithFields(logrus.Fields{
			"containerd_registry_sync_host":             string(host),
			"containerd_registry_sync_secret_name":      s.Name,
			"containerd_registry_sync_secret_namespace": s.Namespace,
		}).Debug("Found secret")
	}
	return confs, nil
}
