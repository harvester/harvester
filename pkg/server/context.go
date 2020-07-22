package server

import (
	vmv1 "github.com/rancher/vm/pkg/apis/vm.cattle.io/v1alpha1"
	"github.com/rancher/wrangler/pkg/schemes"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubevirtv1 "kubevirt.io/client-go/api/v1alpha3"
)

var (
	localSchemeBuilder = runtime.SchemeBuilder{
		vmv1.AddToScheme,
		kubevirtv1.AddToScheme,
	}
	AddToScheme = localSchemeBuilder.AddToScheme
	Scheme      = runtime.NewScheme()
)

func init() {
	utilruntime.Must(AddToScheme(Scheme))
	utilruntime.Must(schemes.AddToScheme(Scheme))
}
