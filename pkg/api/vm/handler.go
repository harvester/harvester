package vm

import (
	"encoding/json"
	"net/http"
	"reflect"

	ctlkubevirtv1alpha3 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1alpha3"
	"github.com/rancher/wrangler/pkg/slice"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1alpha3 "kubevirt.io/client-go/api/v1alpha3"
	cdiv1 "kubevirt.io/containerized-data-importer/pkg/apis/core/v1alpha1"
)

type vmActionHandler struct {
	vms     ctlkubevirtv1alpha3.VirtualMachineClient
	vmis    ctlkubevirtv1alpha3.VirtualMachineInstanceClient
	vmCache ctlkubevirtv1alpha3.VirtualMachineCache
}

func (h *vmActionHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	action := vars["action"]
	namespace := vars["namespace"]
	name := vars["name"]

	switch action {
	case startVM, stopVM:
		running := action == startVM
		if err := h.updateVMRunningStatus(name, namespace, action, running); err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			rw.Write([]byte(err.Error()))
			return
		}

		rw.WriteHeader(http.StatusNoContent)
	case restartVM:
		if err := h.restartVM(name, namespace); err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			rw.Write([]byte(err.Error()))
		}
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
	default:
		rw.WriteHeader(http.StatusBadRequest)
		rw.Write([]byte("Unsupported action"))
	}
}

func (h *vmActionHandler) updateVMRunningStatus(name, namespace, action string, running bool) error {
	vm, err := h.vms.Get(namespace, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if vm.Spec.Running != nil && running == *vm.Spec.Running {
		return nil
	}

	vmCopy := vm.DeepCopy()
	vmCopy.Spec.Running = &running
	_, err = h.vms.Update(vmCopy)
	if err != nil {
		return errors.Wrapf(err, "Failed to %s to virtual machine %s/%s", action, namespace, name)
	}

	return nil
}

func (h *vmActionHandler) restartVM(name, namespace string) error {
	if err := h.vmis.Delete(namespace, name, &metav1.DeleteOptions{}); err != nil {
		return errors.Wrapf(err, "Failed to delete virtualMachineInstance %s/%s", namespace, name)
	}

	return nil
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
