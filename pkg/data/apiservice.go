package data

import (
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/apply"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	AggregationSecretName = "harvester-aggregation"
)

func addAPIService(apply apply.Apply, namespace string) error {
	return apply.
		WithDynamicLookup().
		WithSetID("harvester-apiservice").
		ApplyObjects(&v3.APIService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "harvester",
				Namespace: namespace,
			},
			Spec: v3.APIServiceSpec{
				SecretName:      AggregationSecretName,
				SecretNamespace: namespace,
				PathPrefixes:    []string{"/v1/harvester/", "/dashboard/"},
				Paths:           []string{"/v1/harvester"},
			},
		})
}
