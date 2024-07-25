package generators

import (
	"path/filepath"
	"strings"

	args "github.com/rancher/wrangler/v3/pkg/controller-gen/args"
	"k8s.io/gengo/v2/generator"
)

func Target(customArgs *args.CustomArgs, name string, generators func(context *generator.Context) []generator.Generator) generator.SimpleTarget {
	parts := strings.Split(name, "/")
	return generator.SimpleTarget{
		PkgName:        groupPath(parts[len(parts)-1]),
		PkgPath:        name,
		PkgDir:         filepath.Join(customArgs.OutputBase, name),
		HeaderComment:  customArgs.BoilerplateContent,
		GeneratorsFunc: generators,
	}
}
