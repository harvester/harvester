package types

import (
	"fmt"

	"github.com/rancher/wrangler/pkg/webhook"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/webhook/config"
)

type Request struct {
	*webhook.Request
	options *config.Options
}

func NewRequest(webhookRequest *webhook.Request, options *config.Options) *Request {
	return &Request{
		Request: webhookRequest,
		options: options,
	}
}

func (r *Request) Username() string {
	return r.UserInfo.Username
}

func (r *Request) IsFromController() bool {
	return r.Username() == r.options.HarvesterControllerUsername
}

func (r *Request) IsGarbageCollection() bool {
	return r.Operation == admissionv1.Delete && r.Username() == r.options.GarbageCollectionUsername
}

func (r *Request) DecodeObjects() (oldObj runtime.Object, newObj runtime.Object, err error) {
	operation := r.Operation
	if operation == admissionv1.Delete || operation == admissionv1.Update {
		oldObj, err = r.DecodeOldObject()
		if err != nil {
			return
		}
		if operation == admissionv1.Delete {
			// no new object for DELETE operation
			return
		}
	}
	newObj, err = r.DecodeObject()
	return
}

func (r *Request) String() string {
	return fmt.Sprintf("Request (user: %s, %s, namespace: %s, name: %s, operation: %s)", r.UserInfo.Username, r.Kind, r.Namespace, r.Name, r.Operation)
}
