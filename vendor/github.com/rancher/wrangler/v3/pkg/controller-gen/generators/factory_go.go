package generators

import (
	"fmt"
	"io"

	"github.com/rancher/wrangler/v3/pkg/controller-gen/args"
	"k8s.io/gengo/v2/generator"
)

func FactoryGo(group string, customArgs *args.CustomArgs) generator.Generator {
	return &factory{
		group:      group,
		customArgs: customArgs,
		GoGenerator: generator.GoGenerator{
			OutputFilename: "factory.go",
			OptionalBody:   []byte(factoryBody),
		},
	}
}

type factory struct {
	generator.GoGenerator

	group      string
	customArgs *args.CustomArgs
}

func (f *factory) Imports(*generator.Context) []string {
	imports := Imports

	for gv, types := range f.customArgs.TypesByGroup {
		if f.group == gv.Group && len(types) > 0 {
			imports = append(imports,
				fmt.Sprintf("%s \"%s\"", gv.Version, types[0].Package))
		}
	}

	return imports
}

func (f *factory) Init(c *generator.Context, w io.Writer) error {
	if err := f.GoGenerator.Init(c, w); err != nil {
		return err
	}

	sw := generator.NewSnippetWriter(w, c, "{{", "}}")
	m := map[string]interface{}{
		"groupName": upperLowercase(f.group),
	}

	sw.Do("\n\nfunc (c *Factory) {{.groupName}}() Interface {\n", m)
	sw.Do("	return New(c.ControllerFactory())\n", m)
	sw.Do("}\n\n", m)

	sw.Do("\n\nfunc (c *Factory) WithAgent(userAgent string) Interface {\n", m)
	sw.Do("	return New(controller.NewSharedControllerFactoryWithAgent(userAgent, c.ControllerFactory()))\n", m)
	sw.Do("}\n\n", m)

	return sw.Error()
}

var factoryBody = `
type Factory struct {
	*generic.Factory
}

func NewFactoryFromConfigOrDie(config *rest.Config) *Factory {
	f, err := NewFactoryFromConfig(config)
	if err != nil {
		panic(err)
	}
	return f
}

func NewFactoryFromConfig(config *rest.Config) (*Factory, error) {
	return NewFactoryFromConfigWithOptions(config, nil)
}

func NewFactoryFromConfigWithNamespace(config *rest.Config, namespace string) (*Factory, error) {
	return NewFactoryFromConfigWithOptions(config, &FactoryOptions{
		Namespace: namespace,
	})
}

type FactoryOptions = generic.FactoryOptions

func NewFactoryFromConfigWithOptions(config *rest.Config, opts *FactoryOptions) (*Factory, error) {
	f, err := generic.NewFactoryFromConfigWithOptions(config, opts)
	return &Factory{
		Factory: f,
	}, err
}

func NewFactoryFromConfigWithOptionsOrDie(config *rest.Config, opts *FactoryOptions) *Factory {
    f, err := NewFactoryFromConfigWithOptions(config, opts)
	if err != nil {
		panic(err)
	}
	return f
}

`
