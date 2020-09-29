package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/rancher/apiserver/pkg/apierror"
	ctlkubevirtv1alpha3 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1alpha3"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/rancher/wrangler/pkg/slice"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
	vmiCache   ctlkubevirtv1alpha3.VirtualMachineInstanceCache
	vmims      ctlkubevirtv1alpha3.VirtualMachineInstanceMigrationClient
	vmimCache  ctlkubevirtv1alpha3.VirtualMachineInstanceMigrationCache
	restClient *rest.RESTClient
}

func (h vmActionHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if err := h.doAction(rw, req); err != nil {
		if e, ok := err.(*apierror.APIError); ok {
			rw.WriteHeader(e.Code.Status)
		} else {
			rw.WriteHeader(http.StatusInternalServerError)
		}
		rw.Write([]byte(err.Error()))
	} else {
		rw.WriteHeader(http.StatusNoContent)
	}
}

func (h *vmActionHandler) doAction(rw http.ResponseWriter, r *http.Request) error {
	vars := mux.Vars(r)
	action := vars["action"]
	namespace := vars["namespace"]
	name := vars["name"]

	switch action {
	case ejectCdRom:
		var input EjectCdRomActionInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}

		if len(input.DiskNames) == 0 {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter diskNames is empty")
		}

		return h.ejectCdRom(name, namespace, input.DiskNames)
	case migrate:
		return h.migrate(namespace, name)
	case abortMigration:
		return h.abortMigration(namespace, name)
	case startVM, stopVM, restartVM:
		if err := h.subresourceOperate(r.Context(), vmResource, namespace, name, action); err != nil {
			return fmt.Errorf("%s virtual machine %s/%s failed, %v", action, namespace, name, err)
		}
	case pauseVM, unpauseVM:
		if err := h.subresourceOperate(r.Context(), vmiResource, namespace, name, action); err != nil {
			return fmt.Errorf("%s virtual machine %s/%s failed, %v", action, namespace, name, err)
		}
	default:
		return apierror.NewAPIError(validation.InvalidAction, "Unsupported action")
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
		return h.vmis.Delete(namespace, name, &metav1.DeleteOptions{})
	}

	return nil
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

func (h *vmActionHandler) migrate(namespace, name string) error {
	vmi, err := h.vmiCache.Get(namespace, name)
	if err != nil {
		return err
	}
	if !vmi.IsRunning() {
		return errors.New("The VM is not in running state")
	}
	if vmi.Status.MigrationState != nil && !vmi.Status.MigrationState.Completed {
		return errors.New("The VM is already in migrating state")
	}
	vmim := &v1alpha3.VirtualMachineInstanceMigration{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name + "-",
			Namespace:    namespace,
		},
		Spec: v1alpha3.VirtualMachineInstanceMigrationSpec{
			VMIName: name,
		},
	}
	_, err = h.vmims.Create(vmim)
	return err
}

func (h *vmActionHandler) abortMigration(namespace, name string) error {
	vmi, err := h.vmiCache.Get(namespace, name)
	if err != nil {
		return err
	}
	if vmi.Status.MigrationState == nil || vmi.Status.MigrationState.Completed {
		return errors.New("The VM is not in migrating state")
	}

	vmims, err := h.vmimCache.List(namespace, labels.Everything())
	if err != nil {
		return err
	}
	for _, vmim := range vmims {
		if vmi.Status.MigrationState.MigrationUID == vmim.UID {
			if !vmim.IsRunning() {
				return fmt.Errorf("cannot abort the migration as it is in %q phase", vmim.Status.Phase)
			}
			//Migration is aborted by deleting the VMIM object
			if err := h.vmims.Delete(namespace, vmim.Name, &metav1.DeleteOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}
