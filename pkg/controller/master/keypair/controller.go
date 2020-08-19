package keypair

import (
	"fmt"

	"golang.org/x/crypto/ssh"

	apisv1alpha1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
)

// Handler computes key pairs' fingerprints
type Handler struct {
	keyPairClient v1alpha1.KeyPairClient
}

func (h *Handler) OnKeyPairChanged(key string, keyPair *apisv1alpha1.KeyPair) (*apisv1alpha1.KeyPair, error) {
	if keyPair == nil || keyPair.DeletionTimestamp != nil {
		return keyPair, nil
	}

	if keyPair.Spec.PublicKey == "" || keyPair.Status.FingerPrint != "" {
		return keyPair, nil
	}

	toUpdate := keyPair.DeepCopy()
	publicKey := []byte(keyPair.Spec.PublicKey)
	pk, _, _, _, err := ssh.ParseAuthorizedKey(publicKey)
	if err != nil {
		apisv1alpha1.KeyPairValidated.False(toUpdate)
		apisv1alpha1.KeyPairValidated.Reason(toUpdate, fmt.Sprintf("failed to parse the public key, error: %v", err))
	} else {
		fingerPrint := ssh.FingerprintLegacyMD5(pk)
		toUpdate.Status.FingerPrint = fingerPrint
		apisv1alpha1.KeyPairValidated.True(toUpdate)
	}
	return h.keyPairClient.Update(toUpdate)
}
