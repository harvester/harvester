package cert

import v1 "k8s.io/api/core/v1"

func IsValidTLSSecret(secret *v1.Secret) bool {
	if secret == nil {
		return false
	}
	if _, ok := secret.Data[v1.TLSCertKey]; !ok {
		return false
	}
	if _, ok := secret.Data[v1.TLSPrivateKeyKey]; !ok {
		return false
	}
	return true
}
