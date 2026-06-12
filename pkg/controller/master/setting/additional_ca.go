package setting

import (
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const cdiAdditionalCAConfigMapKey = "ca.pem"

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

	// Distribute CA as a ConfigMap in the CDI install namespace (h.namespace)
	// and set TrustedCAProxy on the CDI object.
	if err := h.syncCDIAdditionalCA(setting); err != nil {
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

// syncCDIAdditionalCA ensures that:
//  1. A ConfigMap named CDIAdditionalCAConfigMapName exists in h.namespace (harvester-system)
//     and contains the additional CA PEM under the key "ca.pem".
//  2. The singleton CDI object's Spec.Config.ImportProxy.TrustedCAProxy is set to the
//     ConfigMap name (or cleared when the CA value is empty), so that the CDI operator
//     injects the CA into every importer pod.
func (h *Handler) syncCDIAdditionalCA(setting *harvesterv1.Setting) error {
	if err := h.syncCDIAdditionalCAConfigMap(setting); err != nil {
		return err
	}
	return h.syncCDITrustedCAProxy(setting)
}

func (h *Handler) syncCDIAdditionalCAConfigMap(setting *harvesterv1.Setting) error {
	data := map[string]string{
		cdiAdditionalCAConfigMapKey: setting.Value,
	}

	cm, err := h.configmapCache.Get(h.namespace, util.CDIAdditionalCAConfigMapName)
	if errors.IsNotFound(err) {
		newCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      util.CDIAdditionalCAConfigMapName,
				Namespace: h.namespace,
			},
			Data: data,
		}
		_, err = h.configmaps.Create(newCM)
		return err
	}
	if err != nil {
		return err
	}

	if reflect.DeepEqual(cm.Data, data) {
		return nil
	}

	toUpdate := cm.DeepCopy()
	toUpdate.Data = data
	_, err = h.configmaps.Update(toUpdate)
	return err
}

func (h *Handler) syncCDITrustedCAProxy(setting *harvesterv1.Setting) error {
	cdi, err := h.cdiCache.Get(util.CDIObjectName)
	if err != nil {
		if errors.IsNotFound(err) {
			// CDI not installed yet –> nothing to configure.
			return nil
		}
		return fmt.Errorf("failed to get CDI object %q: %w", util.CDIObjectName, err)
	}

	toUpdate := cdi.DeepCopy()
	changed := false

	if setting.Value == "" {
		// Clear the TrustedCAProxy when no additional CA is configured.
		if toUpdate.Spec.Config != nil && toUpdate.Spec.Config.ImportProxy != nil && toUpdate.Spec.Config.ImportProxy.TrustedCAProxy != nil {
			toUpdate.Spec.Config.ImportProxy.TrustedCAProxy = nil
			changed = true
		}
	} else {
		cmName := util.CDIAdditionalCAConfigMapName
		if toUpdate.Spec.Config == nil {
			toUpdate.Spec.Config = &cdiv1.CDIConfigSpec{}
			changed = true
		}
		if toUpdate.Spec.Config.ImportProxy == nil {
			toUpdate.Spec.Config.ImportProxy = &cdiv1.ImportProxy{}
			changed = true
		}
		if toUpdate.Spec.Config.ImportProxy.TrustedCAProxy == nil || *toUpdate.Spec.Config.ImportProxy.TrustedCAProxy != cmName {
			toUpdate.Spec.Config.ImportProxy.TrustedCAProxy = &cmName
			changed = true
		}
	}

	if !changed {
		return nil
	}

	if _, err := h.cdiClient.Update(toUpdate); err != nil {
		return fmt.Errorf("failed to update CDI object %q with TrustedCAProxy: %w", util.CDIObjectName, err)
	}
	return nil
}
