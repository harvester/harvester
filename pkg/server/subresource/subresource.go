package subresource

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/apierror"
)

type Resource struct {
	Name        string
	ObjectName  string
	SubResource string
	Namespace   string
}

type Handler struct{}

type HealthHandler struct{}

type ResourceHandler interface {
	Matched(resource string) bool
	SubResourceHandler(rw http.ResponseWriter, r *http.Request, resource Resource) error
}

var (
	handlers []ResourceHandler
)

func RegisterSubResourceHandler(handler ResourceHandler) {
	handlers = append(handlers, handler)
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)

	namespace := vars["namespace"]
	resource := vars["resource"]
	name := vars["name"]
	subresource := vars["subresource"]

	fmt.Println("namespace: ", namespace)
	fmt.Println("resource: ", resource)
	fmt.Println("name: ", name)
	fmt.Println("subresource: ", subresource)

	var (
		err   error
		found bool
	)

	for _, handler := range handlers {
		if handler.Matched(resource) {
			fmt.Println("==== handler matched ====")
			fmt.Println("namespace: ", namespace)
			fmt.Println("resource: ", resource)
			fmt.Println("name: ", name)
			fmt.Println("subresource: ", subresource)
			fmt.Println("==== handler matched ====")
			err = handler.SubResourceHandler(rw, req, Resource{
				Name:        name,
				ObjectName:  name,
				SubResource: subresource,
				Namespace:   namespace,
			})
			found = true
			break
		}
	}

	if !found {
		rw.WriteHeader(http.StatusNotFound)
		_, _ = rw.Write([]byte("Not found subresource handler"))
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

func (h *HealthHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	rw.WriteHeader(http.StatusOK)
}
