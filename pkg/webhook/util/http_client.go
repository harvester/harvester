package util

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
)

func GetHTTPTransportWithCertificates(config *rest.Config) (*http.Transport, error) {
	logrus.Debugf("config: %+v", config)
	caCert, err := os.ReadFile(config.TLSClientConfig.CAFile)
	if err != nil {
		return nil, err
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tr := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
		TLSClientConfig: &tls.Config{
			RootCAs:            caCertPool,
			InsecureSkipVerify: false,
		},
	}

	return tr, nil
}
