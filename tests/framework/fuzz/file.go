package fuzz

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
)

type FileByteSize int64

const (
	B FileByteSize = 1 << (10 * iota)
	KB
	MB
	GB
)

// File tries to create a file with given size and returns the path and the SHA256 checksum or error.
func File(size FileByteSize) (path string, checksum string, berr error) {
	tempFile, err := ioutil.TempFile("", "rand-")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		_ = tempFile.Close()
		if berr != nil {
			_ = os.Remove(tempFile.Name())
		}
	}()

	sha256Hasher := sha256.New()

	_, err = io.CopyN(io.MultiWriter(tempFile, sha256Hasher), rand.Reader, int64(size))
	if err != nil {
		return "", "", fmt.Errorf("failed to create random file: %w", err)
	}

	return tempFile.Name(), hex.EncodeToString(sha256Hasher.Sum(nil)), nil
}
