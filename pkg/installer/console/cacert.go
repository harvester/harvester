package console

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/util/rand"
)

func newSecureProxyClient(pool *x509.CertPool) http.Client {
	client := newProxyClient()
	client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
	}
	return client
}

// fetchVerifiedCA retrieves the server CA over insecure TLS and authenticates it via
// HMAC-SHA512 using the join token before trusting it (rancherd bootstrap protocol).
func fetchVerifiedCA(serverURL, token string) (*x509.CertPool, error) {
	nonce := rand.String(16)

	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(serverURL, "/")+"/cacerts", nil)
	if err != nil {
		return nil, &permanentError{err}
	}
	req.Header.Set("Authorization", "Bearer "+hashBase64([]byte(token)))
	req.Header.Set("X-Cattle-Nonce", nonce)

	//nolint:gosec // insecure TLS is intentional; the CA is authenticated by HMAC before it is trusted
	insecureClient := newInsecureProxyClient()
	resp, err := insecureClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// shortcut: 1 MiB CA response limit, raise it only if supported CA bundles exceed it.
	caBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, cacertsResponseError(resp.StatusCode, caBytes)
	}

	if err := checkHMACHeader(resp.Header.Get("X-Cattle-Hash"), token, nonce, caBytes); err != nil {
		return nil, err
	}

	if len(bytes.TrimSpace(caBytes)) == 0 {
		return nil, &permanentError{fmt.Errorf("server returned empty CA")}
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caBytes) {
		return nil, &permanentError{fmt.Errorf("failed to parse server CA certificate")}
	}
	return pool, nil
}

func cacertsResponseError(statusCode int, data []byte) error {
	err := fmt.Errorf("cacerts returned %d: %s", statusCode, data)
	if statusCode >= 400 && statusCode < 500 {
		return &permanentError{err}
	}
	return err
}

func checkHMACHeader(header, token, nonce string, data []byte) error {
	if header == "" {
		return &permanentError{fmt.Errorf("missing X-Cattle-Hash header; join token may be incorrect")}
	}
	if !hmac.Equal([]byte(header), []byte(hmacHash(token, nonce, data))) {
		return &permanentError{fmt.Errorf("CA verification failed: HMAC mismatch, possible MITM attack")}
	}
	return nil
}

func hmacHash(token, nonce string, data []byte) string {
	mac := hmac.New(sha512.New, []byte(token))
	mac.Write([]byte(nonce))
	mac.Write([]byte{0})
	mac.Write(data)
	mac.Write([]byte{0})
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func hashBase64(b []byte) string {
	h := sha256.Sum256(b)
	return base64.StdEncoding.EncodeToString(h[:])
}
