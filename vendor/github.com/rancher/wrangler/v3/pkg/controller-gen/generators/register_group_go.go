package generators

import (
	"fmt"
	"strings"

	"github.com/rancher/wrangler/v3/pkg/controller-gen/args"
	"k8s.io/gengo/v2/generator"
)

func RegisterGroupGo(group string, customArgs *args.CustomArgs) generator.Generator {
	return &registerGroupGo{
		group:      group,
		customArgs: customArgs,
		GoGenerator: generator.GoGenerator{
			OutputFilename: "zz_generated_register.go",
		},
	}
}

type registerGroupGo struct {
	generator.GoGenerator

	group      string
	customArgs *args.CustomArgs
}

func (f *registerGroupGo) Name() string {
	// Keep the old behavior of generating comments without the .go suffix
	return strings.TrimSuffix(f.Filename(), ".go")
}

func (f *registerGroupGo) PackageConsts(*generator.Context) []string {
	return []string{
		fmt.Sprintf("GroupName = \"%s\"", f.group),
	}
}
