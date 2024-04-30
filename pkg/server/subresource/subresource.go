package subresource

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	apiService = schema.GroupVersion{Group: "subresources.harvesterhci.io", Version: "v1beta1"}

	VirtualMachineImages   = schema.GroupVersionResource{Group: apiService.Group, Version: apiService.Version, Resource: "virtualmachineimages"}
	UpgradeLogs            = schema.GroupVersionResource{Group: apiService.Group, Version: apiService.Version, Resource: "upgradelogs"}
	Node                   = schema.GroupVersionResource{Group: apiService.Group, Version: apiService.Version, Resource: "nodes"}
	VirtualMachines        = schema.GroupVersionResource{Group: apiService.Group, Version: apiService.Version, Resource: "virtualmachines"}
	PersistentVolumeClaims = schema.GroupVersionResource{Group: apiService.Group, Version: apiService.Version, Resource: "persistentvolumeclaims"}
	VolumeSnapshots        = schema.GroupVersionResource{Group: apiService.Group, Version: apiService.Version, Resource: "snapshot.storage.k8s.io.volumesnapshot"}

	namespacedSubResources    = []schema.GroupVersionResource{VirtualMachineImages, UpgradeLogs, VirtualMachines, PersistentVolumeClaims, VolumeSnapshots}
	nonNamespacedSubResources = []schema.GroupVersionResource{Node}
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
	handlers        []ResourceHandler
	healthCheckPath = fmt.Sprintf("/apis/%s/%s", apiService.Group, apiService.Version)
)

func NewSubResourceHandler(mux *mux.Router) {
	var paths []string

	for _, resource := range namespacedSubResources {
		path := fmt.Sprintf("/apis/%s/%s/namespaces/{namespace}/%s/{name}/{subresource}", resource.Group, resource.Version, resource.Resource)
		paths = append(paths, path)
	}

	for _, resource := range nonNamespacedSubResources {
		path := fmt.Sprintf("/apis/%s/%s/%s/{name}/{subresource}", resource.Group, resource.Version, resource.Resource)
		paths = append(paths, path)
	}

	for _, path := range paths {
		mux.Path(path).Handler(&handler{})
	}

	mux.Path(healthCheckPath).Handler(&healthHandler{})
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
