package util

import (
	"crypto/tls"
	"crypto/x509"

	"github.com/sirupsen/logrus"
)

func GetAddrEarliestExpiringCert(addr string) (*x509.Certificate, error) {
	conf := &tls.Config{
		InsecureSkipVerify: true,
	}

	conn, err := tls.Dial("tcp", addr, conf)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"addr": addr,
			"conf": conf,
		}).WithError(err).Error("tls.Dial")
		return nil, err
	}
	defer conn.Close()

	var earliestExpiringCert *x509.Certificate
	certs := conn.ConnectionState().PeerCertificates
	for _, cert := range certs {
		if earliestExpiringCert == nil || earliestExpiringCert.NotAfter.After(cert.NotAfter) {
			earliestExpiringCert = cert
		}
	}

	return earliestExpiringCert, nil
}

func GetAddrsEarliestExpiringCert(addrs []string) (*x509.Certificate, error) {
	var earliestExpiringCert *x509.Certificate
	for _, addr := range addrs {
		cert, err := GetAddrEarliestExpiringCert(addr)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"addr": addr,
			}).WithError(err).Error("GetEarliestExpiringCert")
			continue
		}

		if earliestExpiringCert == nil || earliestExpiringCert.NotAfter.After(cert.NotAfter) {
			earliestExpiringCert = cert
		}
	}

	return earliestExpiringCert, nil
}
