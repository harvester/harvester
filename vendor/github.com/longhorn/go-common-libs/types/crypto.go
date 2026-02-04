package types

import (
	"time"
)

const (
	CryptoKeyProvider = "CRYPTO_KEY_PROVIDER"
	CryptoKeyValue    = "CRYPTO_KEY_VALUE"
	CryptoKeyCipher   = "CRYPTO_KEY_CIPHER"
	CryptoKeyHash     = "CRYPTO_KEY_HASH"
	CryptoKeySize     = "CRYPTO_KEY_SIZE"
	CryptoPBKDF       = "CRYPTO_PBKDF"
)

const LuksTimeout = time.Minute
