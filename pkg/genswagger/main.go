package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"strings"

	"github.com/emicklei/go-restful"
	"github.com/go-openapi/spec"
	"k8s.io/kube-openapi/pkg/builder"
	"k8s.io/kube-openapi/pkg/common"
	_ "kubevirt.io/client-go/apis/snapshot/v1alpha1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/genswagger/rest"
)

var outputFile = flag.String("output", "api/openapi-spec/swagger.json", "Output file.")

// Generate OpenAPI spec definitions for Harvester Resource
func main() {
	flag.Parse()
	config := createConfig()
	webServices := rest.AggregatedAPIs()
	swagger, err := builder.BuildOpenAPISpec(webServices, config)
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
			m := v1beta1.GetOpenAPIDefinitions(ref)
			rest.AddV3OpenAPIDefinitions(m, ref)
			return m
		},

		GetDefinitionName: func(name string) (string, spec.Extensions) {
			//adapting k8s style
			name = strings.ReplaceAll(name, "github.com/harvester/harvester/pkg/apis/harvesterhci.io", "harvesterhci.io")
			name = strings.ReplaceAll(name, "k8s.io/api/core", "k8s.io")
			name = strings.ReplaceAll(name, "k8s.io/apimachinery/pkg/apis/meta", "k8s.io")
			name = strings.ReplaceAll(name, "kubevirt.io/client-go/api", "kubevirt.io")
			name = strings.ReplaceAll(name, "kubevirt.io/containerized-data-importer/pkg/apis/core", "cdi.kubevirt.io")
			name = strings.ReplaceAll(name, "github.com/rancher/rancher/pkg/apis/management.cattle.io", "management.cattle.io")
			name = strings.ReplaceAll(name, "/", ".")

			return name, nil
		},
		GetOperationIDAndTags: func(r *restful.Route) (string, []string, error) {
			var apiGroup string
			if strings.HasPrefix(r.Path, "/apis/") {
				splits := strings.Split(r.Path, "/")
				if len(splits) >= 3 {
					apiGroup = splits[2]
				}
			} else if strings.HasPrefix(r.Path, "/v3-public") {
				apiGroup = "v3-public"
			}
			return r.Operation, []string{apiGroup}, nil
		},
	}
}
