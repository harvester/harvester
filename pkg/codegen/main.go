package main

import (
	"os"

	controllergen "github.com/rancher/wrangler/pkg/controller-gen"
	"github.com/rancher/wrangler/pkg/controller-gen/args"
	kubevirtv1 "kubevirt.io/client-go/api/v1"
)

func main() {
	os.Unsetenv("GOPATH")
	controllergen.Run(args.Options{
		OutputPackage: "github.com/cnrancher/rancher-vm/pkg/generated",
		Boilerplate:   "scripts/boilerplate.go.txt",
		Groups: map[string]args.Group{
			"vm.cattle.io": {
				Types:           []interface{}{},
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
