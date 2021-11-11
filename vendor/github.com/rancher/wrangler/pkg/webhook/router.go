package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	defObjType    = &unstructured.Unstructured{}
	jsonPatchType = v1.PatchTypeJSONPatch
)

func NewRouter() *Router {
	return &Router{}
}

type Router struct {
	matches []*RouteMatch
}

func (r *Router) sendError(rw http.ResponseWriter, review *v1.AdmissionReview, err error) {
	logrus.Debug(err)
	if review == nil || review.Request == nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	review.Response.Result = &errors.NewInternalError(err).ErrStatus
	writeResponse(rw, review)
}

func writeResponse(rw http.ResponseWriter, review *v1.AdmissionReview) {
	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(review)
}

func (r *Router) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	review := &v1.AdmissionReview{}
	err := json.NewDecoder(req.Body).Decode(review)
	if err != nil {
		r.sendError(rw, review, err)
		return
	}

	if review.Request == nil {
		r.sendError(rw, review, fmt.Errorf("request is not set"))
		return
	}

	response := &Response{
		AdmissionResponse: v1.AdmissionResponse{
			UID: review.Request.UID,
		},
	}

	review.Response = &response.AdmissionResponse

	if err := r.admit(response, review.Request, req); err != nil {
		r.sendError(rw, review, err)
		return
	}

	writeResponse(rw, review)
}

func (r *Router) admit(response *Response, request *v1.AdmissionRequest, req *http.Request) error {
	for _, m := range r.matches {
		if m.matches(request) {
			err := m.admit(response, &Request{
				AdmissionRequest: *request,
				Context:          req.Context(),
				objTemplate:      m.getObjType(),
			})
			logrus.Debugf("admit result: %s %s %s user=%s allowed=%v err=%v", request.Operation, request.Kind.String(), resourceString(request.Namespace, request.Name), request.UserInfo.Username, response.Allowed, err)
			return err
		}
	}
	logrus.Debugf("no route match found for %s %s %s", request.Operation, request.Kind.String(), resourceString(request.Namespace, request.Name))
	return nil
}

func (r *Router) next() *RouteMatch {
	match := &RouteMatch{}
	r.matches = append(r.matches, match)
	return match
}

type Request struct {
	v1.AdmissionRequest

	Context     context.Context
	objTemplate runtime.Object
}

func (r *Request) DecodeOldObject() (runtime.Object, error) {
	obj := r.objTemplate.DeepCopyObject()
	err := json.Unmarshal(r.OldObject.Raw, obj)
	return obj, err
}

func (r *Request) DecodeObject() (runtime.Object, error) {
	obj := r.objTemplate.DeepCopyObject()
	err := json.Unmarshal(r.Object.Raw, obj)
	return obj, err
}

type Response struct {
	v1.AdmissionResponse
}

func (r *Response) CreatePatch(request *Request, newObj runtime.Object) error {
	if len(r.Patch) > 0 {
		return fmt.Errorf("response patch has already been already been assigned")
	}

	newBytes, err := json.Marshal(newObj)
	if err != nil {
		return err
	}

	patch, err := jsonpatch.CreateMergePatch(request.Object.Raw, newBytes)
	if err != nil {
		return err
	}

	r.Patch = patch
	r.PatchType = &jsonPatchType
	return nil
}

type Handler interface {
	Admit(resp *Response, req *Request) error
}

type HandlerFunc func(resp *Response, req *Request) error

func (h HandlerFunc) Admit(resp *Response, req *Request) error {
	return h(resp, req)
}

// resourceString returns the resource formatted as a string
func resourceString(ns, name string) string {
	if ns == "" {
		return name
	}
	return fmt.Sprintf("%s/%s", ns, name)
}
