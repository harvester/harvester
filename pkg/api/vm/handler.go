package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/rancher/apiserver/pkg/apierror"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/rancher/wrangler/pkg/slice"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/rest"
	kv1 "kubevirt.io/client-go/api/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	vmResource    = "virtualmachines"
	vmiResource   = "virtualmachineinstances"
	sshAnnotation = "harvesterhci.io/sshNames"

	systemDiskBootOrder    uint = 1
	pvcNameRandomStringLen int  = 5
)

type vmActionHandler struct {
	namespace                 string
	vms                       ctlkubevirtv1.VirtualMachineClient
	vmis                      ctlkubevirtv1.VirtualMachineInstanceClient
	vmCache                   ctlkubevirtv1.VirtualMachineCache
	vmiCache                  ctlkubevirtv1.VirtualMachineInstanceCache
	vmims                     ctlkubevirtv1.VirtualMachineInstanceMigrationClient
	vmTemplateClient          ctlharvesterv1.VirtualMachineTemplateClient
	vmTemplateVersionClient   ctlharvesterv1.VirtualMachineTemplateVersionClient
	vmimCache                 ctlkubevirtv1.VirtualMachineInstanceMigrationCache
	backups                   ctlharvesterv1.VirtualMachineBackupClient
	backupCache               ctlharvesterv1.VirtualMachineBackupCache
	restores                  ctlharvesterv1.VirtualMachineRestoreClient
	settingCache              ctlharvesterv1.SettingCache
	nodeCache                 ctlcorev1.NodeCache
	pvcCache                  ctlcorev1.PersistentVolumeClaimCache
	secretClient              ctlcorev1.SecretClient
	secretCache               ctlcorev1.SecretCache
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

		return h.ejectCdRom(r.Context(), name, namespace, input.DiskNames)
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
		var input CreateTemplateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}

		if input.Name == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Template name is required")
		}
		return h.createTemplate(namespace, name, input)
	case addVolume:
		var input AddVolumeInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}
		if input.DiskName == "" || input.VolumeSourceName == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter `diskName` and `volumeName` are required")
		}
		return h.addVolume(r.Context(), namespace, name, input)
	case removeVolume:
		var input RemoveVolumeInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}
		if input.DiskName == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter `volumeName` are required")
		}
		return h.removeVolume(r.Context(), namespace, name, input)
	default:
		return apierror.NewAPIError(validation.InvalidAction, "Unsupported action")
	}
	return nil
}

func (h *vmActionHandler) ejectCdRom(ctx context.Context, name, namespace string, diskNames []string) error {
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
		return h.subresourceOperate(ctx, vmResource, namespace, name, restartVM)
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
	toRemoveClaimNames := make([]string, 0, len(vm.Spec.Template.Spec.Volumes))
	for _, vol := range vm.Spec.Template.Spec.Volumes {
		if vol.VolumeSource.PersistentVolumeClaim != nil && slice.ContainsString(diskNames, vol.Name) {
			toRemoveClaimNames = append(toRemoveClaimNames, vol.VolumeSource.PersistentVolumeClaim.ClaimName)
			continue
		}
		volumes = append(volumes, vol)
	}

	if err := removeVolumeClaimTemplatesFromVmAnnotation(vm, toRemoveClaimNames); err != nil {
		return err
	}
	vm.Spec.Template.Spec.Volumes = volumes
	vm.Spec.Template.Spec.Domain.Devices.Disks = disks
	return nil
}

func removeVolumeClaimTemplatesFromVmAnnotation(vm *kv1.VirtualMachine, toRemoveDiskNames []string) error {
	volumeClaimTemplatesStr, ok := vm.Annotations[util.AnnotationVolumeClaimTemplates]
	if !ok {
		return nil
	}
	var volumeClaimTemplates, toUpdateVolumeClaimTemplates []corev1.PersistentVolumeClaim
	if err := json.Unmarshal([]byte(volumeClaimTemplatesStr), &volumeClaimTemplates); err != nil {
		return err
	}
	for _, volumeClaimTemplate := range volumeClaimTemplates {
		if !slice.ContainsString(toRemoveDiskNames, volumeClaimTemplate.Name) {
			toUpdateVolumeClaimTemplates = append(toUpdateVolumeClaimTemplates, volumeClaimTemplate)
		}
	}
	toUpdateVolumeClaimTemplateBytes, err := json.Marshal(toUpdateVolumeClaimTemplates)
	if err != nil {
		return err
	}
	vm.Annotations[util.AnnotationVolumeClaimTemplates] = string(toUpdateVolumeClaimTemplateBytes)
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
	if !isReady(vmi) {
		return errors.New("Can't migrate the VM, the VM is not in ready status")
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
	backup := &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.Name,
			Namespace: vmNamespace,
		},
		Spec: harvesterv1.VirtualMachineBackupSpec{
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
	backup := &harvesterv1.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.Name,
			Namespace: vmNamespace,
		},
		Spec: harvesterv1.VirtualMachineRestoreSpec{
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
	targetSetting, err := h.settingCache.Get(settings.BackupTargetSettingName)
	if err == nil && harvesterv1.SettingConfigured.IsTrue(targetSetting) {
		// backup target may be reset to initial/default, the SettingConfigured.IsTrue meets
		target, err := settings.DecodeBackupTarget(targetSetting.Value)
		if err != nil {
			return err
		}
		if !target.IsDefaultBackupTarget() {
			return nil
		}
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
func (h *vmActionHandler) createTemplate(namespace, name string, input CreateTemplateInput) error {
	vmt, err := h.vmTemplateClient.Create(
		&harvesterv1.VirtualMachineTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      input.Name,
				Namespace: namespace,
			},
			Spec: harvesterv1.VirtualMachineTemplateSpec{
				Description: input.Description,
			},
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

	vmtvName := fmt.Sprintf("%s-%s", vmt.Name, rand.String(5))
	vmtv, err := h.vmTemplateVersionClient.Create(
		&harvesterv1.VirtualMachineTemplateVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmtvName,
				Namespace: namespace,
			},
			Spec: harvesterv1.VirtualMachineTemplateVersionSpec{
				TemplateID:  vmID,
				Description: fmt.Sprintf("Template drived from virtual machine [%s]", vmID),
				VM:          sanitizeVirtualMachineForTemplateVersion(vmtvName, vm),
				KeyPairIDs:  keyPairIDs,
			},
		})
	if err != nil {
		return err
	}

	return h.createCloudConfigSecrets(vmtv, vm)
}

func (h *vmActionHandler) createCloudConfigSecrets(templateVersion *harvesterv1.VirtualMachineTemplateVersion, vm *kv1.VirtualMachine) error {
	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.CloudInitNoCloud == nil {
			continue
		}
		if volume.CloudInitNoCloud.UserDataSecretRef != nil {
			toCreateSecretName := getTemplateVersionUserDataSecretName(templateVersion.Name, volume.Name)
			if err := h.copySecret(volume.CloudInitNoCloud.UserDataSecretRef.Name, toCreateSecretName, templateVersion); err != nil {
				return err
			}
		}
		if volume.CloudInitNoCloud.NetworkDataSecretRef != nil {
			toCreateSecretName := getTemplateVersionNetworkDataSecretName(templateVersion.Name, volume.Name)
			if err := h.copySecret(volume.CloudInitNoCloud.NetworkDataSecretRef.Name, toCreateSecretName, templateVersion); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *vmActionHandler) copySecret(sourceName, targetName string, templateVersion *harvesterv1.VirtualMachineTemplateVersion) error {
	secret, err := h.secretCache.Get(templateVersion.Namespace, sourceName)
	if err != nil {
		return err
	}
	toCreate := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetName,
			Namespace: secret.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: templateVersion.APIVersion,
					Kind:       templateVersion.Kind,
					Name:       templateVersion.Name,
					UID:        templateVersion.UID,
				},
			},
		},
		Data: secret.Data,
	}
	_, err = h.secretClient.Create(toCreate)
	return err

}

// addVolume add a hotplug volume with given volume source and disk name.
func (h *vmActionHandler) addVolume(ctx context.Context, namespace, name string, input AddVolumeInput) error {
	// We only permit volume source from existing PersistentVolumeClaim at this moment.
	// KubeVirt won't check PVC existence so we validate it on our own.
	if _, err := h.pvcCache.Get(namespace, input.VolumeSourceName); err != nil {
		return err
	}

	// Restrict the flexibility of disk options here but future extension may be possible.
	body, err := json.Marshal(kv1.AddVolumeOptions{
		Name: input.DiskName,
		Disk: &kv1.Disk{
			DiskDevice: kv1.DiskDevice{
				Disk: &kv1.DiskTarget{
					// KubeVirt only support SCSI for hotplug volume.
					Bus: "scsi",
				},
			},
		},
		VolumeSource: &kv1.HotplugVolumeSource{
			PersistentVolumeClaim: &kv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: input.VolumeSourceName,
				},
				Hotpluggable: true,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to serialize payload,: %v", err)
	}

	// Ref: https://kubevirt.io/api-reference/v0.44.0/operations.html#_v1vm-addvolume
	return h.virtSubresourceRestClient.
		Put().
		Namespace(namespace).
		Resource(vmResource).
		Name(name).
		SubResource(strings.ToLower(addVolume)).
		Body(body).
		Do(ctx).
		Error()
}

// removeVolume remove a hotplug volume by its disk name
func (h *vmActionHandler) removeVolume(ctx context.Context, namespace, name string, input RemoveVolumeInput) error {
	vmi, err := h.vmiCache.Get(namespace, name)
	if err != nil {
		return err
	}

	// Ensure the existence of the disk. KubeVirt will take care of other cases
	// such as trying to remove a non-hotplug volume.
	found := false
	for _, vol := range vmi.Spec.Volumes {
		if vol.Name == input.DiskName {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("Disk `%s` not found in virtual machine `%s/%s`", input.DiskName, namespace, name)
	}

	body, err := json.Marshal(kv1.RemoveVolumeOptions{
		Name: input.DiskName,
	})

	if err != nil {
		return fmt.Errorf("failed to serialize payload,: %v", err)
	}
	// Ref: https://kubevirt.io/api-reference/v0.44.0/operations.html#_v1vm-removevolume
	return h.virtSubresourceRestClient.
		Put().
		Namespace(namespace).
		Resource(vmResource).
		Name(name).
		SubResource(strings.ToLower(removeVolume)).
		Body(body).
		Do(ctx).
		Error()
}

func (h *vmActionHandler) resetVM(vmName, vmNamespace string) error {
	vm, err := h.vmCache.Get(vmNamespace, vmName)
	if err != nil {
		return err
	}

	if vm.Annotations[util.AnnotationRemovePVC] != "" {
		return fmt.Errorf("vm is reseting")
	}

	updatedVM := vm.DeepCopy()
	if err = h.resetVMSystemDisk(updatedVM); err != nil {
		return err
	}

	if _, err = h.vms.Update(updatedVM); err != nil {
		return err
	}

	if err = h.vmis.Delete(vmNamespace, vmName, &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

func (h *vmActionHandler) resetVMSystemDisk(vm *kv1.VirtualMachine) error {
	//pre check
	disks := vm.Spec.Template.Spec.Domain.Devices.Disks
	if len(disks) == 0 {
		return fmt.Errorf("disks is empty")
	}

	volumes := vm.Spec.Template.Spec.Volumes
	if len(volumes) == 0 {
		return fmt.Errorf("volumes is empty")
	}

	//get systemDisk
	var systemDiskName string
	for _, disk := range disks {
		if disk.BootOrder != nil && *disk.BootOrder == systemDiskBootOrder {
			systemDiskName = disk.Name
			break
		}
	}

	if systemDiskName == "" {
		return fmt.Errorf("can't find disk with bootOrder %d", systemDiskBootOrder)
	}

	//get systemDiskPVC
	var oldSystemDiskPVCName string
	var oldSystemDiskVolumeIndex int
	for index, volume := range volumes {
		if volume.Name == systemDiskName && volume.PersistentVolumeClaim != nil {
			oldSystemDiskPVCName = volume.PersistentVolumeClaim.ClaimName
			oldSystemDiskVolumeIndex = index
			break
		}
	}

	if oldSystemDiskPVCName == "" {
		return fmt.Errorf("can't find volume for disk %s", systemDiskName)
	}

	oldSystemDiskPVC, err := h.pvcCache.Get(vm.Namespace, oldSystemDiskPVCName)
	if err != nil {
		return err
	}

	imageID, ok := oldSystemDiskPVC.Annotations[util.AnnotationImageID]
	if !ok {
		return fmt.Errorf("can't find image id from pvc %s/%s", oldSystemDiskPVC.Namespace, oldSystemDiskPVC.Name)
	}

	//generate volumeClaimTemplates
	newSystemDiskPVCName := generateDiskPVCName(vm.Name, systemDiskName)
	addedPVC := generatePVC(newSystemDiskPVCName, imageID, oldSystemDiskPVC.Spec.Resources)
	newVolumeClaimTemplate, err := generateVolumeClaimTemplates(vm.Annotations[util.AnnotationVolumeClaimTemplates], oldSystemDiskPVCName, addedPVC)
	if err != nil {
		return fmt.Errorf("failed to generate pvc template, error %s", err.Error())
	}

	//reset systemDiskPVC
	vm.Annotations[util.AnnotationRemovePVC] = oldSystemDiskPVCName
	vm.Annotations[util.AnnotationVolumeClaimTemplates] = newVolumeClaimTemplate
	vm.Spec.Template.Spec.Volumes[oldSystemDiskVolumeIndex].PersistentVolumeClaim.ClaimName = newSystemDiskPVCName

	return nil
}

func sanitizeVirtualMachineForTemplateVersion(templateVersionName string, vm *kv1.VirtualMachine) harvesterv1.VirtualMachineSourceSpec {
	sanitizedVm := removeMacAddresses(vm)
	sanitizedVm = replaceCloudInitSecrets(templateVersionName, sanitizedVm)

	return harvesterv1.VirtualMachineSourceSpec{
		ObjectMeta: sanitizedVm.ObjectMeta,
		Spec:       sanitizedVm.Spec,
	}
}

func replaceCloudInitSecrets(templateVersionName string, vm *kv1.VirtualMachine) *kv1.VirtualMachine {
	sanitizedVm := vm.DeepCopy()
	for index, volume := range sanitizedVm.Spec.Template.Spec.Volumes {
		if volume.CloudInitNoCloud == nil {
			continue
		}
		if volume.CloudInitNoCloud.UserDataSecretRef != nil {
			sanitizedVm.Spec.Template.Spec.Volumes[index].CloudInitNoCloud.UserDataSecretRef.Name = getTemplateVersionUserDataSecretName(templateVersionName, volume.Name)
		}
		if volume.CloudInitNoCloud.NetworkDataSecretRef != nil {
			sanitizedVm.Spec.Template.Spec.Volumes[index].CloudInitNoCloud.NetworkDataSecretRef.Name = getTemplateVersionNetworkDataSecretName(templateVersionName, volume.Name)
		}
	}
	return sanitizedVm
}

// removeMacAddresses replaces the mac address of each device interface with an empty string.
// This is because macAddresses are unique, and should not reuse the original's.
func removeMacAddresses(vm *kv1.VirtualMachine) *kv1.VirtualMachine {
	sanitizedVm := vm.DeepCopy()
	for index := range sanitizedVm.Spec.Template.Spec.Domain.Devices.Interfaces {
		sanitizedVm.Spec.Template.Spec.Domain.Devices.Interfaces[index].MacAddress = ""
	}
	return sanitizedVm
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

func getTemplateVersionUserDataSecretName(templateVersionName, volumeName string) string {
	return fmt.Sprintf("templateversion-%s-%s-userdata", templateVersionName, volumeName)
}

func getTemplateVersionNetworkDataSecretName(templateVersionName, volumeName string) string {
	return fmt.Sprintf("templateversion-%s-%s-networkdata", templateVersionName, volumeName)
}

func generateVolumeClaimTemplates(template, deletedPVCName string, added *corev1.PersistentVolumeClaim) (string, error) {
	pvcs := []*corev1.PersistentVolumeClaim{added}

	if template != "" {
		var current []*corev1.PersistentVolumeClaim
		if err := json.Unmarshal([]byte(template), &current); err != nil {
			return "", err
		}

		var index int
		for i, v := range current {
			if v.Name == deletedPVCName {
				index = i
				break
			}
		}

		pvcs = append(pvcs, current[:index]...)
		pvcs = append(pvcs, current[index+1:]...)
	}

	b, err := json.Marshal(pvcs)
	if err != nil {
		return "", nil
	}

	return string(b), nil
}

func generatePVC(name, imageID string, resource corev1.ResourceRequirements) *corev1.PersistentVolumeClaim {
	_, imageName := ref.Parse(imageID)
	storageClassName := getStorageClassFromImage(imageName)
	volumeMode := corev1.PersistentVolumeBlock

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				util.AnnotationImageID: imageID,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClassName,
			VolumeMode:       &volumeMode,
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			Resources: resource,
		},
	}
}

func generateDiskPVCName(vmName, diskName string) string {
	return fmt.Sprintf("%s-%s-%s", vmName, diskName, rand.String(pvcNameRandomStringLen))
}

func getStorageClassFromImage(imageName string) string {
	return fmt.Sprintf("longhorn-%s", imageName)
}
