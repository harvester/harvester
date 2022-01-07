package runtime

import (
	"context"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	catalogv1 "github.com/rancher/rancher/pkg/apis/catalog.cattle.io/v1"
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
			createHelmChartConfigCRD(),
			createNetworkAttachmentDefinitionCRD(),
			createManagedChartCRD(),
			createAppCRD(),
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

func createHelmChartConfigCRD() wcrd.CRD {
	mChart := crd.FromGV(helmv1.SchemeGroupVersion, "HelmChartConfig", helmv1.HelmChartConfig{})
	mChart.PluralName = "helmchartconfigs"
	mChart.SingularName = "helmchartconfig"
	return mChart
}

func createAppCRD() wcrd.CRD {
	app := crd.FromGV(catalogv1.SchemeGroupVersion, "App", catalogv1.App{})
	app.PluralName = "apps"
	app.SingularName = "app"
	return app
}
