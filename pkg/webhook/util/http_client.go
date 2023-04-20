package util

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
)

func GetHTTPTransportWithCertificates(config *rest.Config) (*http.Transport, error) {
	logrus.Debugf("config: %+v", config)
	caCert, err := ioutil.ReadFile(config.TLSClientConfig.CAFile)
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
