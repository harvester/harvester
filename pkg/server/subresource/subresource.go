package subresource

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	mux.Path(healthCheckPath).Handler(&healthHandler{})
	mux.Path(fmt.Sprintf("/apis/%s/%s/namespaces/{namespace}/{resource}/{name}/{subresource}", apiService.Group, apiService.Version)).Handler(&handler{})
	mux.Path(fmt.Sprintf("/apis/%s/%s/{resource}/{name}/{subresource}", apiService.Group, apiService.Version)).Handler(&handler{})
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
		err = apierror.NewAPIError(validation.InvalidAction, fmt.Sprintf("Unsupported %s/%s", resource.Name, resource.SubResource))
	}

	if err != nil {
		status := http.StatusInternalServerError
		if e, ok := err.(*apierror.APIError); ok {
			status = e.Code.Status
		} else if apierrors.IsNotFound(err) {
			status = http.StatusNotFound
		}
		rw.WriteHeader(status)
		fmt.Println(err)
		_, _ = rw.Write([]byte(err.Error()))
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (h *healthHandler) ServeHTTP(rw http.ResponseWriter, _ *http.Request) {
	rw.WriteHeader(http.StatusOK)
}
