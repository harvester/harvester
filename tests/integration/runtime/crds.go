package runtime

import (
	"context"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	mgmtv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	wcrd "github.com/rancher/wrangler/pkg/crd"
	"k8s.io/client-go/rest"

	"github.com/harvester/harvester/pkg/util/crd"
)

// createCRDs creates CRDs needed in integration tests
func createCRDs(ctx context.Context, restConfig *rest.Config) error {
	factory, err := crd.NewFactoryFromClient(ctx, restConfig)
	if err != nil {
		return err
	}
	return factory.
		BatchCreateCRDsIfNotExisted(
			createNetworkAttachmentDefinitionCRD(),
			createManagedChartCRD(),
		).
		BatchWait()
}

func createNetworkAttachmentDefinitionCRD() wcrd.CRD {
	nad := crd.FromGV(cniv1.SchemeGroupVersion, "NetworkAttachmentDefinition", cniv1.NetworkAttachmentDefinition{})
	nad.PluralName = "network-attachment-definitions"
	nad.SingularName = "network-attachment-definition"
	return nad
}

func createManagedChartCRD() wcrd.CRD {
	mChart := crd.FromGV(mgmtv3.SchemeGroupVersion, "ManagedChart", mgmtv3.ManagedChart{})
	mChart.PluralName = "managedcharts"
	mChart.SingularName = "managedchart"
	return mChart
}
