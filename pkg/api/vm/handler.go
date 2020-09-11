package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	ctlkubevirtv1alpha3 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1alpha3"
	"github.com/rancher/wrangler/pkg/slice"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	v1alpha3 "kubevirt.io/client-go/api/v1alpha3"
	cdiv1 "kubevirt.io/containerized-data-importer/pkg/apis/core/v1alpha1"
)

const (
	vmResource  = "virtualmachines"
	vmiResource = "virtualmachineinstances"
)

type vmActionHandler struct {
	vms        ctlkubevirtv1alpha3.VirtualMachineClient
	vmis       ctlkubevirtv1alpha3.VirtualMachineInstanceClient
	vmCache    ctlkubevirtv1alpha3.VirtualMachineCache
	restClient *rest.RESTClient
}

func (h *vmActionHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	action := vars["action"]
	namespace := vars["namespace"]
	name := vars["name"]
	var resource string

	switch action {
	case ejectCdRom:
		var input EjectCdRomActionInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			rw.WriteHeader(http.StatusBadRequest)
			rw.Write([]byte("Failed to decode request body, " + err.Error()))
			return
		}

		if len(input.DiskNames) == 0 {
			rw.WriteHeader(http.StatusBadRequest)
			rw.Write([]byte("Parameter diskNames is empty"))
			return
		}

		if err := h.ejectCdRom(name, namespace, input.DiskNames); err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			rw.Write([]byte(err.Error()))
		}
		rw.WriteHeader(http.StatusNoContent)
		return
	case startVM, stopVM, restartVM:
		resource = vmResource
	case pauseVM, unpauseVM:
		resource = vmiResource
	default:
		rw.WriteHeader(http.StatusBadRequest)
		rw.Write([]byte("Unsupported action"))
	}

	ctx := context.Background()
	if err := h.subresourceOperate(ctx, resource, namespace, name, action); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		rw.Write([]byte(fmt.Sprintf("%s virtualMachine %s:%s failed, %v", action, namespace, name, err)))
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (h *vmActionHandler) ejectCdRom(name, namespace string, diskNames []string) error {
	vm, err := h.vmCache.Get(namespace, name)
	if err != nil {
		return err
	}

	vmCopy := vm.DeepCopy()
	if err := ejectCdRomFromVM(vmCopy, diskNames); err != nil {
		return err
	}

	if !reflect.DeepEqual(vm, vmCopy) {
		if _, err := h.vms.Update(vmCopy); err != nil {
			return err
		}
	}

	return h.vmis.Delete(namespace, name, &metav1.DeleteOptions{})
}

func (h *vmActionHandler) subresourceOperate(ctx context.Context, resource, namespace, name, subresourece string) error {
	return h.restClient.Put().Namespace(namespace).Resource(resource).SubResource(subresourece).Name(name).Do(ctx).Error()
}

func ejectCdRomFromVM(vm *v1alpha3.VirtualMachine, diskNames []string) error {
	var disks []v1alpha3.Disk
	for _, disk := range vm.Spec.Template.Spec.Domain.Devices.Disks {
		if slice.ContainsString(diskNames, disk.Name) {
			if disk.CDRom == nil {
				return errors.New("disk " + disk.Name + " isn't a CD-ROM disk")
			}
			continue
		}
		disks = append(disks, disk)
	}

	var volumes []v1alpha3.Volume
	var ejectedVolumeNames []string
	for _, vol := range vm.Spec.Template.Spec.Volumes {
		if vol.VolumeSource.DataVolume != nil && slice.ContainsString(diskNames, vol.Name) {
			ejectedVolumeNames = append(ejectedVolumeNames, vol.VolumeSource.DataVolume.Name)
			continue
		}
		volumes = append(volumes, vol)
	}

	var dvs []cdiv1.DataVolume
	for _, v := range vm.Spec.DataVolumeTemplates {
		if slice.ContainsString(ejectedVolumeNames, v.Name) {
			continue
		}
		dvs = append(dvs, v)
	}

	vm.Spec.DataVolumeTemplates = dvs
	vm.Spec.Template.Spec.Volumes = volumes
	vm.Spec.Template.Spec.Domain.Devices.Disks = disks
	return nil
}
