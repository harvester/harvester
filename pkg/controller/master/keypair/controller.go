package keypair

import (
	"fmt"

	"golang.org/x/crypto/ssh"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

// Handler computes key pairs' fingerprints
type Handler struct {
	keyPairClient ctlharvesterv1.KeyPairClient
}

func (h *Handler) OnKeyPairChanged(key string, keyPair *harvesterv1.KeyPair) (*harvesterv1.KeyPair, error) {
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
		harvesterv1.KeyPairValidated.False(toUpdate)
		harvesterv1.KeyPairValidated.Reason(toUpdate, fmt.Sprintf("failed to parse the public key, error: %v", err))
	} else {
		fingerPrint := ssh.FingerprintLegacyMD5(pk)
		toUpdate.Status.FingerPrint = fingerPrint
		harvesterv1.KeyPairValidated.True(toUpdate)
	}
	return h.keyPairClient.Update(toUpdate)
}
