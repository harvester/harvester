package event

import (
	"encoding/json"

	v1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

type Controller struct {
	vmClient    ctlkubevirtv1.VirtualMachineClient
	vmCache     ctlkubevirtv1.VirtualMachineCache
	eventClient v1.EventController
}

func (ctr *Controller) SyncEvent(_ string, event *corev1.Event) (*corev1.Event, error) {
	if event == nil {
		return event, nil
	}

	if event.InvolvedObject.Kind == "VirtualMachineInstance" {
		if err := ctr.updateVMEventRecord(event, event.InvolvedObject.Namespace, event.InvolvedObject.Name); err != nil {
			logrus.Errorf("failed to update event record: %v", err)
			return event, err
		}
	}

	return event, nil
}

func (ctr *Controller) updateVMEventRecord(event *corev1.Event, vmiNamespace, vmiName string) error {
	var (
		oldEvents       []*corev1.Event
		newEvents       []*corev1.Event
		eventRecords    string
		newEventRecords []byte
		latestEvents    []*corev1.Event
		keepLatestCount = 5
		err             error
	)

	vm, err := ctr.vmCache.Get(vmiNamespace, vmiName)
	if err != nil {
		logrus.Errorf("failed to get vm: %v", err)
		return nil
	}
	vmDp := vm.DeepCopy()

	eventRecords, _ = vmDp.Annotations[util.AnnotationVMIEventRecords]

	if eventRecords != "" {
		if err = json.Unmarshal([]byte(eventRecords), &oldEvents); err != nil {
			logrus.Errorf("updateVMEventRecord: failed to unmarshal eventRecords: %v", err)
			return err
		}
	}

	// When we append new events, we should trim it by keepLatestCount.
	// Make sure we don't put too many events in the annotation.
	newEvents = append(oldEvents, event)

	if len(newEvents) > keepLatestCount {
		latestEvents = newEvents[len(newEvents)-keepLatestCount:]
	} else {
		latestEvents = newEvents
	}

	if newEventRecords, err = json.Marshal(latestEvents); err != nil {
		logrus.Errorf("updateVMEventRecord: failed to marshal latestEvents: %v", err)
		return err
	}

	vmDp.Annotations[util.AnnotationVMIEventRecords] = string(newEventRecords)

	if _, err := ctr.vmClient.Update(vmDp); err != nil {
		logrus.Errorf("updateVMEventRecord: failed to update vm: %v", err)
		return err
	}

	return nil
}
