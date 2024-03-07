package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/emicklei/go-restful/v3"
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
	webServices := rest.AggregatedWebServices()
	config := createConfig(webServices)
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

func allOperations(path *spec3.Path) []*spec3.Operation {
	// all operations (see the definition of path.PathProps):
	return []*spec3.Operation{
		path.Get,
		path.Put,
		path.Post,
		path.Delete,
		path.Options,
		path.Head,
		path.Patch,
		path.Trace,
	}
}

func createConfig(webServices []*restful.WebService) *common.OpenAPIV3Config {
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
			// see https://spec.openapis.org/oas/v3.0.3#security-scheme-object
			"Basic": {
				SecuritySchemeProps: spec3.SecuritySchemeProps{
					Type:   "http",
					Scheme: "Basic",
				},
			},
			"Bearer": {
				SecuritySchemeProps: spec3.SecuritySchemeProps{
					Type:   "http",
					Scheme: "Bearer",
				},
			},
		},
		PostProcessSpec: func(swagger *spec3.OpenAPI) (*spec3.OpenAPI, error) {
			// find all the schemas in the spec and clear uniqueItems for non-array types
			for _, path := range swagger.Paths.Paths {
				for _, param := range path.Parameters {
					if param == nil {
						continue
					}
					if schema := param.Schema; schema != nil {
						if !schema.Type.Contains("array") {
							schema.UniqueItems = false
						}
					}
				}
				for _, op := range allOperations(path) {
					if op == nil {
						continue
					}
					// find all the schemas in the spec
					for _, param := range op.Parameters {
						if param != nil {
							if schema := param.Schema; schema != nil {
								if !schema.Type.Contains("array") {
									schema.UniqueItems = false
								}
							}
						}
					}
				}
			}
			// add pattern validation, mins, maxes to the generated spec
			for _, service := range webServices {
				if service == nil {
					continue
				}
				_ = service.PathParameters()
				routes := service.Routes()
				for _, route := range routes {
					if path, ok := swagger.Paths.Paths[route.Path]; ok && path != nil {
						var op *spec3.Operation
						switch route.Method {
						case "GET":
							op = path.Get
						case "POST":
							op = path.Post
						case "PUT":
							op = path.Put
						case "DELETE":
							op = path.Delete
						case "PATCH":
							op = path.Patch
						case "OPTIONS":
							op = path.Options
						case "HEAD":
							op = path.Head
						case "TRACE":
							op = path.Trace
						default:
							log.Panicf("webServices path `%s` has unknown operation `%s %s`", route.Path, route.Method, route.Path)
						}
						if op == nil {
							log.Panicf("webServices path `%s %s` is missing from generated *spec3.OpenAPI", route.Method, route.Path)
						}

						serviceParams := route.ParameterDocs
						for _, serviceParam := range serviceParams {
							if serviceParam == nil {
								continue
							}
							serviceParam := serviceParam.Data()
							if serviceParam.Name == "body" {
								continue // don't handle the request body
							}
							// find the corresponding spec3.Parameter by searching first
							// in the path and then in the operation parameters
							var specParam *spec3.Parameter
							candidates := append(path.Parameters, op.Parameters...)

							for _, candidate := range candidates {
								if candidate == nil {
									continue
								}
								if candidate.Name == serviceParam.Name {
									switch serviceParam.Kind {
									case restful.PathParameterKind:
										if candidate.In == "path" {
											specParam = candidate
											break
										}
									case restful.QueryParameterKind:
										if candidate.In == "query" {
											specParam = candidate
											break
										}
									case restful.BodyParameterKind:
										if candidate.In == "body" {
											specParam = candidate
											break
										}
									case restful.HeaderParameterKind:
										if candidate.In == "header" {
											specParam = candidate
											break
										}
									// we only care about these 3 kinds, though we might also care about cookies
									case restful.FormParameterKind:
									case restful.MultiPartFormParameterKind:
									default:
										log.Panicf("unexpected service kind `%d`", serviceParam.Kind)
									}
								}
							}
							if specParam == nil {
								log.Panicf("webServices route `%s %s` has no parameter `%s` in generated *spec3.OpenAPI", route.Method, route.Path, serviceParam.Name)
							}

							if specParam == nil {
								log.Panicf(
									"webServices route `%s %s` has no parameter `%s` in generated *spec3.OpenAPI",
									route.Method, route.Path, serviceParam.Name)
							}

							schema := specParam.Schema
							if schema == nil {
								// schema = &spec.Schema{}
								// specParam.Schema = schema
								log.Panicf(
									"Nil schema for parameter `%s` in route `%s %s`",
									serviceParam.Name, route.Operation, route.Path)
							}
							switch serviceParam.DataType {
							case "string":
								// propagate pattern, if present
								if serviceParam.Pattern != "" {
									schema.Pattern = serviceParam.Pattern
								}
							case "integer":
								// propagate min, max, if present
								if serviceParam.Minimum != nil {
									schema.Minimum = serviceParam.Minimum
								}
								if serviceParam.Maximum != nil {
									schema.Maximum = serviceParam.Maximum
								}
							}
						}

					} else {
						log.Panicf("webServices path `%s` not found in generated *spec3.OpenAPI", route.Path)
					}
				}
			}

			// clean up invalid component(?)
			for _, schema := range swagger.Components.Schemas {
				if schema == nil {
					continue
				}
				if schema.Required != nil {
					// get unique strings
					counts := make(map[string]uint8)
					for _, s := range schema.Required {
						counts[s]++
					}
					required := make([]string, 0, len(counts))
					for requiredProp := range counts {
						required = append(required, requiredProp)
					}
					sort.Strings(required)
					schema.Required = required
				}
			}

			return swagger, nil
		},
	}
}

func addSummaryForMethods(swagger *spec3.OpenAPI) {
	// t := reflect.TypeOf(new(spec3.Operation))
	for _, path := range swagger.Paths.Paths {
		if path == nil {
			continue
		}
		ops := allOperations(path)
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
