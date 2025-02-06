package cluster

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/resource"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
)

type Handler struct {
	vmCache   ctlkubevirtv1.VirtualMachineCache
	nodeCache ctlcorev1.NodeCache
}

func (h Handler) Do(ctx *harvesterServer.Ctx) (harvesterServer.ResponseBody, error) {
	rw := ctx.RespWriter()

	nodeDeviceAvailability, err := h.generateDeviceAvailability()
	if err != nil {
		return nil, err
	}

	result, err := json.Marshal(nodeDeviceAvailability)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal node device capacity: %v", err)
	}

	_, err = rw.Write(result)
	return nil, err
}

func (h Handler) generateDeviceAvailability() (map[string]resource.Quantity, error) {
	nodes, err := h.nodeCache.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("error looking up nodes from nodeCache: %v", err)
	}

	vmList, err := h.vmCache.List(corev1.NamespaceAll, labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("error looking up vms from vmCache: %v", err)
	}

	return calculateAllocation(nodes, vmList), nil
}

func calculateAllocation(nodes []*corev1.Node, vms []*kubevirtv1.VirtualMachine) map[string]resource.Quantity {
	nodeDeviceAvailability := make(map[string]resource.Quantity)
	for _, node := range nodes {
		for resourceName, quantity := range node.Status.Allocatable {
			existingQuantity := nodeDeviceAvailability[resourceName.String()]
			existingQuantity.Add(quantity)
			nodeDeviceAvailability[resourceName.String()] = existingQuantity
		}
	}
	for _, vm := range vms {
		for _, hostDevice := range vm.Spec.Template.Spec.Domain.Devices.HostDevices {
			currentAvailability, ok := nodeDeviceAvailability[hostDevice.DeviceName]
			if ok {
				currentAvailability.Sub(*resource.NewQuantity(1, resource.DecimalSI))
				nodeDeviceAvailability[hostDevice.DeviceName] = currentAvailability
			}
		}

		for _, gpuDevice := range vm.Spec.Template.Spec.Domain.Devices.GPUs {
			currentAvailability, ok := nodeDeviceAvailability[gpuDevice.DeviceName]
			if ok {
				currentAvailability.Sub(*resource.NewQuantity(1, resource.DecimalSI))
				nodeDeviceAvailability[gpuDevice.DeviceName] = currentAvailability
			}
		}
	}
	return nodeDeviceAvailability
}
