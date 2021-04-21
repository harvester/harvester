package server

import (
	"github.com/rancher/wrangler/pkg/schemes"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubevirtv1 "kubevirt.io/client-go/api/v1"
	cdiv1 "kubevirt.io/containerized-data-importer/pkg/apis/core/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

var (
	localSchemeBuilder = runtime.SchemeBuilder{
		harvesterv1.AddToScheme,
		kubevirtv1.AddToScheme,
		cdiv1.AddToScheme,
	}
	AddToScheme = localSchemeBuilder.AddToScheme
	Scheme      = runtime.NewScheme()
)

func init() {
	utilruntime.Must(AddToScheme(Scheme))
	utilruntime.Must(schemes.AddToScheme(Scheme))
}
