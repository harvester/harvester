package node

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/norman/httperror"
	"github.com/rancher/wrangler/v3/pkg/data/convert"
	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/rancher/wrangler/v3/pkg/slice"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"

	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
	"github.com/harvester/harvester/pkg/controller/master/nodedrain"
	harvesterctlv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/drainhelper"
)

const (
	drainKey                     = "kubevirt.io/drain"
	enableMaintenanceModeAction  = "enableMaintenanceMode"
	disableMaintenanceModeAction = "disableMaintenanceMode"
	clearMaintenanceModeAction   = "clearMaintenanceMode"
	cordonAction                 = "cordon"
	uncordonAction               = "uncordon"
	listUnhealthyVM              = "listUnhealthyVM"
	maintenancePossible          = "maintenancePossible"
	powerAction                  = "powerAction"
	powerActionPossible          = "powerActionPossible"
	seederAddonName              = "harvester-seeder"
	defaultAddonNamespace        = "harvester-system"
	nodeReady                    = "inventoryNodeReady"
	enableCPUManager             = "enableCPUManager"
	disableCPUManager            = "disableCPUManager"
)

var (
	seederGVR            = schema.GroupVersionResource{Group: "metal.harvesterhci.io", Version: "v1alpha1", Resource: "inventories"}
	possiblePowerActions = []string{"shutdown", "poweron", "reboot"}
)

func Formatter(request *types.APIRequest, resource *types.RawResource) {
	resource.Actions = make(map[string]string, 3)
	resource.AddAction(request, listUnhealthyVM)
	resource.AddAction(request, maintenancePossible)
	resource.AddAction(request, powerActionPossible)

	if request.AccessControl.CanUpdate(request, resource.APIObject, resource.Schema) != nil {
		return
	}

	resource.AddAction(request, enableCPUManager)
	resource.AddAction(request, disableCPUManager)

	node := &corev1.Node{}
	if err := convert.ToObj(resource.APIObject.Data(), node); err != nil {
		logrus.Errorf("Failed to decode node %s from raw resource: %v", resource.ID, err)
		return
	}

	condition := util.GetMaintenanceModeCondition(node)
	intentCleanupComplete := !drainhelper.HasDrainRequest(node) && !drainhelper.IsForcedDrainRequested(node)

	if condition == nil {
		// No MaintenanceMode condition exists, so no maintenance attempt is active
		// or failed and the node can enter maintenance mode.
		resource.AddAction(request, enableMaintenanceModeAction)
	} else {
		switch {
		case util.IsMaintenanceModeCondition(condition, corev1.ConditionFalse, util.NodeConditionReasonError):
			// MaintenanceMode=False/Error means validation failed before Draining
			// began. No drain side effects exist, so the error can be cleared.
			if intentCleanupComplete {
				resource.AddAction(request, clearMaintenanceModeAction)
			}
		case util.IsMaintenanceModeCondition(condition, corev1.ConditionTrue, util.NodeConditionReasonError):
			// MaintenanceMode=True/Error means Draining timed out after side effects
			// may have occurred. The node remains maintenance-protected, so the error
			// can be disabled, but never cleared or retried directly.
			resource.AddAction(request, disableMaintenanceModeAction)
		case util.IsMaintenanceModeDrainComplete(condition):
			// MaintenanceMode=True/Evacuating or True/Completed means DrainNode
			// finished successfully. The node can be disabled or used for power actions.
			resource.AddAction(request, disableMaintenanceModeAction)
			resource.AddAction(request, powerAction)
		}
	}

	if node.Spec.Unschedulable {
		resource.AddAction(request, "uncordon")
	} else {
		resource.AddAction(request, "cordon")
	}
}

type ActionHandler struct {
	jobCache                    ctlbatchv1.JobCache
	nodeCache                   ctlcorev1.NodeCache
	nodeClient                  ctlcorev1.NodeClient
	longhornVolumeCache         ctllhv1.VolumeCache
	longhornReplicaCache        ctllhv1.ReplicaCache
	virtualMachineClient        ctlkubevirtv1.VirtualMachineClient
	virtualMachineCache         ctlkubevirtv1.VirtualMachineCache
	virtualMachineInstanceCache ctlkubevirtv1.VirtualMachineInstanceCache
	addonCache                  harvesterctlv1beta1.AddonCache
	dynamicClient               dynamic.Interface
	virtSubresourceRestClient   rest.Interface
	ctx                         context.Context
}

func (h ActionHandler) Do(ctx *harvesterServer.Ctx) (harvesterServer.ResponseBody, error) {
	req, rw := ctx.Req(), ctx.RespWriter()
	vars := util.EncodeVars(mux.Vars(req))
	action := vars["action"]
	name := vars["name"]
	node, err := h.nodeCache.Get(name)
	if err != nil {
		return nil, err
	}
	toUpdate := node.DeepCopy()
	switch action {
	case enableMaintenanceModeAction:
		return nil, h.enableMaintenanceMode(req, toUpdate)
	case disableMaintenanceModeAction:
		return nil, h.disableMaintenanceMode(name)
	case clearMaintenanceModeAction:
		return nil, h.clearMaintenanceMode(name)
	case cordonAction:
		return nil, h.cordonUncordonNode(toUpdate, cordonAction, true)
	case uncordonAction:
		return nil, h.cordonUncordonNode(toUpdate, uncordonAction, false)
	case listUnhealthyVM:
		return nil, h.listUnhealthyVM(rw, toUpdate)
	case maintenancePossible:
		return nil, h.maintenancePossible(toUpdate)
	case powerActionPossible:
		return nil, h.powerActionPossible(rw, name)
	case powerAction:
		var input PowerActionInput
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			return nil, apierror.NewAPIError(validation.InvalidBodyContent, fmt.Sprintf("Failed to decode request body: %v ", err))
		}
		return nil, h.powerAction(toUpdate, input.Operation)
	case enableCPUManager:
		return nil, h.enableCPUManager(toUpdate)
	case disableCPUManager:
		return nil, h.disableCPUManager(toUpdate)
	default:
		return nil, apierror.NewAPIError(validation.InvalidAction, "Unsupported action")
	}
}

func (h ActionHandler) enableCPUManager(node *corev1.Node) error {
	return h.requestCPUManager(node, ctlnode.CPUManagerStaticPolicy)
}

func (h ActionHandler) disableCPUManager(node *corev1.Node) error {
	return h.requestCPUManager(node, ctlnode.CPUManagerNonePolicy)
}

func (h ActionHandler) requestCPUManager(node *corev1.Node, policy ctlnode.CPUManagerPolicy) error {
	newNode := node.DeepCopy()
	if newNode.Annotations == nil {
		newNode.Annotations = make(map[string]string)
	}

	updateStatus := &ctlnode.CPUManagerUpdateStatus{
		Status: ctlnode.CPUManagerRequestedStatus,
		Policy: policy,
	}

	bytes, err := json.Marshal(updateStatus)
	if err != nil {
		return err
	}
	newNode.Annotations[util.AnnotationCPUManagerUpdateStatus] = string(bytes)
	if _, err = h.nodeClient.Update(newNode); err != nil {
		return err
	}

	return nil
}

func (h ActionHandler) cordonUncordonNode(node *corev1.Node, actionName string, cordon bool) error {
	if cordon == node.Spec.Unschedulable {
		return httperror.NewAPIError(httperror.InvalidAction, fmt.Sprintf("Node %s already %sed", node.Name, actionName))
	}
	node.Spec.Unschedulable = cordon
	_, err := h.nodeClient.Update(node)
	return err
}

func (h ActionHandler) enableMaintenanceMode(req *http.Request, node *corev1.Node) error {
	node, err := h.nodeClient.Get(node.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	condition := util.GetMaintenanceModeCondition(node)

	if condition != nil {
		return httperror.NewAPIError(httperror.InvalidAction, "Maintenance mode cannot be enabled in the current state")
	}

	// API-based tests run enableMaintenanceMode directly. The maintenance-possible
	// action ensures it cannot be called where maintenance mode is not possible.
	var maintenanceInput MaintenanceModeInput
	if err := json.NewDecoder(req.Body).Decode(&maintenanceInput); err != nil {
		return apierror.NewAPIError(validation.InvalidBodyContent, fmt.Sprintf("Failed to decode request body: %v ", err))
	}

	if maintenanceInput.Force == "true" {
		logrus.Infof("forced drain requested for node %s", node.Name)
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		nodeObj, err := h.nodeClient.Get(node.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		liveCondition := util.GetMaintenanceModeCondition(nodeObj)

		if liveCondition != nil {
			return httperror.NewAPIError(httperror.InvalidAction, "Maintenance mode cannot be enabled in the current state")
		}

		drainhelper.SetDrainRequest(nodeObj, maintenanceInput.Force == "true")
		_, err = h.nodeClient.Update(nodeObj)
		return err
	})
}

type maintenanceModeUpdateFunc func(node *corev1.Node)

func (h ActionHandler) disableMaintenanceMode(nodeName string) error {
	node, err := h.nodeClient.Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	condition := util.GetMaintenanceModeCondition(node)

	if !util.CanDisableMaintenanceMode(condition) {
		return httperror.NewAPIError(httperror.InvalidAction, "Maintenance mode cannot be disabled in the current state")
	}

	disableMaintenanceModeFunc := func(node *corev1.Node) {
		node.Spec.Unschedulable = false
		for i, taint := range node.Spec.Taints {
			if taint.Key == drainKey {
				node.Spec.Taints = append(node.Spec.Taints[:i], node.Spec.Taints[i+1:]...)
				break
			}
		}
		drainhelper.ClearDrainRequest(node)
	}

	err = h.retryMaintenanceModeUpdate(nodeName, disableMaintenanceModeFunc)
	if err != nil {
		return err
	}

	// Restart those VMs that have been labeled to be shut down before
	// maintenance mode and that should be restarted when the maintenance
	// mode has been disabled again.
	if err := util.RestartMaintenanceModeVMsOnNode(h.virtualMachineClient, h.virtualMachineCache, nodeName, util.MaintainModeStrategyShutdownAndRestartAfterDisable); err != nil {
		return err
	}

	return h.removeMaintenanceModeCondition(nodeName, false)
}

func (h ActionHandler) clearMaintenanceMode(nodeName string) error {
	return h.removeMaintenanceModeCondition(nodeName, true)
}

func (h ActionHandler) removeMaintenanceModeCondition(nodeName string, preCheckErrorOnly bool) error {
	logrus.WithFields(logrus.Fields{
		"node":              nodeName,
		"preCheckErrorOnly": preCheckErrorOnly,
	}).Info("Removing node maintenance mode condition")

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := h.nodeClient.Get(nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		condition := util.GetMaintenanceModeCondition(node)
		if condition == nil {
			return nil
		}

		if preCheckErrorOnly && (drainhelper.HasDrainRequest(node) || drainhelper.IsForcedDrainRequested(node)) {
			return httperror.NewAPIError(httperror.InvalidAction, "Maintenance mode error cannot be cleared until the previous request is cleaned up")
		}
		if preCheckErrorOnly && !util.IsMaintenanceModeCondition(condition, corev1.ConditionFalse, util.NodeConditionReasonError) {
			return httperror.NewAPIError(httperror.InvalidAction, "Only a maintenance mode pre-check error can be cleared")
		}
		if !preCheckErrorOnly && (!util.CanDisableMaintenanceMode(condition) || drainhelper.HasDrainRequest(node)) {
			return httperror.NewAPIError(httperror.InvalidAction, "Maintenance mode disable state changed")
		}

		node = node.DeepCopy()
		util.RemoveMaintenanceModeCondition(node)
		_, err = h.nodeClient.UpdateStatus(node)
		return err
	})
}

func (h ActionHandler) retryMaintenanceModeUpdate(nodeName string, updateFunc maintenanceModeUpdateFunc) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := h.nodeClient.Get(nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		toUpdate := node.DeepCopy()
		updateFunc(toUpdate)
		_, err = h.nodeClient.Update(toUpdate)
		return err
	})
}

func (h ActionHandler) listUnhealthyVM(rw http.ResponseWriter, node *corev1.Node) error {
	ndc := nodedrain.ActionHelper(h.nodeCache, h.virtualMachineInstanceCache, h.longhornVolumeCache, h.longhornReplicaCache)
	nonMigrtableVMList, err := ndc.FindNonMigratableVMS(node)
	if err != nil {
		return err
	}
	respObj := make([]ListUnhealthyVM, 0, len(nonMigrtableVMList))
	for condition, vms := range nonMigrtableVMList {
		respObj = append(respObj, ListUnhealthyVM{
			Message: fmt.Sprintf("The following VMs cannot be migrated due to %s condition", condition),
			VMs:     vms,
		})
	}

	rw.WriteHeader(http.StatusOK)
	return json.NewEncoder(rw).Encode(&respObj)
}

func (h ActionHandler) maintenancePossible(node *corev1.Node) error {
	return drainhelper.DrainPossible(h.nodeCache, node)
}

func (h ActionHandler) powerActionPossible(rw http.ResponseWriter, node string) error {
	addon, err := h.addonCache.Get(defaultAddonNamespace, seederAddonName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Errorf("addon %s not found", seederAddonName)
			rw.WriteHeader(http.StatusFailedDependency)
			return nil
		}
		return err
	}

	if !addon.Spec.Enabled {
		logrus.Errorf("addon %s is not enabled", seederAddonName)
		rw.WriteHeader(http.StatusFailedDependency)
		return nil
	}

	inventoryObject, err := h.fetchInventoryObject(node)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Errorf("inventory %s not found", node)
			rw.WriteHeader(http.StatusFailedDependency)
			return nil
		}
		return err
	}

	val, ok, err := unstructured.NestedString(inventoryObject.Object, "status", "status")
	if err != nil {
		logrus.Errorf("error fetching status from inventory object %s :%v", node, err)
		rw.WriteHeader(http.StatusConflict)
		return err
	}

	if !ok {
		errMsg := fmt.Sprintf("no field .status.status present in inventory object: %s", node)
		logrus.Errorf("%s", errMsg)
		rw.WriteHeader(http.StatusConflict)
		return fmt.Errorf("%s", errMsg)
	}

	if val != nodeReady {
		errMsg := fmt.Sprintf("expected to find inventory %s status %s, but current status is: %s", node, nodeReady, val)
		logrus.Errorf("%s", errMsg)
		rw.WriteHeader(http.StatusConflict)
		return fmt.Errorf("%s", errMsg)
	}

	rw.WriteHeader(http.StatusNoContent)
	return nil
}

func (h ActionHandler) powerAction(node *corev1.Node, operation string) error {
	liveNode, err := h.nodeClient.Get(node.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	condition := util.GetMaintenanceModeCondition(liveNode)
	if !util.IsMaintenanceModeDrainComplete(condition) {
		return httperror.NewAPIError(httperror.InvalidAction, "Power actions are unavailable in the current maintenance mode state")
	}
	node = liveNode

	if !slice.ContainsString(possiblePowerActions, operation) {
		return fmt.Errorf("operation %s is not a valid power opreation. valid values need to be in %v", operation, possiblePowerActions)
	}

	inventoryObject, err := h.fetchInventoryObject(node.Name)
	if err != nil {
		return err
	}

	iObj := inventoryObject.DeepCopy()
	powerActionMap, ok, err := unstructured.NestedMap(iObj.Object, "status", "powerAction")
	if err != nil {
		return err
	}

	oldPowerAction, oldPowerActionOk, err := unstructured.NestedString(iObj.Object, "spec", "powerActionRequested")
	if err != nil {
		return err
	}

	if !ok {
		powerActionMap = make(map[string]interface{})
	}

	// if last power action is not yet completed, do not accept new request
	if oldPowerActionOk {
		if val, completed := powerActionMap["actionStatus"]; !completed || val == "" {
			return fmt.Errorf("waiting for power action status for action %s for node %s to be populated", oldPowerAction, node.Name)
		}
	}

	if err := unstructured.SetNestedField(iObj.Object, operation, "spec", "powerActionRequested"); err != nil {
		return err
	}

	// update powerActionRequest on inventory object
	iObjUpdate, err := h.dynamicClient.Resource(seederGVR).Namespace(defaultAddonNamespace).Update(h.ctx, iObj, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	// clear lastJobName
	powerActionMap["lastJobName"] = ""
	if err := unstructured.SetNestedMap(iObjUpdate.Object, powerActionMap, "status", "powerAction"); err != nil {
		return err
	}

	_, err = h.dynamicClient.Resource(seederGVR).Namespace(defaultAddonNamespace).UpdateStatus(h.ctx, iObjUpdate, metav1.UpdateOptions{})
	return err
}

func (h ActionHandler) fetchInventoryObject(node string) (*unstructured.Unstructured, error) {
	return h.dynamicClient.Resource(seederGVR).Namespace(defaultAddonNamespace).Get(h.ctx, node, metav1.GetOptions{})
}
