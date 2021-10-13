package node

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/norman/httperror"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	corev1 "k8s.io/api/core/v1"

	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
)

const (
	drainKey                     = "kubevirt.io/drain"
	enableMaintenanceModeAction  = "enableMaintenanceMode"
	disableMaintenanceModeAction = "disableMaintenanceMode"
	cordonAction                 = "cordon"
	uncordonAction               = "uncordon"
)

func Formatter(request *types.APIRequest, resource *types.RawResource) {
	resource.Actions = make(map[string]string, 1)
	if request.AccessControl.CanUpdate(request, resource.APIObject, resource.Schema) != nil {
		return
	}

	if resource.APIObject.Data().String("metadata", "annotations", ctlnode.MaintainStatusAnnotationKey) != "" {
		resource.AddAction(request, disableMaintenanceModeAction)
	} else {
		resource.AddAction(request, enableMaintenanceModeAction)
	}

	if resource.APIObject.Data().Bool("spec", "unschedulable") {
		resource.AddAction(request, "uncordon")
	} else {
		resource.AddAction(request, "cordon")
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
	case cordonAction:
		return h.cordonUncordonNode(toUpdate, cordonAction, true)
	case uncordonAction:
		return h.cordonUncordonNode(toUpdate, uncordonAction, false)
	default:
		return apierror.NewAPIError(validation.InvalidAction, "Unsupported action")
	}
}

func (h ActionHandler) cordonUncordonNode(node *corev1.Node, actionName string, cordon bool) error {
	if cordon == node.Spec.Unschedulable {
		return httperror.NewAPIError(httperror.InvalidAction, fmt.Sprintf("Node %s already %sed", node.Name, actionName))
	}
	node.Spec.Unschedulable = cordon
	_, err := h.nodeClient.Update(node)
	return err
}

func (h ActionHandler) enableMaintenanceMode(node *corev1.Node) error {
	node.Spec.Unschedulable = true
	if !hasDrainTaint(node.Spec.Taints) {
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

func hasDrainTaint(taints []corev1.Taint) bool {
	for _, taint := range taints {
		if taint.Key == drainKey {
			return true
		}
	}
	return false
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
