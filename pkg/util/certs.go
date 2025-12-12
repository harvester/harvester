package util

import (
	"crypto/tls"
	"crypto/x509"

	"github.com/sirupsen/logrus"
)

func GetAddrEarliestExpiringCert(addr string, earliestExpiringCert *x509.Certificate) (*x509.Certificate, error) {
	conf := &tls.Config{
		InsecureSkipVerify: true,
	}

	conn, err := tls.Dial("tcp", addr, conf)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"addr": addr,
			"conf": conf,
		}).WithError(err).Error("tls.Dial")
		return earliestExpiringCert, err
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	for _, cert := range certs {
		if earliestExpiringCert == nil || earliestExpiringCert.NotAfter.After(cert.NotAfter) {
			earliestExpiringCert = cert
		}
	}

	return earliestExpiringCert, nil
}

func GetAddrsEarliestExpiringCert(controlPlaneIps, witnessIps, workerIps []string) (*x509.Certificate, error) {
	var (
		earliestExpiringCert *x509.Certificate
		err                  error
	)

	// Kubernetes ports: https://kubernetes.io/docs/reference/networking/ports-and-protocols/
	apiServerPort := "6443"
	etcdClientPort := "2379"
	etcdServerPort := "2380"
	kubeletPort := "10250"
	for _, ip := range controlPlaneIps {
		for _, port := range []string{apiServerPort, etcdClientPort, etcdServerPort, kubeletPort} {
			earliestExpiringCert, err = GetAddrEarliestExpiringCert(ip+":"+port, earliestExpiringCert)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"ip":   ip,
					"port": port,
				}).WithError(err).Error("GetEarliestExpiringCert for control plane node")
				continue
			}
		}
	}

	for _, ip := range witnessIps {
		for _, port := range []string{apiServerPort, etcdClientPort, etcdServerPort} {
			earliestExpiringCert, err = GetAddrEarliestExpiringCert(ip+":"+port, earliestExpiringCert)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"ip":   ip,
					"port": port,
				}).WithError(err).Error("GetEarliestExpiringCert for witness node")
				continue
			}
		}
	}

	for _, ip := range workerIps {
		for _, port := range []string{kubeletPort} {
			earliestExpiringCert, err = GetAddrEarliestExpiringCert(ip+":"+port, earliestExpiringCert)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"ip":   ip,
					"port": port,
				}).WithError(err).Error("GetEarliestExpiringCert for worker node")
				continue
			}
		}
	}

	return earliestExpiringCert, nil
}
