package main

import (
	"os"

	controllergen "github.com/rancher/wrangler/pkg/controller-gen"
	"github.com/rancher/wrangler/pkg/controller-gen/args"
	kubevirtv1 "kubevirt.io/client-go/api/v1alpha3"
	cdiv1 "kubevirt.io/containerized-data-importer/pkg/apis/core/v1beta1"

	harv1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
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
					kubevirtv1.VirtualMachineInstanceMigration{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
			cdiv1.SchemeGroupVersion.Group: {
				Types: []interface{}{
					cdiv1.DataVolume{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
		},
	})
}
