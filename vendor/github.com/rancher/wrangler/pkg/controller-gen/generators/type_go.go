package generators

import (
	"fmt"
	"io"
	"strings"

	args2 "github.com/rancher/wrangler/pkg/controller-gen/args"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/gengo/args"
	"k8s.io/gengo/generator"
	"k8s.io/gengo/namer"
	"k8s.io/gengo/types"
)

func TypeGo(gv schema.GroupVersion, name *types.Name, args *args.GeneratorArgs, customArgs *args2.CustomArgs) generator.Generator {
	return &typeGo{
		name:       name,
		gv:         gv,
		args:       args,
		customArgs: customArgs,
		DefaultGen: generator.DefaultGen{
			OptionalName: strings.ToLower(name.Name),
		},
	}
}

type typeGo struct {
	generator.DefaultGen

	name       *types.Name
	gv         schema.GroupVersion
	args       *args.GeneratorArgs
	customArgs *args2.CustomArgs
}

func (f *typeGo) Imports(context *generator.Context) []string {
	packages := append(Imports,
		fmt.Sprintf("%s \"%s\"", f.gv.Version, f.name.Package))

	return packages
}

func (f *typeGo) Init(c *generator.Context, w io.Writer) error {
	sw := generator.NewSnippetWriter(w, c, "{{", "}}")

	if err := f.DefaultGen.Init(c, w); err != nil {
		return err
	}

	t := c.Universe.Type(*f.name)
	m := map[string]interface{}{
		"type":       f.name.Name,
		"lowerName":  namer.IL(f.name.Name),
		"plural":     plural.Name(t),
		"version":    f.gv.Version,
		"namespaced": namespaced(t),
		"hasStatus":  hasStatus(t),
		"statusType": statusType(t),
	}

	sw.Do(typeBody, m)
	return sw.Error()
}

func statusType(t *types.Type) string {
	for _, m := range t.Members {
		if m.Name == "Status" {
			return m.Type.Name.Name
		}
	}
	return ""
}

func hasStatus(t *types.Type) bool {
	for _, m := range t.Members {
		if m.Name == "Status" && m.Type.Name.Package == t.Name.Package {
			return true
		}
	}
	return false
}

var typeBody = `
// {{.type}}Controller interface for managing {{.type}} resources.
type {{.type}}Controller interface {
    generic.{{ if not .namespaced}}NonNamespaced{{end}}ControllerInterface[*{{.version}}.{{.type}}, *{{.version}}.{{.type}}List]
}

// {{.type}}Client interface for managing {{.type}} resources in Kubernetes.
type {{.type}}Client interface {
	generic.{{ if not .namespaced}}NonNamespaced{{end}}ClientInterface[*{{.version}}.{{.type}}, *{{.version}}.{{.type}}List]
}

// {{.type}}Cache interface for retrieving {{.type}} resources in memory.
type {{.type}}Cache interface {
	generic.{{ if not .namespaced}}NonNamespaced{{end}}CacheInterface[*{{.version}}.{{.type}}]
}

{{ if .hasStatus -}}
type {{.type}}StatusHandler func(obj *{{.version}}.{{.type}}, status {{.version}}.{{.statusType}}) ({{.version}}.{{.statusType}}, error)

type {{.type}}GeneratingHandler func(obj *{{.version}}.{{.type}}, status {{.version}}.{{.statusType}}) ([]runtime.Object, {{.version}}.{{.statusType}}, error)

func Register{{.type}}StatusHandler(ctx context.Context, controller {{.type}}Controller, condition condition.Cond, name string, handler {{.type}}StatusHandler) {
	statusHandler := &{{.lowerName}}StatusHandler{
		client:    controller,
		condition: condition,
		handler:   handler,
	}
	controller.AddGenericHandler(ctx, name, generic.FromObjectHandlerToHandler(statusHandler.sync))
}

func Register{{.type}}GeneratingHandler(ctx context.Context, controller {{.type}}Controller, apply apply.Apply,
	condition condition.Cond, name string, handler {{.type}}GeneratingHandler, opts *generic.GeneratingHandlerOptions) {
	statusHandler := &{{.lowerName}}GeneratingHandler{
		{{.type}}GeneratingHandler: handler,
		apply:                            apply,
		name:                             name,
		gvk:                              controller.GroupVersionKind(),
	}
	if opts != nil {
		statusHandler.opts = *opts
	}
	controller.OnChange(ctx, name, statusHandler.Remove)
	Register{{.type}}StatusHandler(ctx, controller, condition, name, statusHandler.Handle)
}

type {{.lowerName}}StatusHandler struct {
	client    {{.type}}Client
	condition condition.Cond
	handler   {{.type}}StatusHandler
}

func (a *{{.lowerName}}StatusHandler) sync(key string, obj *{{.version}}.{{.type}}) (*{{.version}}.{{.type}}, error) {
	if obj == nil {
		return obj, nil
	}

	origStatus := obj.Status.DeepCopy()
	obj = obj.DeepCopy()
	newStatus, err := a.handler(obj, obj.Status)
	if err != nil {
		// Revert to old status on error
		newStatus = *origStatus.DeepCopy()
	}

	if a.condition != "" {
		if errors.IsConflict(err) {
			a.condition.SetError(&newStatus, "", nil)
		} else {
			a.condition.SetError(&newStatus, "", err)
		}
	}
	if !equality.Semantic.DeepEqual(origStatus, &newStatus) {
		if a.condition != "" {
			// Since status has changed, update the lastUpdatedTime
			a.condition.LastUpdated(&newStatus, time.Now().UTC().Format(time.RFC3339))
		}

		var newErr error
		obj.Status = newStatus
		newObj, newErr := a.client.UpdateStatus(obj)
		if err == nil {
			err = newErr
		}
		if newErr == nil {
			obj = newObj
		}
	}
	return obj, err
}

type {{.lowerName}}GeneratingHandler struct {
	{{.type}}GeneratingHandler
	apply apply.Apply
	opts  generic.GeneratingHandlerOptions
	gvk   schema.GroupVersionKind
	name  string
}

func (a *{{.lowerName}}GeneratingHandler) Remove(key string, obj *{{.version}}.{{.type}}) (*{{.version}}.{{.type}}, error) {
	if obj != nil {
		return obj, nil
	}

	obj = &{{.version}}.{{.type}}{}
	obj.Namespace, obj.Name = kv.RSplit(key, "/")
	obj.SetGroupVersionKind(a.gvk)

	return nil, generic.ConfigureApplyForObject(a.apply, obj, &a.opts).
		WithOwner(obj).
		WithSetID(a.name).
		ApplyObjects()
}

func (a *{{.lowerName}}GeneratingHandler) Handle(obj *{{.version}}.{{.type}}, status {{.version}}.{{.statusType}}) ({{.version}}.{{.statusType}}, error) {
	if !obj.DeletionTimestamp.IsZero() {
		return status, nil
	}

	objs, newStatus, err := a.{{.type}}GeneratingHandler(obj, status)
	if err != nil {
		return newStatus, err
	}

	return newStatus, generic.ConfigureApplyForObject(a.apply, obj, &a.opts).
		WithOwner(obj).
		WithSetID(a.name).
		ApplyObjects(objs...)
}
{{- end }}
`
