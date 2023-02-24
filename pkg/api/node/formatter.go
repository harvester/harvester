package node

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/norman/httperror"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
	"github.com/harvester/harvester/pkg/controller/master/nodedrain"
	kubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	longhornv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/drainhelper"
)

const (
	drainKey                     = "kubevirt.io/drain"
	enableMaintenanceModeAction  = "enableMaintenanceMode"
	disableMaintenanceModeAction = "disableMaintenanceMode"
	cordonAction                 = "cordon"
	uncordonAction               = "uncordon"
	listUnhealthyVM              = "listUnhealthyVM"
	maintenancePossible          = "maintenancePossible"
)

func Formatter(request *types.APIRequest, resource *types.RawResource) {
	resource.Actions = make(map[string]string, 3)
	resource.AddAction(request, listUnhealthyVM)
	resource.AddAction(request, maintenancePossible)
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
	nodeCache                   ctlcorev1.NodeCache
	nodeClient                  ctlcorev1.NodeClient
	longhornVolumeCache         longhornv1beta1.VolumeCache
	longhornReplicaCache        longhornv1beta1.ReplicaCache
	virtualMachineInstanceCache kubevirtv1.VirtualMachineInstanceCache
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
	vars := util.EncodeVars(mux.Vars(req))
	action := vars["action"]
	name := vars["name"]
	node, err := h.nodeCache.Get(name)
	if err != nil {
		return err
	}
	toUpdate := node.DeepCopy()
	switch action {
	case enableMaintenanceModeAction:
		var maintenanceInput MaintenanceModeInput
		if err := json.NewDecoder(req.Body).Decode(&maintenanceInput); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}

		if maintenanceInput.Force == "true" {
			logrus.Infof("forced drain requested for node %s", node.Name)
		}
		return h.enableMaintenanceMode(toUpdate, maintenanceInput.Force)
	case disableMaintenanceModeAction:
		return h.disableMaintenanceMode(name)
	case cordonAction:
		return h.cordonUncordonNode(toUpdate, cordonAction, true)
	case uncordonAction:
		return h.cordonUncordonNode(toUpdate, uncordonAction, false)
	case listUnhealthyVM:
		return h.listUnhealthyVM(rw, toUpdate)
	case maintenancePossible:
		return h.maintenancePossible(toUpdate)
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

func (h ActionHandler) enableMaintenanceMode(node *corev1.Node, force string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		nodeObj, err := h.nodeClient.Get(node.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		if nodeObj.Annotations == nil {
			nodeObj.Annotations = make(map[string]string)
		}
		nodeObj.Annotations[drainhelper.DrainAnnotation] = "true"
		if force == "true" {
			nodeObj.Annotations[drainhelper.ForcedDrain] = "true"
		}
		_, err = h.nodeClient.Update(nodeObj)
		return err
	})
}

type maintenanceModeUpdateFunc func(node *corev1.Node)

func (h ActionHandler) disableMaintenanceMode(nodeName string) error {
	disableMaintaenanceModeFunc := func(node *corev1.Node) {
		node.Spec.Unschedulable = false
		for i, taint := range node.Spec.Taints {
			if taint.Key == drainKey {
				node.Spec.Taints = append(node.Spec.Taints[:i], node.Spec.Taints[i+1:]...)
				break
			}
		}
		delete(node.Annotations, drainhelper.DrainAnnotation)
		delete(node.Annotations, drainhelper.ForcedDrain)
		delete(node.Annotations, ctlnode.MaintainStatusAnnotationKey)
	}

	return h.retryMaintenanceModeUpdate(nodeName, disableMaintaenanceModeFunc, "disable")
}

func (h ActionHandler) retryMaintenanceModeUpdate(nodeName string, updateFunc maintenanceModeUpdateFunc, actionName string) error {
	maxTry := 3
	for i := 0; i < maxTry; i++ {
		node, err := h.nodeCache.Get(nodeName)
		if err != nil {
			return err
		}
		toUpdate := node.DeepCopy()
		updateFunc(toUpdate)
		if reflect.DeepEqual(node, toUpdate) {
			return nil
		}
		_, err = h.nodeClient.Update(toUpdate)
		if err == nil || !errors.IsConflict(err) {
			return err
		}
		// after last try, do not sleep, return asap.
		if i < maxTry-1 {
			time.Sleep(2 * time.Second)
		}
	}

	return fmt.Errorf("failed to %s maintenance mode on node:%s", actionName, nodeName)
}

func (h ActionHandler) listUnhealthyVM(rw http.ResponseWriter, node *corev1.Node) error {
	ndc := nodedrain.ActionHelper(h.nodeCache, h.virtualMachineInstanceCache, h.longhornVolumeCache, h.longhornReplicaCache)
	vmList, err := ndc.FindAndListVM(node)
	if err != nil {
		return err
	}

	var respObj ListUnhealthyVM

	if len(vmList) > 0 {
		respObj.Message = "Following unhealthy VMs will be impacted by the node drain:"
		respObj.VMs = vmList
	}

	rw.WriteHeader(http.StatusOK)
	return json.NewEncoder(rw).Encode(&respObj)
}

func (h ActionHandler) maintenancePossible(node *corev1.Node) error {
	return drainhelper.DrainPossible(h.nodeCache, node)
}
