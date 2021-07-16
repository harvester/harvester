package data

import (
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/config"
)

const (
	AggregationSecretName = "harvester-aggregation"
)

func createAPIService(mgmtCtx *config.Management, namespace string) error {
	apiServices := mgmtCtx.RancherManagementFactory.Management().V3().APIService()
	harvesterAggregationAPI := &v3.APIService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "harvester",
			Namespace: namespace,
		},
		Spec: v3.APIServiceSpec{
			SecretName:      AggregationSecretName,
			SecretNamespace: namespace,
			PathPrefixes:    []string{"/v1/harvester/"},
			Paths:           []string{"/v1/harvester"},
		},
	}
	if _, err := apiServices.Create(harvesterAggregationAPI); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}
