package factory

import (
	"crypto"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/rancher/dynamiclistener/cert"
)

func GenCA() (*x509.Certificate, crypto.Signer, error) {
	caKey, err := NewPrivateKey()
	if err != nil {
		return nil, nil, err
	}

	caCert, err := NewSelfSignedCACert(caKey, fmt.Sprintf("dynamiclistener-ca@%d", time.Now().Unix()), "dynamiclistener-org")
	if err != nil {
		return nil, nil, err
	}

	return caCert, caKey, nil
}

// Deprecated: Use LoadOrGenCAChain instead as it supports intermediate CAs
func LoadOrGenCA() (*x509.Certificate, crypto.Signer, error) {
	chain, signer, err := LoadOrGenCAChain()
	if err != nil {
		return nil, nil, err
	}
	return chain[0], signer, err
}

func LoadOrGenCAChain() ([]*x509.Certificate, crypto.Signer, error) {
	certs, key, err := loadCA()
	if err == nil {
		return certs, key, nil
	}

	cert, key, err := GenCA()
	if err != nil {
		return nil, nil, err
	}
	certs = []*x509.Certificate{cert}

	certBytes, keyBytes, err := MarshalChain(key, certs...)
	if err != nil {
		return nil, nil, err
	}

	if err := os.MkdirAll("./certs", 0700); err != nil {
		return nil, nil, err
	}

	if err := ioutil.WriteFile("./certs/ca.pem", certBytes, 0600); err != nil {
		return nil, nil, err
	}

	if err := ioutil.WriteFile("./certs/ca.key", keyBytes, 0600); err != nil {
		return nil, nil, err
	}

	return certs, key, nil
}

func loadCA() ([]*x509.Certificate, crypto.Signer, error) {
	return LoadCertsChain("./certs/ca.pem", "./certs/ca.key")
}

func LoadCA(caPem, caKey []byte) (*x509.Certificate, crypto.Signer, error) {
	chain, signer, err := LoadCAChain(caPem, caKey)
	if err != nil {
		return nil, nil, err
	}
	return chain[0], signer, nil
}

func LoadCAChain(caPem, caKey []byte) ([]*x509.Certificate, crypto.Signer, error) {
	key, err := cert.ParsePrivateKeyPEM(caKey)
	if err != nil {
		return nil, nil, err
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, nil, fmt.Errorf("key is not a crypto.Signer")
	}

	certs, err := cert.ParseCertsPEM(caPem)
	if err != nil {
		return nil, nil, err
	}

	return certs, signer, nil
}

// Deprecated: Use LoadCertsChain instead as it supports intermediate CAs
func LoadCerts(certFile, keyFile string) (*x509.Certificate, crypto.Signer, error) {
	chain, signer, err := LoadCertsChain(certFile, keyFile)
	if err != nil {
		return nil, nil, err
	}
	return chain[0], signer, err
}

func LoadCertsChain(certFile, keyFile string) ([]*x509.Certificate, crypto.Signer, error) {
	caPem, err := ioutil.ReadFile(certFile)
	if err != nil {
		return nil, nil, err
	}
	caKey, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, nil, err
	}

	return LoadCAChain(caPem, caKey)
}
