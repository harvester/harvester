/*
Package encryption provides encryption and decryption functions, while
abstracting away key management concerns.
Uses AES-GCM encryption, with key rotation, keeping keys in memory.
*/
package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"sync"

	"github.com/pkg/errors"
)

var (
	ErrKeyNotFound = errors.New("data key not found")
	// maxWriteCount holds the maximum amount of times the active key can be
	// used, prior to it being rotated. 2^32 is the currently recommended key
	// wear-out params by NIST for AES-GCM using random nonces.
	maxWriteCount int64 = 1 << 32
)

const (
	keySize = 32 // 32 for AES-256
)

// Manager uses AES-GCM encryption and keeps in memory the data encryption
// keys. The active encryption key is automatically rotated once it has been
// used over a certain amount of times - defined by maxWriteCount.
type Manager struct {
	dataKeys         [][]byte
	activeKeyCounter int64

	// lock works as the mutual exclusion lock for dataKeys.
	lock sync.RWMutex
	// counterLock works as the mutual exclusion lock for activeKeyCounter.
	counterLock sync.Mutex
}

// NewManager returns Manager, which satisfies db.Encryptor and db.Decryptor
func NewManager() (*Manager, error) {
	m := &Manager{
		dataKeys: [][]byte{},
	}
	m.newDataEncryptionKey()

	return m, nil
}

// Encrypt encrypts the specified data, returning: the encrypted data, the nonce used to encrypt the data, and an ID identifying the key that was used (as it rotates). On failure error is returned instead.
func (m *Manager) Encrypt(data []byte) ([]byte, []byte, uint32, error) {
	dek, keyID, err := m.fetchActiveDataKey()
	if err != nil {
		return nil, nil, 0, err
	}
	aead, err := createGCMCypher(dek)
	if err != nil {
		return nil, nil, 0, err
	}
	edata, nonce, err := encrypt(aead, data)
	if err != nil {
		return nil, nil, 0, err
	}
	return edata, nonce, keyID, nil
}

// Decrypt accepts a chunk of encrypted data, the nonce used to encrypt it and the ID of the used key (as it rotates). It returns the decrypted data or an error.
func (m *Manager) Decrypt(edata, nonce []byte, keyID uint32) ([]byte, error) {
	dek, err := m.key(keyID)
	if err != nil {
		return nil, err
	}

	aead, err := createGCMCypher(dek)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create GCMCypher from DEK")
	}
	data, err := aead.Open(nil, nonce, edata, nil)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to decrypt data using keyid %d", keyID))
	}
	return data, nil
}

func encrypt(aead cipher.AEAD, data []byte) ([]byte, []byte, error) {
	if aead == nil {
		return nil, nil, fmt.Errorf("aead is nil, cannot encrypt data")
	}
	nonce := make([]byte, aead.NonceSize())
	_, err := rand.Read(nonce)
	if err != nil {
		return nil, nil, err
	}
	sealed := aead.Seal(nil, nonce, data, nil)
	return sealed, nonce, nil
}

func createGCMCypher(key []byte) (cipher.AEAD, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	return aead, nil
}

// fetchActiveDataKey returns the current data key and its key ID.
// Each call results in activeKeyCounter being incremented by 1. When the
// the activeKeyCounter exceeds maxWriteCount, the active data key is
// rotated - before being returned.
func (m *Manager) fetchActiveDataKey() ([]byte, uint32, error) {
	m.counterLock.Lock()
	defer m.counterLock.Unlock()

	m.activeKeyCounter++
	if m.activeKeyCounter >= maxWriteCount {
		return m.newDataEncryptionKey()
	}

	return m.activeKey()
}

func (m *Manager) newDataEncryptionKey() ([]byte, uint32, error) {
	dek := make([]byte, keySize)
	_, err := rand.Read(dek)
	if err != nil {
		return nil, 0, err
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	m.activeKeyCounter = 1

	m.dataKeys = append(m.dataKeys, dek)
	keyID := uint32(len(m.dataKeys) - 1)

	return dek, keyID, nil
}

func (m *Manager) activeKey() ([]byte, uint32, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	nk := len(m.dataKeys)
	if nk == 0 {
		return nil, 0, ErrKeyNotFound
	}
	keyID := uint32(nk - 1)

	return m.dataKeys[keyID], keyID, nil
}

func (m *Manager) key(keyID uint32) ([]byte, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if len(m.dataKeys) <= int(keyID) {
		return nil, fmt.Errorf("%w: %v", ErrKeyNotFound, keyID)
	}
	return m.dataKeys[keyID], nil
}
