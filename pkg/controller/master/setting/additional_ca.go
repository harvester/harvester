package setting

import (
	"fmt"

	"k8s.io/apimachinery/pkg/labels"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

func (h *Handler) syncAdditionalTrustedCAs(setting *harvesterv1.Setting) error {
	// Add envs to the backup secret used by Longhorn backups
	backupConfig := map[string]string{
		"AWS_CERT": setting.Value,
	}
	if err := h.updateBackupSecret(backupConfig); err != nil {
		return err
	}

	// Distribute CA secrets to required system namespaces
	if err := h.syncAdditionalCASecrets(setting); err != nil {
		return err
	}

	// Update all existing CDI ConfigMaps across all namespaces.
	if err := h.syncCDIAdditionalCAConfigMaps(setting); err != nil {
		return err
	}

	//redeploy system services. The CA certs will be injected by the mutation webhook.
	return h.redeployDeployment(h.namespace, "harvester")
}

func (h *Handler) syncAdditionalCASecrets(setting *harvesterv1.Setting) error {
	namespaces := []string{h.namespace, util.LonghornSystemNamespaceName, util.CattleSystemNamespaceName}

	for _, namespace := range namespaces {
		secret, err := h.secretCache.Get(namespace, util.AdditionalCASecretName)
		if errors.IsNotFound(err) {
			newSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.AdditionalCASecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					util.AdditionalCAFileName: []byte(setting.Value),
				},
			}

			if _, err := h.secrets.Create(newSecret); err != nil {
				return err
			}
			continue
		} else if err != nil {
			return err
		}

		toUpdate := secret.DeepCopy()
		toUpdate.Data = map[string][]byte{
			util.AdditionalCAFileName: []byte(setting.Value),
		}
		if _, err := h.secrets.Update(toUpdate); err != nil {
			return err
		}
	}
	return nil
}

// syncCDIAdditionalCAConfigMaps updates all existing 'harvester-additional-ca-cdi' ConfigMaps
// across all namespaces when the 'additional-ca' setting changes.
func (h *Handler) syncCDIAdditionalCAConfigMaps(setting *harvesterv1.Setting) error {
	namespaces, err := h.namespacesCache.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	cmData := map[string]string{
		util.CDIAdditionalCAConfigMapKey: setting.Value,
	}

	// Update 'ConfigMap' in each namespace where it exists.
	for _, ns := range namespaces {
		cm, err := h.configmapCache.Get(ns.Name, util.CDIAdditionalCAConfigMapName)
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}

		// Update if data has changed.
		_, err = util.UpdateConfigMapData(h.configmaps, cm, cmData)
		if err != nil {
			return fmt.Errorf("failed to update additional CA in ConfigMap %q: %w", util.GetNamespacedName(cm), err)
		}
	}

	return nil
}
