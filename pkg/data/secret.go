package data

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/util"
)

func createSecrets(mgmt *config.Management) error {
	secrets := mgmt.CoreFactory.Core().V1().Secret()

	// Initializing the secret for Plan cattle-system/sync-additional-ca,
	// so sync-additional-ca doesn't fail to mount harvester-additional-ca secret to jobs.
	additionalCA := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: util.CattleSystemNamespaceName,
			Name:      util.AdditionalCASecretName,
		},
		Data: map[string][]byte{
			util.AdditionalCAFileName: []byte(""),
		},
	}
	if _, err := secrets.Create(&additionalCA); err != nil && !apierrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "Failed to create secret %s/%s", additionalCA.Namespace, additionalCA.Name)
	}

	return nil
}
