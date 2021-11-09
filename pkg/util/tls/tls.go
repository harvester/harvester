package tls

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// ValidateServingBundle validates that a PEM bundle contains at least one certificate,
// and that the first certificate has a CN or SAN set.
func ValidateServingBundle(data []byte) error {
	var exists bool

	// The first PEM in a bundle should always have a CN set.
	i := 0

	for containsPEMHeader(data) {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			return errors.New("failed to parse PEM block")
		}
		if block.Type != "CERTIFICATE" {
			return fmt.Errorf("unexpected block type '%s'", block.Type)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return err
		}

		// Only run the CN and SAN checks on the first cert in a bundle
		if i == 0 && !hasCommonName(cert) && !hasSubjectAltNames(cert) {
			return errors.New("certificate has no common name or subject alt name")
		}

		exists = true
		i++
	}

	if !exists {
		return errors.New("failed to locate certificate")
	}

	return nil
}

// ValidateCABundle validates that a PEM bundle contains at least one valid certificate.
func ValidateCABundle(data []byte) error {
	var exists bool

	for containsPEMHeader(data) {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			return errors.New("failed to parse PEM block")
		}
		if block.Type != "CERTIFICATE" {
			return fmt.Errorf("unexpected block type '%s'", block.Type)
		}

		exists = true
	}

	if !exists {
		return errors.New("failed to locate certificate")
	}
	return nil
}

// ValidatePrivateKey validates that a PEM bundle contains a private key.
func ValidatePrivateKey(data []byte) error {
	var keys int

	for containsPEMHeader(data) {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			return errors.New("failed to parse PEM block")
		}
		switch block.Type {
		case "PRIVATE KEY":
			if _, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
				return err
			}
			keys++
		case "RSA PRIVATE KEY":
			if _, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
				return err
			}
			keys++
		case "EC PRIVATE KEY":
			if _, err := x509.ParseECPrivateKey(block.Bytes); err != nil {
				return err
			}
			keys++
		case "EC PARAMETERS":
			// ignored
		default:
			return fmt.Errorf("unexpected block type '%s'", block.Type)
		}
	}

	switch keys {
	case 0:
		return errors.New("failed to locate private key")
	case 1:
		return nil
	default:
		return errors.New("multiple private keys")
	}
}

func hasCommonName(c *x509.Certificate) bool {
	return strings.TrimSpace(c.Subject.CommonName) != ""
}

func hasSubjectAltNames(c *x509.Certificate) bool {
	return len(c.DNSNames) > 0 || len(c.IPAddresses) > 0
}

// containsPEMHeader returns true if the given slice contains a string that looks like a PEM header block. The problem
// is that pem.Decode does not give us a way to distinguish between a missing PEM block and an invalid PEM block. This
// means that if there is any non-PEM data at the end of a byte slice, we would normally detect it as an error. However,
// users of the OpenSSL API check for the `PEM_R_NO_START_LINE` error code and would accept files with trailing non-PEM
// data.
func containsPEMHeader(data []byte) bool {
	// A PEM header starts with the begin token.
	start := bytes.Index(data, []byte("-----BEGIN"))
	if start == -1 {
		return false
	}

	// And ends with the end token.
	end := bytes.Index(data[start+10:], []byte("-----"))
	if end == -1 {
		return false
	}

	// And must be on a single line.
	if bytes.Contains(data[start:start+end], []byte("\n")) {
		return false
	}

	return true
}
