package controller

import (
	"context"
	"fmt"
	"io"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	imapi "github.com/longhorn/longhorn-instance-manager/pkg/api"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/types"
)

// InstanceHandler can handle the state transition of correlated instance and
// engine/replica object. It assumed the instance it's going to operate with is using
// the SAME NAME from the engine/replica object
type InstanceHandler struct {
	ds                     *datastore.DataStore
	instanceManagerHandler InstanceManagerHandler
	eventRecorder          record.EventRecorder
}

type InstanceManagerHandler interface {
	GetInstance(obj interface{}) (*longhorn.InstanceProcess, error)
	CreateInstance(obj interface{}) (*longhorn.InstanceProcess, error)
	DeleteInstance(obj interface{}) error
	LogInstance(ctx context.Context, obj interface{}) (*engineapi.InstanceManagerClient, *imapi.LogStream, error)
}

func NewInstanceHandler(ds *datastore.DataStore, instanceManagerHandler InstanceManagerHandler, eventRecorder record.EventRecorder) *InstanceHandler {
	return &InstanceHandler{
		ds:                     ds,
		instanceManagerHandler: instanceManagerHandler,
		eventRecorder:          eventRecorder,
	}
}

func (h *InstanceHandler) syncStatusWithInstanceManager(im *longhorn.InstanceManager, instanceName string, spec *longhorn.InstanceSpec, status *longhorn.InstanceStatus) {
	defer func() {
		if status.CurrentState == longhorn.InstanceStateStopped {
			status.InstanceManagerName = ""
		}
	}()

	if im == nil || im.Status.CurrentState == longhorn.InstanceManagerStateUnknown {
		if status.Started {
			if status.CurrentState != longhorn.InstanceStateUnknown {
				logrus.Warnf("The related node %v of instance %v is down or deleted, will mark the instance as state UNKNOWN", spec.NodeID, instanceName)
			}
			status.CurrentState = longhorn.InstanceStateUnknown
		} else {
			status.CurrentState = longhorn.InstanceStateStopped
			status.CurrentImage = ""
		}
		status.IP = ""
		status.StorageIP = ""
		status.Port = 0
		return
	}

	if im.Status.CurrentState == longhorn.InstanceManagerStateStopped || im.Status.CurrentState == longhorn.InstanceManagerStateError || im.DeletionTimestamp != nil {
		if status.Started {
			if status.CurrentState != longhorn.InstanceStateError {
				logrus.Warnf("Cannot find the instance manager for the running instance %v, will mark the instance as state ERROR", instanceName)
			}
			status.CurrentState = longhorn.InstanceStateError
		} else {
			status.CurrentState = longhorn.InstanceStateStopped
		}
		status.CurrentImage = ""
		status.IP = ""
		status.StorageIP = ""
		status.Port = 0
		return
	}

	if im.Status.CurrentState == longhorn.InstanceManagerStateStarting {
		if status.Started {
			if status.CurrentState != longhorn.InstanceStateError {
				logrus.Warnf("The starting instance manager %v shouldn't contain the running instance %v, will mark the instance as state ERROR", im.Name, instanceName)
			}
			status.CurrentState = longhorn.InstanceStateError
			status.CurrentImage = ""
			status.IP = ""
			status.StorageIP = ""
			status.Port = 0
		}
		return
	}

	instance, exists := im.Status.Instances[instanceName]
	if !exists {
		if status.Started {
			if status.CurrentState != longhorn.InstanceStateError {
				logrus.Warnf("Cannot find the instance status in instance manager %v for the running instance %v, will mark the instance as state ERROR", im.Name, instanceName)
			}
			status.CurrentState = longhorn.InstanceStateError
		} else {
			status.CurrentState = longhorn.InstanceStateStopped
		}
		status.CurrentImage = ""
		status.IP = ""
		status.StorageIP = ""
		status.Port = 0
		return
	}

	if status.InstanceManagerName != "" && status.InstanceManagerName != im.Name {
		logrus.Errorf("BUG: The related process of instance %v is found in the instance manager %v, but the instance manager name in the instance status is %v. "+
			"The instance manager name shouldn't change except for cleanup",
			instanceName, im.Name, status.InstanceManagerName)
	}
	// `status.InstanceManagerName` should be set when the related instance process status
	// exists in the instance manager.
	// `status.InstanceManagerName` can be used to clean up the process in instance manager
	// and fetch log even if the instance status becomes `error` or `stopped`
	status.InstanceManagerName = im.Name

	switch instance.Status.State {
	case longhorn.InstanceStateStarting:
		status.CurrentState = longhorn.InstanceStateStarting
		status.CurrentImage = ""
		status.IP = ""
		status.StorageIP = ""
		status.Port = 0
	case longhorn.InstanceStateRunning:
		status.CurrentState = longhorn.InstanceStateRunning

		imPod, err := h.ds.GetPod(im.Name)
		if err != nil {
			logrus.WithError(err).Errorf("failed to get instance manager pod from %v", im.Name)
			return
		}

		if imPod == nil {
			logrus.Debugf("instance manager pod from %v not exist in datastore", im.Name)
			return
		}

		storageIP := h.ds.GetStorageIPFromPod(imPod)
		if status.StorageIP != storageIP {
			status.StorageIP = storageIP
			logrus.Debugf("Instance %v starts running, Storage IP %v", instanceName, status.StorageIP)
		}

		if status.IP != im.Status.IP {
			status.IP = im.Status.IP
			logrus.Debugf("Instance %v starts running, IP %v", instanceName, status.IP)
		}
		if status.Port != int(instance.Status.PortStart) {
			status.Port = int(instance.Status.PortStart)
			logrus.Debugf("Instance %v starts running, Port %d", instanceName, status.Port)
		}
		// only set CurrentImage when first started, since later we may specify
		// different spec.EngineImage for upgrade
		if status.CurrentImage == "" {
			status.CurrentImage = spec.EngineImage
		}
	case longhorn.InstanceStateStopping:
		if status.Started {
			status.CurrentState = longhorn.InstanceStateError
		} else {
			status.CurrentState = longhorn.InstanceStateStopping
		}
		status.CurrentImage = ""
		status.IP = ""
		status.StorageIP = ""
		status.Port = 0
	case longhorn.InstanceStateStopped:
		if status.Started {
			status.CurrentState = longhorn.InstanceStateError
		} else {
			status.CurrentState = longhorn.InstanceStateStopped
		}
		status.CurrentImage = ""
		status.IP = ""
		status.StorageIP = ""
		status.Port = 0
	default:
		if status.CurrentState != longhorn.InstanceStateError {
			logrus.Warnf("Instance %v is state %v, error message: %v", instanceName, instance.Status.State, instance.Status.ErrorMsg)
		}
		status.CurrentState = longhorn.InstanceStateError
		status.CurrentImage = ""
		status.IP = ""
		status.StorageIP = ""
		status.Port = 0
	}
}

// getNameFromObj will get the name from the object metadata, which will be used
// as podName later
func (h *InstanceHandler) getNameFromObj(obj runtime.Object) (string, error) {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return "", err
	}
	return metadata.GetName(), nil
}

func (h *InstanceHandler) ReconcileInstanceState(obj interface{}, spec *longhorn.InstanceSpec, status *longhorn.InstanceStatus) (err error) {
	runtimeObj, ok := obj.(runtime.Object)
	if !ok {
		return fmt.Errorf("obj is not a runtime.Object: %v", obj)
	}
	instanceName, err := h.getNameFromObj(runtimeObj)
	if err != nil {
		return err
	}

	isCLIAPIVersionOne := false
	if status.CurrentImage != "" {
		isCLIAPIVersionOne, err = h.ds.IsEngineImageCLIAPIVersionOne(status.CurrentImage)
		if err != nil {
			return err
		}
	}

	var im *longhorn.InstanceManager
	if !isCLIAPIVersionOne {
		if status.InstanceManagerName != "" {
			im, err = h.ds.GetInstanceManager(status.InstanceManagerName)
			if err != nil {
				if datastore.ErrorIsNotFound(err) {
					logrus.Debugf("cannot find instance manager %v", status.InstanceManagerName)
				} else {
					return err
				}
			}
		}
		// There should be an available instance manager for a scheduled instance when its related engine image is compatible
		if im == nil && spec.EngineImage != "" && spec.NodeID != "" {
			// The related node maybe cleaned up then there is no available instance manager for this instance (typically it's replica).
			isNodeDownOrDeleted, err := h.ds.IsNodeDownOrDeleted(spec.NodeID)
			if err != nil {
				return err
			}
			if !isNodeDownOrDeleted {
				im, err = h.ds.GetInstanceManagerByInstance(obj)
				if err != nil {
					return errors.Wrapf(err, "failed to get instance manager for instance %v", instanceName)
				}
			}
		}
	}

	if spec.LogRequested {
		if !status.LogFetched {
			logrus.Warnf("Try to get requested log for %v in instance manager %v", instanceName, status.InstanceManagerName)
			if im == nil {
				logrus.Warnf("Cannot get the log for %v due to Instance Manager is already gone", status.InstanceManagerName)
			} else if err := h.printInstanceLogs(instanceName, runtimeObj); err != nil {
				logrus.Warnf("cannot get requested log for instance %v on node %v, error %v", instanceName, im.Spec.NodeID, err)
			}
			status.LogFetched = true
		}
	} else { // spec.LogRequested = false
		status.LogFetched = false
	}

	if status.SalvageExecuted && !spec.SalvageRequested {
		status.SalvageExecuted = false
	}

	// do nothing for incompatible instance except for deleting
	switch spec.DesireState {
	case longhorn.InstanceStateRunning:
		if isCLIAPIVersionOne {
			return nil
		}

		if im == nil {
			break
		}

		if i, exists := im.Status.Instances[instanceName]; exists && i.Status.State == longhorn.InstanceStateRunning {
			status.Started = true
			break
		}

		// there is a delay between createInstance() invocation and InstanceManager update,
		// createInstance() may be called multiple times.
		if status.CurrentState != longhorn.InstanceStateStopped {
			break
		}

		err = h.createInstance(instanceName, runtimeObj)
		if err != nil {
			return err
		}

		// Set the SalvageExecuted flag to clear the SalvageRequested flag.
		if spec.SalvageRequested {
			status.SalvageExecuted = true
		}

	case longhorn.InstanceStateStopped:
		if isCLIAPIVersionOne {
			if err := h.deleteInstance(instanceName, runtimeObj); err != nil {
				return err
			}
			status.Started = false
			status.CurrentState = longhorn.InstanceStateStopped
			status.CurrentImage = ""
			status.InstanceManagerName = ""
			status.IP = ""
			status.StorageIP = ""
			status.Port = 0
			return nil
		}

		if im != nil && im.DeletionTimestamp == nil {
			// there is a delay between deleteInstance() invocation and state/InstanceManager update,
			// deleteInstance() may be called multiple times.
			if _, exists := im.Status.Instances[instanceName]; exists {
				if err := h.deleteInstance(instanceName, runtimeObj); err != nil {
					return err
				}
			}
		}
		status.Started = false
	default:
		return fmt.Errorf("BUG: unknown instance desire state: desire %v", spec.DesireState)
	}

	oldState := status.CurrentState

	h.syncStatusWithInstanceManager(im, instanceName, spec, status)

	if oldState != status.CurrentState {
		logrus.Debugf("Instance handler updated instance %v state, old state %v, new state %v", instanceName, oldState, status.CurrentState)
	}

	if status.CurrentState == longhorn.InstanceStateRunning {
		// If `spec.DesireState` is `longhorn.InstanceStateStopped`, `spec.NodeID` has been unset by volume controller.
		if spec.DesireState != longhorn.InstanceStateStopped {
			if spec.NodeID != im.Spec.NodeID {
				status.CurrentState = longhorn.InstanceStateError
				status.IP = ""
				status.StorageIP = ""
				err := fmt.Errorf("BUG: instance %v NodeID %v is not the same as the instance manager %v NodeID %v", instanceName, spec.NodeID, im.Name, im.Spec.NodeID)
				logrus.Errorf("%v", err)
				return err
			}
		}
	} else if status.CurrentState == longhorn.InstanceStateError {
		if im != nil {
			if _, exists := im.Status.Instances[instanceName]; exists {
				logrus.Warnf("Instance %v crashed on Instance Manager %v at %v, try to get log", instanceName, im.Name, im.Spec.NodeID)
				if err := h.printInstanceLogs(instanceName, runtimeObj); err != nil {
					logrus.Warnf("cannot get crash log for instance %v on Instance Manager %v at %v, error %v", instanceName, im.Name, im.Spec.NodeID, err)
				}
			}
		}
	}
	return nil
}

func (h *InstanceHandler) printInstanceLogs(instanceName string, obj runtime.Object) error {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	client, stream, err := h.instanceManagerHandler.LogInstance(ctx, obj)
	if err != nil {
		return err
	}
	defer client.Close()
	for {
		line, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			logrus.Errorf("failed to receive log for instance %v: %v", instanceName, err)
			return err
		}
		logrus.Warnf("%s: %s", instanceName, line)
	}
	return nil
}

func (h *InstanceHandler) createInstance(instanceName string, obj runtime.Object) error {
	_, err := h.instanceManagerHandler.GetInstance(obj)
	if err == nil {
		logrus.Debugf("Instance process %v had been created, need to wait for instance manager update", instanceName)
		return nil
	}
	if !types.ErrorIsNotFound(err) {
		return err
	}

	logrus.Debugf("Prepare to create instance %v", instanceName)
	if _, err := h.instanceManagerHandler.CreateInstance(obj); err != nil {
		if !types.ErrorAlreadyExists(err) {
			h.eventRecorder.Eventf(obj, v1.EventTypeWarning, EventReasonFailedStarting, "Error starting %v: %v", instanceName, err)
			return err
		}
		// Already exists, lost track may due to previous datastore conflict
		return nil
	}
	h.eventRecorder.Eventf(obj, v1.EventTypeNormal, EventReasonStart, "Starts %v", instanceName)

	return nil
}

func (h *InstanceHandler) deleteInstance(instanceName string, obj runtime.Object) error {
	// May try to force deleting instances on lost node. Don't need to check the instance
	logrus.Debugf("Prepare to delete instance %v", instanceName)
	if err := h.instanceManagerHandler.DeleteInstance(obj); err != nil {
		h.eventRecorder.Eventf(obj, v1.EventTypeWarning, EventReasonFailedStopping, "Error stopping %v: %v", instanceName, err)
		return err
	}
	h.eventRecorder.Eventf(obj, v1.EventTypeNormal, EventReasonStop, "Stops %v", instanceName)

	return nil
}
