/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package generators

import (
	"fmt"
	"path/filepath"
	"strings"

	args "github.com/rancher/wrangler/v3/pkg/controller-gen/args"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/gengo/v2/generator"
	"k8s.io/gengo/v2/types"
)

type ClientGenerator struct {
	Fakes map[string][]string
}

func NewClientGenerator() *ClientGenerator {
	return &ClientGenerator{
		Fakes: make(map[string][]string),
	}
}

// Packages makes the client package definition.
func (cg *ClientGenerator) GetTargets(context *generator.Context, customArgs *args.CustomArgs) []generator.Target {
	generateTypesGroups := map[string]bool{}

	for groupName, group := range customArgs.Options.Groups {
		if group.GenerateTypes {
			generateTypesGroups[groupName] = true
		}
	}

	var (
		packageList []generator.Target
		groups      = map[string]bool{}
	)

	for gv, types := range customArgs.TypesByGroup {
		if !groups[gv.Group] {
			packageList = append(packageList, cg.groupPackage(gv.Group, customArgs))
			if generateTypesGroups[gv.Group] {
				packageList = append(packageList, cg.typesGroupPackage(types[0], gv, customArgs))
			}
		}
		groups[gv.Group] = true
		packageList = append(packageList, cg.groupVersionPackage(gv, customArgs))

		if generateTypesGroups[gv.Group] {
			packageList = append(packageList, cg.typesGroupVersionPackage(types[0], gv, customArgs))
			packageList = append(packageList, cg.typesGroupVersionDocPackage(types[0], gv, customArgs))
		}
	}

	return packageList
}

func (cg *ClientGenerator) typesGroupPackage(name *types.Name, gv schema.GroupVersion, customArgs *args.CustomArgs) generator.Target {
	packagePath := strings.TrimSuffix(name.Package, "/"+gv.Version)
	return Target(customArgs, packagePath, func(context *generator.Context) []generator.Generator {
		return []generator.Generator{
			RegisterGroupGo(gv.Group, customArgs),
		}
	})
}

func (cg *ClientGenerator) typesGroupVersionDocPackage(name *types.Name, gv schema.GroupVersion, customArgs *args.CustomArgs) generator.Target {
	packagePath := name.Package
	p := Target(customArgs, packagePath, func(context *generator.Context) []generator.Generator {
		return []generator.Generator{
			generator.GoGenerator{
				OutputFilename: "doc.go",
			},
			RegisterGroupVersionGo(gv, customArgs),
			ListTypesGo(gv, customArgs),
		}
	})

	openAPIDirective := ""
	if customArgs.Options.Groups[gv.Group].GenerateOpenAPI {
		openAPIDirective = fmt.Sprintf("\n// +k8s:openapi-gen=true")
	}

	p.HeaderComment = []byte(fmt.Sprintf(`
%s
%s
// +k8s:deepcopy-gen=package
// +groupName=%s
`, string(customArgs.BoilerplateContent), openAPIDirective, gv.Group))

	return p
}

func (cg *ClientGenerator) typesGroupVersionPackage(name *types.Name, gv schema.GroupVersion, customArgs *args.CustomArgs) generator.Target {
	packagePath := name.Package
	return Target(customArgs, packagePath, func(context *generator.Context) []generator.Generator {
		return []generator.Generator{
			RegisterGroupVersionGo(gv, customArgs),
			ListTypesGo(gv, customArgs),
		}
	})
}

func (cg *ClientGenerator) groupPackage(group string, customArgs *args.CustomArgs) generator.Target {
	packagePath := filepath.Join(customArgs.Package, "controllers", groupPackageName(group, customArgs.Options.Groups[group].OutputControllerPackageName))
	return Target(customArgs, packagePath, func(context *generator.Context) []generator.Generator {
		return []generator.Generator{
			FactoryGo(group, customArgs),
			GroupInterfaceGo(group, customArgs),
		}
	})
}

func (cg *ClientGenerator) groupVersionPackage(gv schema.GroupVersion, customArgs *args.CustomArgs) generator.Target {
	packagePath := filepath.Join(customArgs.Package, "controllers", groupPackageName(gv.Group, customArgs.Options.Groups[gv.Group].OutputControllerPackageName), gv.Version)

	return Target(customArgs, packagePath, func(context *generator.Context) []generator.Generator {
		generators := []generator.Generator{
			GroupVersionInterfaceGo(gv, customArgs),
		}

		for _, t := range customArgs.TypesByGroup[gv] {
			generators = append(generators, TypeGo(gv, t, customArgs))
			cg.Fakes[packagePath] = append(cg.Fakes[packagePath], t.Name)
		}

		return generators
	})
}
