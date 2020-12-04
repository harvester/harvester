package secret

import (
	corev1 "k8s.io/api/core/v1"

	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
)

const (
	controllerName              = "serving-cert-controller"
	servingCertName             = "serving-cert"
	servingCertNamespace        = "kube-system"
	servingCertStaticAnnotation = "listener.cattle.io/static"
)

type Handler struct {
	secretCache ctlcorev1.SecretCache
	secrets     ctlcorev1.SecretClient
}

// OnSecretChanged After the serving-cert is created, add annotation to avoid being updated by dynamiclistener again.
func (h *Handler) OnSecretChanged(key string, secret *corev1.Secret) (*corev1.Secret, error) {
	if secret == nil || secret.DeletionTimestamp != nil {
		return secret, nil
	}

	if secret.Namespace != servingCertNamespace || secret.Name != servingCertName {
		return secret, nil
	}

	toUpdate := secret.DeepCopy()
	if toUpdate.Annotations == nil {
		toUpdate.Annotations = make(map[string]string)
	}
	toUpdate.Annotations[servingCertStaticAnnotation] = "true"

	return h.secrets.Update(toUpdate)
}
