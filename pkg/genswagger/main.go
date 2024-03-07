package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"unicode"

	"github.com/gobuffalo/flect"
	_ "github.com/openshift/api/operator/v1"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"k8s.io/kube-openapi/pkg/builder3"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/common/restfuladapter"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
	_ "kubevirt.io/api/snapshot/v1alpha1"
	_ "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	_ "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/genswagger/rest"
)

const (
	defTimeKey   = "k8s.io.v1.Time"
	defTimeValue = ""
)

var outputFile = flag.String("output", "api/openapi-spec/swagger.json", "Output file.") // TODO: rename to openapi.json

var vowels = map[rune]bool{
	'a': true,
	'e': true,
	'i': true,
	'o': true,
	'u': true,
}

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
	swagger, err := builder3.BuildOpenAPISpecFromRoutes(restfuladapter.AdaptWebServices(webServices), config)
	if err != nil {
		log.Fatal(err.Error())
	}
	addSummaryForMethods(swagger)
	fixedTime(swagger)
	jsonBytes, err := json.MarshalIndent(swagger, "", "  ")
	if err != nil {
		log.Fatal(err.Error())
	}
	if err := os.WriteFile(*outputFile, jsonBytes, 0644); err != nil {
		log.Fatal(err.Error())
	}
}

func createConfig() *common.OpenAPIV3Config {
	return &common.OpenAPIV3Config{
		CommonResponses: map[int]*spec3.Response{
			401: {
				ResponseProps: spec3.ResponseProps{
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
		SecuritySchemes: map[string]*spec3.SecurityScheme{
			"Basic": {
				SecuritySchemeProps: spec3.SecuritySchemeProps{
					Type:   "http",
					Scheme: "basic",
				},
			},
			"Bearer": {
				SecuritySchemeProps: spec3.SecuritySchemeProps{
					Type:   "http",
					Scheme: "bearer",
				},
			},
		},
	}
}

func addSummaryForMethods(swagger *spec3.OpenAPI) {
	// t := reflect.TypeOf(new(spec3.Operation))
	for _, path := range swagger.Paths.Paths {
		pathItemProps := path.PathProps
		// all operations (see the definition of path.PathProps):
		ops := []*spec3.Operation{
			pathItemProps.Get,
			pathItemProps.Put,
			pathItemProps.Post,
			pathItemProps.Delete,
			pathItemProps.Options,
			pathItemProps.Head,
			pathItemProps.Patch,
			pathItemProps.Trace,
		}
		for _, op := range ops {
			if op == nil {
				continue
			}
			if op.Summary == "" {
				op.Summary = splitAndTitle(op.OperationProps.OperationId)
			}
		}
	}
}

func splitAndTitle(s string) string {
	var result []string
	var currentWord string

	hasPlural := strings.HasPrefix(strings.ToLower(s), "list")

	for _, char := range s {
		if unicode.IsUpper(char) && currentWord != "" {
			if strings.ToLower(currentWord) == "for" && hasPlural {
				result[len(result)-1] = flect.Pluralize(result[len(result)-1])
			}
			result = append(result, toTitle(currentWord))
			currentWord = ""
		}
		currentWord += string(char)
	}

	if currentWord != "" {
		if hasPlural {
			currentWord = flect.Pluralize(currentWord)
		}
		result = append(result, toTitle(currentWord))
	}

	if !hasPlural {
		result = indef(result)
	}

	return strings.Join(result, " ")
}

func toTitle(s string) string {
	return cases.Title(language.Und, cases.NoLower).String(s)
}

func indef(s []string) []string {
	a := "a"
	if vowels[rune(strings.ToLower(s[1])[0])] {
		a = "an"
	}
	return append([]string{s[0], a}, s[1:]...)
}

func fixedTime(openapi *spec3.OpenAPI) {
	d := openapi.Components.Schemas[defTimeKey] // TODO: check if this schema exists, and if `schemas` is the right place to look
	d.SchemaProps.Format = ""
	d.SchemaProps.Default = defTimeValue

	openapi.Components.Schemas[defTimeKey] = d
}
