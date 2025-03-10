package util

import (
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetKubernetesIps(endpointCache ctlcorev1.EndpointsCache) ([]string, error) {
	endpoints, err := endpointCache.Get(metav1.NamespaceDefault, "kubernetes")
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"namespace": metav1.NamespaceDefault,
			"name":      "kubernetes",
		}).WithError(err).Error("endpointCache.Get")
		return nil, err
	}

	var ips []string
	for _, subset := range endpoints.Subsets {
		for _, address := range subset.Addresses {
			ips = append(ips, address.IP+":6443")
		}
	}

	return ips, nil
}
