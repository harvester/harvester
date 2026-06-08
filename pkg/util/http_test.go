package util

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildHTTPClientWithCA(t *testing.T) {
	validPEM, validCert := generateSelfSignedCertificate(t)

	tests := []struct {
		name              string
		additionalCA      string
		expectDefault     bool
		expectCertTrusted bool
	}{
		{
			name:          "empty additional CA returns default client",
			additionalCA:  "",
			expectDefault: true,
		},
		{
			name:              "valid additional CA extends trust store",
			additionalCA:      validPEM,
			expectCertTrusted: true,
		},
		{
			name:              "invalid additional CA falls back to system pool",
			additionalCA:      "not-a-pem",
			expectCertTrusted: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := BuildHTTPClientWithCA(tc.additionalCA)

			if tc.expectDefault {
				assert.Same(t, http.DefaultClient, client)
				return
			}

			assert.NotSame(t, http.DefaultClient, client)

			transport, ok := client.Transport.(*http.Transport)
			require.True(t, ok, "expected *http.Transport")

			defaultTransport, ok := http.DefaultTransport.(*http.Transport)
			require.True(t, ok, "expected default transport to be *http.Transport")

			assert.NotSame(t, defaultTransport, transport)
			assert.Equal(t, defaultTransport.MaxIdleConns, transport.MaxIdleConns)
			assert.Equal(t, defaultTransport.ForceAttemptHTTP2, transport.ForceAttemptHTTP2)

			require.NotNil(t, transport.TLSClientConfig)
			require.NotNil(t, transport.TLSClientConfig.RootCAs)

			_, err := validCert.Verify(x509.VerifyOptions{Roots: transport.TLSClientConfig.RootCAs})
			if tc.expectCertTrusted {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func generateSelfSignedCertificate(t *testing.T) (string, *x509.Certificate) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "harvester-test-ca",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, privateKey.Public(), privateKey)
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	certificate, err := x509.ParseCertificate(derBytes)
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	return string(pemBytes), certificate
}
