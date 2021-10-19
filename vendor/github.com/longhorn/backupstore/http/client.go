package http

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
)

func getSystemCerts() *x509.CertPool {
	certs, _ := x509.SystemCertPool()
	if certs == nil {
		certs = x509.NewCertPool()
	}
	return certs
}

// GetDefaultClient returns a client that does ssl verification based on the system certificates
func GetDefaultClient() (*http.Client, error) {
	return GetClient(false, nil)
}

func GetInsecureClient() (*http.Client, error) {
	return GetClient(true, nil)
}

func GetClientWithCustomCerts(certs []byte) (*http.Client, error) {
	return GetClient(false, certs)
}

func GetClient(insecure bool, customCerts []byte) (*http.Client, error) {
	certs := getSystemCerts()

	// CA's are base 64 encoded and appended together
	// can contain root CAs, intermediate CAs and certificates
	if ok := certs.AppendCertsFromPEM(customCerts); customCerts != nil && !ok {
		return nil, fmt.Errorf("failed to append custom certificates: %v", string(customCerts))
	}

	// create custom http client that trusts our custom certs
	customTransport := http.DefaultTransport.(*http.Transport).Clone()
	customTransport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: insecure,
		RootCAs:            certs,
	}
	client := &http.Client{Transport: customTransport}
	return client, nil
}
