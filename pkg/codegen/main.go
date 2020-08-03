package main

import (
	"os"

	vmv1 "github.com/rancher/vm/pkg/apis/vm.cattle.io/v1alpha1"
	controllergen "github.com/rancher/wrangler/pkg/controller-gen"
	"github.com/rancher/wrangler/pkg/controller-gen/args"
	kubevirtv1 "kubevirt.io/client-go/api/v1alpha3"
)

func main() {
	os.Unsetenv("GOPATH")
	controllergen.Run(args.Options{
		OutputPackage: "github.com/rancher/vm/pkg/generated",
		Boilerplate:   "scripts/boilerplate.go.txt",
		Groups: map[string]args.Group{
			"vm.cattle.io": {
				Types: []interface{}{
					vmv1.Image{},
					vmv1.Setting{},
					vmv1.KeyPair{},
				},
				GenerateTypes:   true,
				GenerateClients: true,
			},
			kubevirtv1.GroupName: {
				Types: []interface{}{
					kubevirtv1.VirtualMachineInstance{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
		},
	})
}
