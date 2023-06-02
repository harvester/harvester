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

	// Initializing the secret for Plan cattle-system/sync-additional-ca and cattle-system/sync-rke2-registries,
	// so plans don't fail to mount secrets to jobs.
	defaultSecrets := []corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: util.CattleSystemNamespaceName,
				Name:      util.AdditionalCASecretName,
			},
			Data: map[string][]byte{
				util.AdditionalCAFileName: []byte(""),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: util.CattleSystemNamespaceName,
				Name:      util.ContainerdRegistrySecretName,
			},
			Data: map[string][]byte{
				util.ContainerdRegistryFileName: []byte(""),
			},
		},
	}
	for i := range defaultSecrets {
		defaultSecret := defaultSecrets[i]
		if _, err := secrets.Create(&defaultSecret); err != nil && !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "Failed to create secret %s/%s", defaultSecret.Namespace, defaultSecret.Name)
		}
	}

	return nil
}
