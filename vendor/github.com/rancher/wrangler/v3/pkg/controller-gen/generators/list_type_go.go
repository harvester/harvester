package generators

import (
	"io"

	"github.com/rancher/wrangler/v3/pkg/controller-gen/args"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/gengo/v2/generator"
)

func ListTypesGo(gv schema.GroupVersion, customArgs *args.CustomArgs) generator.Generator {
	return &listTypesGo{
		gv:         gv,
		customArgs: customArgs,
		GoGenerator: generator.GoGenerator{
			OutputFilename: "zz_generated_list_types.go",
		},
	}
}

type listTypesGo struct {
	generator.GoGenerator

	gv         schema.GroupVersion
	customArgs *args.CustomArgs
}

func (f *listTypesGo) Imports(*generator.Context) []string {
	return Imports
}

func (f *listTypesGo) Init(c *generator.Context, w io.Writer) error {
	sw := generator.NewSnippetWriter(w, c, "{{", "}}")

	for _, t := range f.customArgs.TypesByGroup[f.gv] {
		m := map[string]interface{}{
			"type": t.Name,
		}
		args.CheckType(c.Universe.Type(*t))
		sw.Do(string(listTypesBody), m)
	}

	return sw.Error()
}

var listTypesBody = `
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// {{.type}}List is a list of {{.type}} resources
type {{.type}}List struct {
	metav1.TypeMeta ` + "`" + `json:",inline"` + "`" + `
	metav1.ListMeta ` + "`" + `json:"metadata"` + "`" + `

	Items []{{.type}} ` + "`" + `json:"items"` + "`" + `
}

func New{{.type}}(namespace, name string, obj {{.type}}) *{{.type}} {
	obj.APIVersion, obj.Kind = SchemeGroupVersion.WithKind("{{.type}}").ToAPIVersionAndKind()
	obj.Name = name
	obj.Namespace = namespace
	return &obj
}
`
