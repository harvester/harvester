package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"strings"

	_ "github.com/openshift/api/operator/v1"
	"k8s.io/kube-openapi/pkg/builder"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/common/restfuladapter"
	"k8s.io/kube-openapi/pkg/validation/spec"
	_ "kubevirt.io/api/snapshot/v1alpha1"
	_ "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	_ "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/genswagger/rest"
)

var outputFile = flag.String("output", "api/openapi-spec/swagger.json", "Output file.")

var kindToTagMappings = map[string]string{
	"VirtualMachine":                  "Virtual Machines",
	"VirtualMachineInstance":          "Virtual Machines",
	"VirtualMachineTemplate":          "Virtual Machine Templates",
	"VirtualMachineTemplateVersion":   "Virtual Machine Templates",
	"PersistentVolumeClaim":           "Volumes",
	"VirtualMachineImage":             "Images",
	"VirtualMachineBackup":            "Backups",
	"VirtualMachineRestore":           "Restores",
	"VirtualMachineInstanceMigration": "Migrations",
	"KeyPair":                         "SSH Keys",
	"Setting":                         "Settings",
	"SupportBundle":                   "Support Bundles",
	"Upgrade":                         "Upgrades",
	"ClusterNetwork":                  "Networks",
	"NodeNetwork":                     "Networks",
	"NetworkAttachmentDefinition":     "Networks",
}

// Generate OpenAPI spec definitions for Harvester Resource
func main() {
	flag.Parse()
	config := createConfig()
	webServices := rest.AggregatedWebServices()
	swagger, err := builder.BuildOpenAPISpecFromRoutes(restfuladapter.AdaptWebServices(webServices), config)
	if err != nil {
		log.Fatal(err.Error())
	}
	jsonBytes, err := json.MarshalIndent(swagger, "", "  ")
	if err != nil {
		log.Fatal(err.Error())
	}
	if err := ioutil.WriteFile(*outputFile, jsonBytes, 0644); err != nil {
		log.Fatal(err.Error())
	}
}

func createConfig() *common.Config {
	return &common.Config{
		CommonResponses: map[int]spec.Response{
			401: {
				ResponseProps: spec.ResponseProps{
					Description: "Unauthorized",
				},
			},
		},
		Info: &spec.Info{
			InfoProps: spec.InfoProps{
				Title:   "Harvester APIs",
				Version: "v1beta1",
			},
		},
		GetDefinitions: func(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
			return rest.SetDefinitions(v1beta1.GetOpenAPIDefinitions(ref))
		},

		GetDefinitionName: func(name string) (string, spec.Extensions) {
			//adapting k8s style
			name = strings.ReplaceAll(name, "github.com/harvester/harvester/pkg/apis/harvesterhci.io", "harvesterhci.io")
			name = strings.ReplaceAll(name, "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io", "network.harvesterhci.io")
			name = strings.ReplaceAll(name, "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io", "k8s.cni.cncf.io")
			name = strings.ReplaceAll(name, "k8s.io/api/core", "k8s.io")
			name = strings.ReplaceAll(name, "k8s.io/apimachinery/pkg/apis/meta", "k8s.io")
			name = strings.ReplaceAll(name, "kubevirt.io/client-go/api", "kubevirt.io")
			name = strings.ReplaceAll(name, "kubevirt.io/containerized-data-importer/pkg/apis/core", "cdi.kubevirt.io")
			name = strings.ReplaceAll(name, "/", ".")

			return name, nil
		},
		GetOperationIDAndTagsFromRoute: func(r common.Route) (string, []string, error) {
			var tag string
			if _, ok := r.Metadata()["kind"]; ok {
				kind := fmt.Sprint(r.Metadata()["kind"])
				tag = kindToTagMappings[kind]
			}
			return r.OperationName(), []string{tag}, nil
		},
	}
}
