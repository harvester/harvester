package setting

import (
	"crypto/sha256"
	"fmt"
	"time"

	v1 "github.com/rancher/wrangler/pkg/generated/controllers/apps/v1"
	corev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

type syncerFunc func(*harvesterv1.Setting) error

var syncers map[string]syncerFunc

type Handler struct {
	namespace       string
	settings        v1beta1.SettingClient
	secrets         corev1.SecretClient
	secretCache     corev1.SecretCache
	deployments     v1.DeploymentClient
	deploymentCache v1.DeploymentCache
}

func (h *Handler) settingOnChanged(_ string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil {
		return nil, nil
	}

	// The setting value hash is stored in the annotation when a setting syncer completes.
	// So that we only proceed when value is changed.
	if setting.Value == "" && setting.Annotations[util.AnnotationHash] == "" {
		return nil, nil
	}
	hash := sha256.New224()
	hash.Write([]byte(setting.Value))
	currentHash := fmt.Sprintf("%x", hash.Sum(nil))
	if currentHash == setting.Annotations[util.AnnotationHash] {
		return nil, nil
	}

	for key, syncer := range syncers {
		if setting.Name != key {
			continue
		}

		if err := syncer(setting); err != nil {
			return setting, err
		}

		toUpdate := setting.DeepCopy()
		if toUpdate.Annotations == nil {
			toUpdate.Annotations = make(map[string]string)
		}
		toUpdate.Annotations[util.AnnotationHash] = currentHash
		return h.settings.Update(toUpdate)
	}

	return nil, nil
}

func (h *Handler) updateBackupSecret(data map[string]string) error {
	secret, err := h.secretCache.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	toUpdate := secret.DeepCopy()
	if toUpdate.Data == nil {
		toUpdate.Data = make(map[string][]byte)
	}
	for key, value := range data {
		toUpdate.Data[key] = []byte(value)
	}
	_, err = h.secrets.Update(toUpdate)
	return err
}

func (h *Handler) redeployDeployment(namespace, name string) error {
	deployment, err := h.deploymentCache.Get(namespace, name)
	if err != nil {
		return err
	}
	toUpdate := deployment.DeepCopy()
	if deployment.Spec.Template.Annotations == nil {
		toUpdate.Spec.Template.Annotations = make(map[string]string)
	}
	toUpdate.Spec.Template.Annotations[util.AnnotationTimestamp] = time.Now().Format(time.RFC3339)

	_, err = h.deployments.Update(toUpdate)
	return err
}
