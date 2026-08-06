package util

import (
	"github.com/sirupsen/logrus"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	ctldiscoveryv1 "github.com/harvester/harvester/pkg/generated/controllers/discovery.k8s.io/v1"
)

func GetKubernetesIps(endpointSliceCache ctldiscoveryv1.EndpointSliceCache) ([]string, error) {
	selector := labels.SelectorFromSet(labels.Set{
		discoveryv1.LabelServiceName: "kubernetes",
	})
	endpointSlices, err := endpointSliceCache.List(metav1.NamespaceDefault, selector)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"namespace":     metav1.NamespaceDefault,
			"labelSelector": selector.String(),
		}).WithError(err).Error("endpointSliceCache.List")
		return nil, err
	}

	var ips []string
	seen := map[string]bool{}
	for _, endpointSlice := range endpointSlices {
		if endpointSlice.AddressType != discoveryv1.AddressTypeIPv4 {
			continue
		}
		for _, endpoint := range endpointSlice.Endpoints {
			// nil Ready is interpreted as ready, per the EndpointSlice API
			if endpoint.Conditions.Ready != nil && !*endpoint.Conditions.Ready {
				continue
			}
			for _, address := range endpoint.Addresses {
				if seen[address] {
					continue
				}
				seen[address] = true
				ips = append(ips, address+":6443")
			}
		}
	}

	return ips, nil
}
