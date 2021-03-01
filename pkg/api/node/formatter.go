package node

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	corev1 "k8s.io/api/core/v1"

	ctlnode "github.com/rancher/harvester/pkg/controller/master/node"
)

const (
	drainKey                     = "kubevirt.io/drain"
	enableMaintenanceModeAction  = "enableMaintenanceMode"
	disableMaintenanceModeAction = "disableMaintenanceMode"
)

func Formatter(request *types.APIRequest, resource *types.RawResource) {
	resource.Actions = make(map[string]string)
	if resource.APIObject.Data().String("metadata", "annotations", ctlnode.MaintainStatusAnnotationKey) != "" {
		resource.AddAction(request, disableMaintenanceModeAction)
	} else {
		resource.AddAction(request, enableMaintenanceModeAction)
	}
}

type ActionHandler struct {
	nodeCache  ctlcorev1.NodeCache
	nodeClient ctlcorev1.NodeClient
}

func (h ActionHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if err := h.do(rw, req); err != nil {
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

func (h ActionHandler) do(rw http.ResponseWriter, req *http.Request) error {
	vars := mux.Vars(req)
	action := vars["action"]
	name := vars["name"]
	node, err := h.nodeCache.Get(name)
	if err != nil {
		return err
	}
	toUpdate := node.DeepCopy()
	switch action {
	case enableMaintenanceModeAction:
		return h.enableMaintenanceMode(toUpdate)
	case disableMaintenanceModeAction:
		return h.disableMaintenanceMode(toUpdate)
	default:
		return apierror.NewAPIError(validation.InvalidAction, "Unsupported action")
	}
}

func (h ActionHandler) enableMaintenanceMode(node *corev1.Node) error {
	node.Spec.Unschedulable = true
	noDrainTaint := true
	for _, taint := range node.Spec.Taints {
		if taint.Key == drainKey {
			noDrainTaint = false
			break
		}
	}
	if noDrainTaint {
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    drainKey,
			Value:  "scheduling",
			Effect: corev1.TaintEffectNoSchedule,
		})
	}
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	node.Annotations[ctlnode.MaintainStatusAnnotationKey] = ctlnode.MaintainStatusRunning
	_, err := h.nodeClient.Update(node)
	return err
}

func (h ActionHandler) disableMaintenanceMode(node *corev1.Node) error {
	node.Spec.Unschedulable = false
	for i, taint := range node.Spec.Taints {
		if taint.Key == drainKey {
			node.Spec.Taints = append(node.Spec.Taints[:i], node.Spec.Taints[i+1:]...)
			break
		}
	}
	delete(node.Annotations, ctlnode.MaintainStatusAnnotationKey)
	_, err := h.nodeClient.Update(node)
	return err
}
