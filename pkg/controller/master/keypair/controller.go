package keypair

import (
	"fmt"

	apisv1alpha1 "github.com/rancher/vm/pkg/apis/vm.cattle.io/v1alpha1"
	"github.com/rancher/vm/pkg/generated/controllers/vm.cattle.io/v1alpha1"
	"golang.org/x/crypto/ssh"
)

// Handler computes key pairs' fingerprints
type Handler struct {
	keyPairs     v1alpha1.KeyPairController
	keyPairCache v1alpha1.KeyPairCache
}

func (h *Handler) OnKeyPairChanged(key string, keyPair *apisv1alpha1.KeyPair) (*apisv1alpha1.KeyPair, error) {
	if keyPair == nil || keyPair.DeletionTimestamp != nil {
		return nil, nil
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
	_, err = h.keyPairs.Update(toUpdate)
	return keyPair, err
}
