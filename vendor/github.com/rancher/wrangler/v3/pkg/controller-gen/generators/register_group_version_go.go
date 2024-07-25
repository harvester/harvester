package generators

import (
	"fmt"
	"io"
	"strings"

	"github.com/rancher/wrangler/v3/pkg/controller-gen/args"
	"github.com/rancher/wrangler/v3/pkg/name"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/gengo/v2/generator"
	"k8s.io/gengo/v2/namer"
	"k8s.io/gengo/v2/types"
)

func RegisterGroupVersionGo(gv schema.GroupVersion, customArgs *args.CustomArgs) generator.Generator {
	return &registerGroupVersionGo{
		gv:         gv,
		customArgs: customArgs,
		GoGenerator: generator.GoGenerator{
			OutputFilename: "zz_generated_register.go",
		},
	}
}

type registerGroupVersionGo struct {
	generator.GoGenerator

	gv         schema.GroupVersion
	customArgs *args.CustomArgs
}

func (f *registerGroupVersionGo) Imports(*generator.Context) []string {
	firstType := f.customArgs.TypesByGroup[f.gv][0]
	typeGroupPath := strings.TrimSuffix(firstType.Package, "/"+f.gv.Version)

	packages := append(Imports,
		fmt.Sprintf("%s \"%s\"", groupPath(f.gv.Group), typeGroupPath))

	return packages
}

func (f *registerGroupVersionGo) Init(c *generator.Context, w io.Writer) error {
	var (
		types   []*types.Type
		orderer = namer.Orderer{Namer: namer.NewPrivateNamer(0)}
		sw      = generator.NewSnippetWriter(w, c, "{{", "}}")
	)

	for _, name := range f.customArgs.TypesByGroup[f.gv] {
		types = append(types, c.Universe.Type(*name))
	}
	types = orderer.OrderTypes(types)

	m := map[string]interface{}{
		"version":   f.gv.Version,
		"groupPath": groupPath(f.gv.Group),
	}

	sw.Do("var (\n", nil)
	for _, t := range types {
		m := map[string]interface{}{
			"name":   t.Name.Name + "ResourceName",
			"plural": name.GuessPluralName(strings.ToLower(t.Name.Name)),
		}

		sw.Do("{{.name}} = \"{{.plural}}\"\n", m)
	}
	sw.Do(")\n", nil)

	sw.Do(registerGroupVersionBody, m)

	for _, t := range types {
		m := map[string]interface{}{
			"type": t.Name.Name,
		}

		sw.Do("&{{.type}}{},\n", m)
		sw.Do("&{{.type}}List{},\n", m)
	}

	sw.Do(registerGroupVersionBodyEnd, nil)

	return sw.Error()
}

var registerGroupVersionBody = `
// SchemeGroupVersion is group version used to register these objects
var SchemeGroupVersion = schema.GroupVersion{Group: {{.groupPath}}.GroupName, Version: "{{.version}}"}

// Kind takes an unqualified kind and returns back a Group qualified GroupKind
func Kind(kind string) schema.GroupKind {
	return SchemeGroupVersion.WithKind(kind).GroupKind()
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

// Adds the list of known types to Scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
`

var registerGroupVersionBodyEnd = `
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
`
