package admission

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/rancher/wrangler/v3/pkg/webhook"
	"github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/webhook/pkg/config"
	werror "github.com/harvester/webhook/pkg/error"
)

// AdmissionType includes mutation and validation
type AdmissionType string

var (
	AdmissionTypeMutation   AdmissionType = "mutation"
	AdmissionTypeValidation AdmissionType = "validation"
)

// Patch returned by the mutator
// JSON Patch operations to mutate input data. See https://jsonpatch.com/ for more information.
type Patch []PatchOp

// PatchOperation includes add, remove, replace, copy, move and test
type PatchOperation string

const (
	PatchOpAdd     PatchOperation = "add"
	PatchOpRemove  PatchOperation = "remove"
	PatchOpReplace PatchOperation = "replace"
	PatchOpCopy    PatchOperation = "copy"
	PatchOpMove    PatchOperation = "move"
	PatchOpTest    PatchOperation = "test"
)

// PatchOp is one patch operation
type PatchOp struct {
	Op    PatchOperation `json:"op,required"`
	Path  string         `json:"path,required"`
	Value interface{}    `json:"value,omitempty"`
}

// Admitter interface is used by AdmissionHandler to check if an operation is allowed.
type Admitter interface {
	// Create checks if a CREATE operation is allowed.
	// Patches contains JSON patch operations to be applied on the API object received by the server.
	// If no error is returned, the operation is allowed.
	Create(request *Request, newObj runtime.Object) (Patch, error)

	// Update checks if a UPDATE operation is allowed.
	// Patches contains JSON patch operations to be applied on the API object received by the server.
	// If no error is returned, the operation is allowed.
	Update(request *Request, oldObj runtime.Object, newObj runtime.Object) (Patch, error)

	// Delete checks if a DELETE operation is allowed.
	// Patches contains JSON patch operations to be applied on the API object received by the server.
	// If no error is returned, the operation is allowed.
	Delete(request *Request, oldObj runtime.Object) (Patch, error)

	// Connect checks if a CONNECT operation is allowed.
	// Patches contains JSON patch operations to be applied on the API object received by the server.
	// If no error is returned, the operation is allowed.
	Connect(request *Request, newObj runtime.Object) (Patch, error)

	// Resource returns the resource that the admitter works on.
	Resource() Resource
}

// Handler for the admitter webhook server
type Handler struct {
	admitter      Admitter
	admissionType AdmissionType
	options       *config.Options
}

// NewHandler returns a new admitter handler
func NewHandler(admitter Admitter, admissionType AdmissionType, options *config.Options) *Handler {
	if err := admitter.Resource().Validate(); err != nil {
		panic(err.Error())
	}
	return &Handler{
		admitter:      admitter,
		admissionType: admissionType,
		options:       options,
	}
}

// Admit function handles the AdmissionReview request
func (v *Handler) Admit(response *webhook.Response, request *webhook.Request) error {
	return v.admit(response, NewRequest(request, v.options))
}

func (v *Handler) admit(response *webhook.Response, req *Request) error {
	logrus.Debugf("%s admitting %s", req, v.admissionType)

	oldObj, newObj, err := req.DecodeObjects()
	if err != nil {
		logrus.Errorf("%s fail to decode objects: %s", req, err)
		response.Result = werror.NewInternalError(err.Error()).AsResult()
		response.Allowed = false
		return err
	}

	var patch Patch

	switch req.Operation {
	case admissionv1.Create:
		patch, err = v.admitter.Create(req, newObj)
	case admissionv1.Delete:
		patch, err = v.admitter.Delete(req, oldObj)
	case admissionv1.Update:
		patch, err = v.admitter.Update(req, oldObj, newObj)
	case admissionv1.Connect:
		patch, err = v.admitter.Connect(req, newObj)
	default:
		err = fmt.Errorf("unsupported operation %s", req.Operation)
	}

	if err != nil {
		var admitErr werror.AdmitError
		if e, ok := err.(werror.AdmitError); ok {
			admitErr = e
		} else {
			admitErr = werror.NewInternalError(err.Error())
		}
		response.Allowed = false
		response.Result = admitErr.AsResult()
		logrus.Debugf("%s operation is rejected: %s", req, admitErr)
		return err
	}

	logrus.Debugf("patch: %+v", patch)
	if patch != nil {
		patchType := admissionv1.PatchTypeJSONPatch
		response.PatchType = &patchType
		response.Patch, err = json.Marshal(patch)
		if err != nil {
			return err
		}
		logrus.Debugf("%v patches: %s", req, string(response.Patch))
	}

	logrus.Debugf("%v operation is allowed", req)
	response.Allowed = true

	return nil
}

func (v *Handler) decodeObjects(request *Request) (oldObj runtime.Object, newObj runtime.Object, err error) {
	operation := request.Operation
	if operation == admissionv1.Delete || operation == admissionv1.Update {
		oldObj, err = request.DecodeOldObject()
		if err != nil {
			return
		}
		if operation == admissionv1.Delete {
			// no new object for DELETE operation
			return
		}
	}
	newObj, err = request.DecodeObject()
	return
}

func (v *Handler) AddToWebhookRouter(router *webhook.Router) {
	rsc := v.admitter.Resource()
	kind := reflect.Indirect(reflect.ValueOf(rsc.ObjectType)).Type().Name()
	router.Kind(kind).Group(rsc.APIGroup).Type(rsc.ObjectType).Handle(v)
}
