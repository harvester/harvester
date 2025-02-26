package common

import (
	"crypto/rand"
	"math/big"
)

// GenRandNumber generates a random positive number less than the boundary value.
// The return value should be [0, boundary) and error is nil when success.
// If the error is not nil, the return value is 0.
func GenRandNumber(boundary int64) (int64, error) {
	randNum, err := rand.Int(rand.Reader, big.NewInt(boundary))
	if err != nil {
		return 0, err
	}
	return randNum.Int64(), nil
}
