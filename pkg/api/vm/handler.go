package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/rancher/apiserver/pkg/apierror"
	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/rancher/wrangler/pkg/slice"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	kv1 "kubevirt.io/client-go/api/v1"

	harvesterv1alpha1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	v1alpha1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	ctlharvesterv1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	ctlvmv1alpha1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	ctlkubevirtv1 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/rancher/harvester/pkg/settings"
	"github.com/rancher/harvester/pkg/util"
)

const (
	vmResource    = "virtualmachines"
	vmiResource   = "virtualmachineinstances"
	sshAnnotation = "harvester.cattle.io/sshNames"
)

type vmActionHandler struct {
	namespace                 string
	vms                       ctlkubevirtv1.VirtualMachineClient
	vmis                      ctlkubevirtv1.VirtualMachineInstanceClient
	vmCache                   ctlkubevirtv1.VirtualMachineCache
	vmiCache                  ctlkubevirtv1.VirtualMachineInstanceCache
	vmims                     ctlkubevirtv1.VirtualMachineInstanceMigrationClient
	vmTemplateClient          ctlvmv1alpha1.VirtualMachineTemplateClient
	vmTemplateVersionClient   ctlvmv1alpha1.VirtualMachineTemplateVersionClient
	vmimCache                 ctlkubevirtv1.VirtualMachineInstanceMigrationCache
	backups                   ctlharvesterv1.VirtualMachineBackupClient
	backupCache               ctlharvesterv1.VirtualMachineBackupCache
	restores                  ctlharvesterv1.VirtualMachineRestoreClient
	settingCache              ctlharvesterv1.SettingCache
	nodeCache                 ctlcorev1.NodeCache
	virtSubresourceRestClient rest.Interface
	virtRestClient            rest.Interface
}

func (h vmActionHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if err := h.doAction(rw, req); err != nil {
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
		var input MigrateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}
		return h.migrate(r.Context(), namespace, name, input.NodeName)
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
	case backupVM:
		var input BackupInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}

		if input.Name == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter backup name is required")
		}

		if err := h.checkBackupTargetConfigured(); err != nil {
			return err
		}

		if err := h.createVMBackup(name, namespace, input); err != nil {
			return err
		}
		return nil
	case restoreVM:
		var input RestoreInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}

		if input.Name == "" || input.BackupName == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter name and backupName are required")
		}

		if err := h.checkBackupTargetConfigured(); err != nil {
			return err
		}

		if err := h.restoreBackup(name, namespace, input); err != nil {
			return err
		}
		return nil
	case createTemplate:
		return h.createTemplate(namespace, name)
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
	return h.virtSubresourceRestClient.Put().Namespace(namespace).Resource(resource).SubResource(subresourece).Name(name).Do(ctx).Error()
}

func ejectCdRomFromVM(vm *kv1.VirtualMachine, diskNames []string) error {
	disks := make([]kv1.Disk, 0, len(vm.Spec.Template.Spec.Domain.Devices.Disks))
	for _, disk := range vm.Spec.Template.Spec.Domain.Devices.Disks {
		if slice.ContainsString(diskNames, disk.Name) {
			if disk.CDRom == nil {
				return errors.New("disk " + disk.Name + " isn't a CD-ROM disk")
			}
			continue
		}
		disks = append(disks, disk)
	}

	volumes := make([]kv1.Volume, 0, len(vm.Spec.Template.Spec.Volumes))
	ejectedVolumeNames := make([]string, 0, len(vm.Spec.Template.Spec.Volumes))
	for _, vol := range vm.Spec.Template.Spec.Volumes {
		if vol.VolumeSource.DataVolume != nil && slice.ContainsString(diskNames, vol.Name) {
			ejectedVolumeNames = append(ejectedVolumeNames, vol.VolumeSource.DataVolume.Name)
			continue
		}
		volumes = append(volumes, vol)
	}

	dvs := make([]kv1.DataVolumeTemplateSpec, 0, len(vm.Spec.DataVolumeTemplates))
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

func (h *vmActionHandler) migrate(ctx context.Context, namespace, vmName string, nodeName string) error {
	vmi, err := h.vmiCache.Get(namespace, vmName)
	if err != nil {
		return err
	}
	if !vmi.IsRunning() {
		return errors.New("The VM is not in running state")
	}
	if !canMigrate(vmi) {
		return errors.New("The VM is already in migrating state")
	}
	vmim := &kv1.VirtualMachineInstanceMigration{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: vmName + "-",
			Namespace:    namespace,
		},
		Spec: kv1.VirtualMachineInstanceMigrationSpec{
			VMIName: vmName,
		},
	}
	if nodeName != "" {
		// check node name is valid
		if _, err := h.nodeCache.Get(nodeName); err != nil {
			return err
		}
		if nodeName == vmi.Status.NodeName {
			return apierror.NewAPIError(validation.InvalidBodyContent, "The VM is currently running on the target node")
		}

		// set vmi node selector before starting the migration
		toUpdateVmi := vmi.DeepCopy()
		if toUpdateVmi.Annotations == nil {
			toUpdateVmi.Annotations = make(map[string]string)
		}
		if toUpdateVmi.Spec.NodeSelector == nil {
			toUpdateVmi.Spec.NodeSelector = make(map[string]string)
		}
		toUpdateVmi.Annotations[util.AnnotationMigrationTarget] = nodeName
		toUpdateVmi.Spec.NodeSelector[corev1.LabelHostname] = nodeName

		if err := util.VirtClientUpdateVmi(ctx, h.virtRestClient, h.namespace, namespace, vmName, toUpdateVmi); err != nil {
			return err
		}
	}

	_, err = h.vmims.Create(vmim)
	return err
}

func (h *vmActionHandler) abortMigration(namespace, name string) error {
	vmi, err := h.vmiCache.Get(namespace, name)
	if err != nil {
		return err
	}
	if !canAbortMigrate(vmi) {
		return errors.New("The VM is not in migrating state")
	}

	vmims, err := h.vmimCache.List(namespace, labels.Everything())
	if err != nil {
		return err
	}
	migrationUID := getMigrationUID(vmi)
	for _, vmim := range vmims {
		if migrationUID == string(vmim.UID) {
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

func (h *vmActionHandler) createVMBackup(vmName, vmNamespace string, input BackupInput) error {
	apiGroup := kv1.SchemeGroupVersion.Group
	backup := &harvesterv1alpha1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.Name,
			Namespace: vmNamespace,
		},
		Spec: harvesterv1alpha1.VirtualMachineBackupSpec{
			Source: corev1.TypedLocalObjectReference{
				APIGroup: &apiGroup,
				Kind:     kv1.VirtualMachineGroupVersionKind.Kind,
				Name:     vmName,
			},
		},
	}
	if _, err := h.backups.Create(backup); err != nil {
		return fmt.Errorf("failed to create VM backup, error: %s", err.Error())
	}
	return nil
}

func (h *vmActionHandler) restoreBackup(vmName, vmNamespace string, input RestoreInput) error {
	if _, err := h.backupCache.Get(vmNamespace, input.BackupName); err != nil {
		return err
	}
	apiGroup := kv1.SchemeGroupVersion.Group
	backup := &harvesterv1alpha1.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.Name,
			Namespace: vmNamespace,
		},
		Spec: harvesterv1alpha1.VirtualMachineRestoreSpec{
			Target: corev1.TypedLocalObjectReference{
				APIGroup: &apiGroup,
				Kind:     kv1.VirtualMachineGroupVersionKind.Kind,
				Name:     vmName,
			},
			VirtualMachineBackupName: input.BackupName,
			NewVM:                    false,
		},
	}
	_, err := h.restores.Create(backup)
	if err != nil {
		return fmt.Errorf("failed to create restore, error: %s", err.Error())
	}

	return nil
}

func (h *vmActionHandler) checkBackupTargetConfigured() error {
	target, err := h.settingCache.Get(settings.BackupTargetSettingName)
	if err == nil && harvesterv1alpha1.SettingConfigured.IsTrue(target) {
		return nil
	}
	return fmt.Errorf("backup target is invalid")
}

func getMigrationUID(vmi *kv1.VirtualMachineInstance) string {
	if vmi.Annotations[util.AnnotationMigrationUID] != "" {
		return vmi.Annotations[util.AnnotationMigrationUID]
	} else if vmi.Status.MigrationState != nil {
		return string(vmi.Status.MigrationState.MigrationUID)
	}
	return ""
}

// createTemplate creates a template and version that are derived from the given virtual machine.
func (h *vmActionHandler) createTemplate(namespace, name string) error {
	vmt, err := h.vmTemplateClient.Create(
		&v1alpha1.VirtualMachineTemplate{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: fmt.Sprintf("%s-clone-", name),
				Namespace:    namespace,
			},
			Spec: v1alpha1.VirtualMachineTemplateSpec{},
		})
	if err != nil {
		return err
	}

	vm, err := h.vmCache.Get(namespace, name)
	if err != nil {
		return err
	}

	keyPairIDs, err := getSSHKeysFromVMITemplateSpec(vm.Spec.Template)
	if err != nil {
		return err
	}
	vmID := fmt.Sprintf("%s/%s", vmt.Namespace, vmt.Name)

	_, err = h.vmTemplateVersionClient.Create(
		&v1alpha1.VirtualMachineTemplateVersion{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: fmt.Sprintf("%s-", vmt.Name),
				Namespace:    namespace,
			},
			Spec: v1alpha1.VirtualMachineTemplateVersionSpec{
				TemplateID:  vmID,
				Description: fmt.Sprintf("Template drived from virtual machine [%s]", vmID),
				VM:          removeMacAddresses(vm.Spec),
				KeyPairIDs:  keyPairIDs,
			},
		})
	return err
}

// removeMacAddresses replaces the mac address of each device interface with an empty string.
// This is because macAddresses are unique, and should not reuse the original's.
func removeMacAddresses(vmSpec kv1.VirtualMachineSpec) kv1.VirtualMachineSpec {
	censoredSpec := vmSpec
	for index := range censoredSpec.Template.Spec.Domain.Devices.Interfaces {
		censoredSpec.Template.Spec.Domain.Devices.Interfaces[index].MacAddress = ""
	}
	return censoredSpec
}

// getSSHKeysFromVMITemplateSpec first checks the given VirtualMachineInstanceTemplateSpec
// for ssh key annotation. If found, it attempts to parse it into a string slice and return
// it.
func getSSHKeysFromVMITemplateSpec(vmitSpec *kv1.VirtualMachineInstanceTemplateSpec) ([]string, error) {
	if vmitSpec == nil {
		return nil, nil
	}
	annos := vmitSpec.ObjectMeta.Annotations
	if annos == nil {
		return nil, nil
	}
	var sshKeys []string
	if err := json.Unmarshal([]byte(annos[sshAnnotation]), &sshKeys); err != nil {
		return nil, err
	}
	return sshKeys, nil
}
