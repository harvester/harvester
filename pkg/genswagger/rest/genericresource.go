package rest

import (
	"fmt"
	"net/http"
	"reflect"

	"github.com/emicklei/go-restful/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	mime "kubevirt.io/kubevirt/pkg/rest"
)

var defaultActions = []Action{
	GetAction,
	GetAll,
	GetNamespacedAll,
	PostAction,
	PutAction,
	PatchAction,
	DeleteAction,
}

var defaultGetActions = []Action{
	GetAction,
	GetAll,
	GetNamespacedAll,
}

type GenericResource struct {
	ws             *restful.WebService
	resource       string
	objPointer     runtime.Object
	objKind        string
	objListPointer runtime.Object
	actions        []Action
	namespaced     bool
	objExample     any
	listExample    any
}

func NewGenericResource(ws *restful.WebService, resource string, objPointer runtime.Object, objKind string, objListPointer runtime.Object, namespaced bool, actions []Action) {
	objExample := reflect.ValueOf(objPointer).Elem().Interface()
	listExample := reflect.ValueOf(objListPointer).Elem().Interface()

	gr := &GenericResource{
		ws:             ws,
		resource:       resource,
		objPointer:     objPointer,
		objKind:        objKind,
		actions:        actions,
		objListPointer: objListPointer,
		namespaced:     namespaced,
		objExample:     objExample,
		listExample:    listExample,
	}

	gr.Build()
}

func (gr *GenericResource) Build() {
	for _, action := range gr.actions {
		builder := action(gr, gr.ws)

		gr.ws.Route(builder)
	}
}

func (gr *GenericResource) addNamespaceParam(builder *restful.RouteBuilder) *restful.RouteBuilder {
	if gr.namespaced {
		builder.Param(NamespaceParam(gr.ws))
	}

	return builder
}

type Action func(gr *GenericResource, ws *restful.WebService) *restful.RouteBuilder

func GetAction(gr *GenericResource, ws *restful.WebService) *restful.RouteBuilder {
	operation := "read" + gr.objKind

	if gr.namespaced {
		operation = "readNamespaced" + gr.objKind
	}

	builder := ws.GET(ResourcePath(gr.resource, gr.namespaced)).
		Produces(mime.MIME_JSON, mime.MIME_YAML, mime.MIME_JSON_STREAM).
		Operation(operation).
		To(Noop).Writes(gr.objExample).
		Doc("Get a "+gr.objKind+" object.").
		Metadata("kind", gr.objKind).
		Returns(http.StatusOK, "OK", gr.objExample).
		Returns(http.StatusUnauthorized, "Unauthorized", "")

	return gr.addNamespaceParam(builder).Param(NameParam(ws)).
		Param(exactParam(ws)).
		Param(exportParam(ws))
}

func GetAll(gr *GenericResource, ws *restful.WebService) *restful.RouteBuilder {
	builder := ws.GET(gr.resource).
		Produces(mime.MIME_JSON, mime.MIME_YAML, mime.MIME_JSON_STREAM).
		Operation("list"+gr.objKind+"ForAllNamespaces").
		To(Noop).Writes(gr.listExample).
		Doc("Get a list of all "+gr.objKind+" objects.").
		Metadata("kind", gr.objKind).
		Returns(http.StatusOK, "OK", gr.listExample).
		Returns(http.StatusUnauthorized, "Unauthorized", "")

	return addCollectionParams(builder, ws)
}

func GetNamespacedAll(gr *GenericResource, ws *restful.WebService) *restful.RouteBuilder {
	operation := "list" + gr.objKind
	doc := fmt.Sprintf("Get a list of all %s objects.", gr.objKind)

	if gr.namespaced {
		doc = fmt.Sprintf("Get a list of %s objects in a namespace.", gr.objKind)
		operation = "listNamespaced" + gr.objKind
	}

	builder := ws.GET(ResourceBasePath(gr.resource, gr.namespaced)).
		Produces(mime.MIME_JSON, mime.MIME_YAML, mime.MIME_JSON_STREAM).
		Operation(operation).
		Writes(gr.listExample).
		To(Noop).
		Doc(doc).
		Metadata("kind", gr.objKind).
		Returns(http.StatusOK, "OK", gr.listExample).
		Returns(http.StatusUnauthorized, "Unauthorized", "")

	return addCollectionParams(gr.addNamespaceParam(builder), ws)
}

func PostAction(gr *GenericResource, ws *restful.WebService) *restful.RouteBuilder {
	operation := "create" + gr.objKind

	if gr.namespaced {
		operation = "createNamespaced" + gr.objKind
	}

	builder := ws.POST(ResourceBasePath(gr.resource, gr.namespaced)).
		Produces(mime.MIME_JSON, mime.MIME_YAML).
		Consumes(mime.MIME_JSON, mime.MIME_YAML).
		Operation(operation).
		To(Noop).Reads(gr.objExample).Writes(gr.objExample).
		Doc("Create a "+gr.objKind+" object.").
		Metadata("kind", gr.objKind).
		Returns(http.StatusOK, "OK", gr.objExample).
		Returns(http.StatusCreated, "Created", gr.objExample).
		Returns(http.StatusAccepted, "Accepted", gr.objExample).
		Returns(http.StatusUnauthorized, "Unauthorized", "")

	return gr.addNamespaceParam(builder)
}

func PutAction(gr *GenericResource, ws *restful.WebService) *restful.RouteBuilder {
	operation := "replace" + gr.objKind

	if gr.namespaced {
		operation = "replaceNamespaced" + gr.objKind
	}

	builder := ws.PUT(ResourcePath(gr.resource, gr.namespaced)).
		Produces(mime.MIME_JSON, mime.MIME_YAML).
		Consumes(mime.MIME_JSON, mime.MIME_YAML).
		Operation(operation).
		To(Noop).Reads(gr.objExample).Writes(gr.objExample).
		Doc("Update a "+gr.objKind+" object.").
		Metadata("kind", gr.objKind).
		Returns(http.StatusOK, "OK", gr.objExample).
		Returns(http.StatusCreated, "Create", gr.objExample).
		Returns(http.StatusUnauthorized, "Unauthorized", "")

	return gr.addNamespaceParam(builder).Param(NameParam(ws))
}

func PatchAction(gr *GenericResource, ws *restful.WebService) *restful.RouteBuilder {
	operation := "patch" + gr.objKind

	if gr.namespaced {
		operation = "patchNamespaced" + gr.objKind
	}

	builder := ws.PATCH(ResourcePath(gr.resource, gr.namespaced)).
		Consumes(mime.MIME_JSON_PATCH, mime.MIME_MERGE_PATCH).
		Produces(mime.MIME_JSON).
		Operation(operation).
		To(Noop).
		Writes(gr.objExample).Reads(metav1.Patch{}).
		Doc("Patch a "+gr.objKind+" object.").
		Metadata("kind", gr.objKind).
		Returns(http.StatusOK, "OK", gr.objExample).
		Returns(http.StatusUnauthorized, "Unauthorized", "")

	return gr.addNamespaceParam(builder).Param(NameParam(ws))
}

func DeleteAction(gr *GenericResource, ws *restful.WebService) *restful.RouteBuilder {
	operation := "delete" + gr.objKind

	if gr.namespaced {
		operation = "deleteNamespaced" + gr.objKind
	}

	builder := ws.DELETE(ResourcePath(gr.resource, gr.namespaced)).
		Produces(mime.MIME_JSON, mime.MIME_YAML).
		Consumes(mime.MIME_JSON, mime.MIME_YAML).
		Operation(operation).
		To(Noop).
		Reads(metav1.DeleteOptions{}).Writes(metav1.Status{}).
		Doc("Delete a "+gr.objKind+" object.").
		Metadata("kind", gr.objKind).
		Returns(http.StatusOK, "OK", gr.objExample).
		Returns(http.StatusUnauthorized, "Unauthorized", "")

	return gr.addNamespaceParam(builder).Param(NameParam(ws)).
		Param(gracePeriodSecondsParam(ws)).
		Param(orphanDependentsParam(ws)).
		Param(propagationPolicyParam(ws))
}
