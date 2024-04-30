package subresource

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/sirupsen/logrus"
)

type Resource struct {
	Name        string
	ObjectName  string
	SubResource string
	Namespace   string
	Namespaced  bool
}

type handler struct{}

type healthHandler struct{}

type ResourceHandler interface {
	// IsMatchedResource checks if the resource, subresource and http method is matched
	IsMatchedResource(resource Resource, method string) bool

	// SubResourceHandler handles the request if `IsMatchedResource` is true.
	SubResourceHandler(rw http.ResponseWriter, r *http.Request, resource Resource) error
}

var (
	handlers                  []ResourceHandler
	apiPath                   = "/apis/subresources.harvesterhci.io/v1beta1"
	healthCheckPath           = apiPath
	namespacedSubResourcePath = apiPath + "/namespaces/{namespace}/{resource}/{name}/{subresource}"
	subResourcePath           = apiPath + "/{resource}/{name}/{subresource}"
)

func NewSubResourceHandler(mux *mux.Router) {
	subHealthHandler := &healthHandler{}
	mux.Path(healthCheckPath).Handler(subHealthHandler)

	subHandler := &handler{}
	mux.Path(namespacedSubResourcePath).Handler(subHandler)
	mux.Path(subResourcePath).Handler(subHandler)
}

func RegisterSubResourceHandler(handler ResourceHandler) {
	handlers = append(handlers, handler)
}

func (h *handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)

	var (
		err      error
		found    bool
		resource = Resource{
			Name:        vars["resource"],
			ObjectName:  vars["name"],
			SubResource: vars["subresource"],
			Namespace:   vars["namespace"],
		}
	)

	if vars["namespace"] != "" {
		resource.Namespaced = true
		resource.Namespace = vars["namespace"]
	}

	logrus.Debug("Resource: ", resource)

	for _, handler := range handlers {
		if !handler.IsMatchedResource(resource, req.Method) {
			continue
		}

		err = handler.SubResourceHandler(rw, req, resource)
		found = true
		break
	}

	if !found {
		rw.WriteHeader(http.StatusBadRequest)
		_, _ = rw.Write([]byte("Invalid resource handler"))
		return
	}

	if err != nil {
		status := http.StatusInternalServerError
		if e, ok := err.(*apierror.APIError); ok {
			status = e.Code.Status
		}
		rw.WriteHeader(status)
		_, _ = rw.Write([]byte(err.Error()))
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (h *healthHandler) ServeHTTP(rw http.ResponseWriter, _ *http.Request) {
	rw.WriteHeader(http.StatusOK)
}
