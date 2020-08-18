package main

import (
	"os"

	harv1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	controllergen "github.com/rancher/wrangler/pkg/controller-gen"
	"github.com/rancher/wrangler/pkg/controller-gen/args"
	kubevirtv1 "kubevirt.io/client-go/api/v1alpha3"
)

func main() {
	os.Unsetenv("GOPATH")
	controllergen.Run(args.Options{
		OutputPackage: "github.com/rancher/harvester/pkg/generated",
		Boilerplate:   "scripts/boilerplate.go.txt",
		Groups: map[string]args.Group{
			"harvester.cattle.io": {
				Types: []interface{}{
					harv1.VirtualMachineImage{},
					harv1.Setting{},
					harv1.KeyPair{},
					harv1.VirtualMachineTemplate{},
					harv1.VirtualMachineTemplateVersion{},
				},
				GenerateTypes:   true,
				GenerateClients: true,
			},
			kubevirtv1.GroupName: {
				Types: []interface{}{
					kubevirtv1.VirtualMachine{},
					kubevirtv1.VirtualMachineInstance{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
		},
	})
}
