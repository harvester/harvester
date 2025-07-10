package cluster

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"

	corev1 "k8s.io/api/core/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/rancher/apiserver/pkg/apierror"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/gorilla/mux"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

type Handler struct {
	vmCache   ctlkubevirtv1.VirtualMachineCache
	nodeCache ctlcorev1.NodeCache
}

func (h Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
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

func (h Handler) do(rw http.ResponseWriter, req *http.Request) error {

	vars := util.EncodeVars(mux.Vars(req))
	link := vars["link"]
	var result []byte
	var err error
	switch link {
	case deviceCapacity:
		result, err = h.generateDeviceAvailabilityResponse()
	case machineTypes:
		result, err = generateMachineTypes()
	}

	if err != nil {
		return fmt.Errorf("unable to marshal api response: %v", err)
	}
	_, err = rw.Write(result)
	return err
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

// generateDeviceAvailabilityResponse is a wrapper around
func (h Handler) generateDeviceAvailabilityResponse() ([]byte, error) {
	nodeDeviceAvailability, err := h.generateDeviceAvailability()
	if err != nil {
		return nil, err
	}
	return json.Marshal(nodeDeviceAvailability)
}

// generateMachineTypes is a helper to return machineTypes for UI to render machine types possible
func generateMachineTypes() ([]byte, error) {
	var machineTypes []string
	switch runtime.GOARCH {
	case "amd64":
		machineTypes = append(machineTypes, "q35")
	case "arm64":
		machineTypes = append(machineTypes, "virt")
	}

	return json.Marshal(machineTypes)
}
