package rancher

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"
)

func (h *Handler) TLSSecretOnChange(_ string, secret *corev1.Secret) (*corev1.Secret, error) {
	if secret == nil || secret.DeletionTimestamp != nil || secret.Namespace != util.CattleSystemNamespaceName || secret.Name != util.InternalTLSSecretName {
		return nil, nil
	}
	return nil, h.addVIPToSAN()
}

// addVIPToSAN writes VIP to TLS SAN of the serving cert
func (h *Handler) addVIPToSAN() error {
	vipConfig, err := h.getVipConfig()
	if err != nil {
		return err
	}
	secret, err := h.SecretCache.Get(util.CattleSystemNamespaceName, util.InternalTLSSecretName)
	if err != nil {
		return err
	}

	toAddAnnotation := tlsCNPrefix + vipConfig.IP
	if _, ok := secret.Annotations[toAddAnnotation]; ok {
		return nil
	}

	toUpdate := secret.DeepCopy()
	if toUpdate.Annotations == nil {
		toUpdate.Annotations = make(map[string]string)
	}
	//clean up other cns
	newAnnotations := map[string]string{}
	for k, v := range toUpdate.Annotations {
		if !strings.Contains(k, tlsCNPrefix) {
			newAnnotations[k] = v
		}
	}
	toUpdate.Annotations = newAnnotations
	toUpdate.Annotations[toAddAnnotation] = vipConfig.IP
	_, err = h.Secrets.Update(toUpdate)
	return err
}
