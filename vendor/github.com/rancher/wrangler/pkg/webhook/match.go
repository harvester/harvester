package webhook

import (
	v1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type RouteMatch struct {
	handler     Handler
	kind        string
	resource    string
	version     string
	subResource string
	dryRun      *bool
	group       string
	name        string
	namespace   string
	operation   v1.Operation
	objType     runtime.Object
}

func (r *RouteMatch) admit(response *Response, request *Request) error {
	if r.handler != nil {
		return r.handler.Admit(response, request)
	}
	return nil
}

func (r *RouteMatch) matches(req *v1.AdmissionRequest) bool {
	var (
		group, version, kind, resource string
	)
	if req.RequestKind != nil {
		group, version, kind = req.RequestKind.Group, req.RequestKind.Version, req.RequestKind.Kind
	}
	if req.RequestResource != nil {
		group, version, resource = req.RequestResource.Group, req.RequestResource.Version, req.RequestResource.Resource
	}

	return checkString(r.kind, kind) &&
		checkString(r.resource, resource) &&
		checkString(r.subResource, req.SubResource) &&
		checkString(r.version, version) &&
		checkString(r.group, group) &&
		checkString(r.name, req.Name) &&
		checkString(r.namespace, req.Namespace) &&
		checkString(string(r.operation), string(req.Operation)) &&
		checkBool(r.dryRun, req.DryRun)
}

func (r *RouteMatch) getObjType() runtime.Object {
	if r.objType == nil {
		return defObjType
	}
	return r.objType
}

func checkString(expected, actual string) bool {
	if expected == "" {
		return true
	}
	return expected == actual
}

func checkBool(expected, actual *bool) bool {
	if expected == nil {
		return true
	}
	if actual == nil {
		return false
	}
	return *expected == *actual
}

// Pretty methods

func (r *RouteMatch) DryRun(dryRun bool) *RouteMatch               { r.dryRun = &dryRun; return r }
func (r *RouteMatch) Group(group string) *RouteMatch               { r.group = group; return r }
func (r *RouteMatch) HandleFunc(handler HandlerFunc) *RouteMatch   { r.handler = handler; return r }
func (r *RouteMatch) Handle(handler Handler) *RouteMatch           { r.handler = handler; return r }
func (r *RouteMatch) Kind(kind string) *RouteMatch                 { r.kind = kind; return r }
func (r *RouteMatch) Name(name string) *RouteMatch                 { r.name = name; return r }
func (r *RouteMatch) Namespace(namespace string) *RouteMatch       { r.namespace = namespace; return r }
func (r *RouteMatch) Operation(operation v1.Operation) *RouteMatch { r.operation = operation; return r }
func (r *RouteMatch) Resource(resource string) *RouteMatch         { r.resource = resource; return r }
func (r *RouteMatch) SubResource(sr string) *RouteMatch            { r.subResource = sr; return r }
func (r *RouteMatch) Type(objType runtime.Object) *RouteMatch      { r.objType = objType; return r }
func (r *RouteMatch) Version(version string) *RouteMatch           { r.version = version; return r }

// Wrappers for pretty methods

func (r *Router) DryRun(dryRun bool) *RouteMatch               { return r.next().DryRun(dryRun) }
func (r *Router) Group(group string) *RouteMatch               { return r.next().Group(group) }
func (r *Router) HandleFunc(hf HandlerFunc) *RouteMatch        { return r.next().HandleFunc(hf) }
func (r *Router) Handle(handler Handler) *RouteMatch           { return r.next().Handle(handler) }
func (r *Router) Kind(kind string) *RouteMatch                 { return r.next().Kind(kind) }
func (r *Router) Name(name string) *RouteMatch                 { return r.next().Name(name) }
func (r *Router) Namespace(namespace string) *RouteMatch       { return r.next().Namespace(namespace) }
func (r *Router) Operation(operation v1.Operation) *RouteMatch { return r.next().Operation(operation) }
func (r *Router) Resource(resource string) *RouteMatch         { return r.next().Resource(resource) }
func (r *Router) SubResource(subResource string) *RouteMatch {
	return r.next().SubResource(subResource)
}
func (r *Router) Type(objType runtime.Object) *RouteMatch { return r.next().Type(objType) }
func (r *Router) Version(version string) *RouteMatch      { return r.next().Version(version) }
