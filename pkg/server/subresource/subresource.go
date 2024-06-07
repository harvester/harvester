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

type ResourceHandler interface {
	// IsMatchedResource checks if the resource, subresource and http method are matched
	IsMatchedResource(resource Resource, method string) bool

	// SubResourceHandler handles the request if `IsMatchedResource` is true.
	SubResourceHandler(rw http.ResponseWriter, r *http.Request, resource Resource) error
}

var (
	handlers        []ResourceHandler
	healthCheckPath = fmt.Sprintf("/apis/%s/%s", apiService.Group, apiService.Version)
)

func injectResource(next http.Handler, resource string) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		mux.Vars(req)["resource"] = resource
		next.ServeHTTP(rw, req)
	})
}

func NewSubResourceHandler(mux *mux.Router) {
	// force full matched routers instead of matching non-existed resource
	for _, resource := range namespacedSubResources {
		path := fmt.Sprintf("/apis/%s/%s/namespaces/{namespace}/%s/{name}/{subresource}", resource.Group, resource.Version, resource.Resource)
		mux.Path(path).Handler(injectResource(http.HandlerFunc(serveSubResource), resource.Resource))
	}

	for _, resource := range nonNamespacedSubResources {
		path := fmt.Sprintf("/apis/%s/%s/%s/{name}/{subresource}", resource.Group, resource.Version, resource.Resource)
		mux.Path(path).Handler(injectResource(http.HandlerFunc(serveSubResource), resource.Resource))
	}

	mux.Path(healthCheckPath).Handler(http.HandlerFunc(healthCheck))
}

func RegisterSubResourceHandler(handler ResourceHandler) {
	handlers = append(handlers, handler)
}

func healthCheck(rw http.ResponseWriter, _ *http.Request) {
	rw.WriteHeader(http.StatusOK)
}

// Execute processes the request from old API schema
func Execute(handler ResourceHandler, rw http.ResponseWriter, r *http.Request, resource Resource) error {
	if !handler.IsMatchedResource(resource, r.Method) {
		return apierror.NewAPIError(validation.InvalidAction, "Unsupported action")
	}

	return handler.SubResourceHandler(rw, r, resource)
}

// serveSubResource processes the request from new API schema
func serveSubResource(rw http.ResponseWriter, req *http.Request) {
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
