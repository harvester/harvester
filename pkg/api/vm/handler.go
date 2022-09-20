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
	wranglername "github.com/rancher/wrangler/pkg/name"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/rancher/wrangler/pkg/slice"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/rest"
	kubevirtv1 "kubevirt.io/api/core/v1"

	volumeapi "github.com/harvester/harvester/pkg/api/volume"
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/builder"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	vmResource    = "virtualmachines"
	vmiResource   = "virtualmachineinstances"
	sshAnnotation = "harvesterhci.io/sshNames"
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
	case pauseVM, unpauseVM, softReboot:
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
	case cloneVM:
		var input CloneInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}

		if input.TargetVM == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter targetVm are required")
		}

		if err := h.cloneVM(name, namespace, input); err != nil {
			return err
		}
		return nil
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

func (h *vmActionHandler) startPreCheck(namespace, name string) error {
	vm, err := h.vmCache.Get(namespace, name)
	if err != nil {
		return err
	}

	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			pvcName := volume.PersistentVolumeClaim.PersistentVolumeClaimVolumeSource.ClaimName
			pvcNamespace := vm.Namespace
			pvc, err := h.pvcCache.Get(pvcNamespace, pvcName)
			if err != nil {
				return err
			}
			if volumeapi.IsResizing(pvc) {
				return fmt.Errorf("can not start the VM %s/%s which has a resizing volume %s/%s", vm.Namespace, vm.Name, pvcNamespace, pvcName)
			}
		}
	}

	return nil
}

func (h *vmActionHandler) subresourceOperate(ctx context.Context, resource, namespace, name, subresourece string) error {
	switch subresourece {
	case startVM:
		if err := h.startPreCheck(namespace, name); err != nil {
			return err
		}
	}

	return h.virtSubresourceRestClient.Put().Namespace(namespace).Resource(resource).SubResource(subresourece).Name(name).Do(ctx).Error()
}

func ejectCdRomFromVM(vm *kubevirtv1.VirtualMachine, diskNames []string) error {
	disks := make([]kubevirtv1.Disk, 0, len(vm.Spec.Template.Spec.Domain.Devices.Disks))
	for _, disk := range vm.Spec.Template.Spec.Domain.Devices.Disks {
		if slice.ContainsString(diskNames, disk.Name) {
			if disk.CDRom == nil {
				return errors.New("disk " + disk.Name + " isn't a CD-ROM disk")
			}
			continue
		}
		disks = append(disks, disk)
	}

	volumes := make([]kubevirtv1.Volume, 0, len(vm.Spec.Template.Spec.Volumes))
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

func removeVolumeClaimTemplatesFromVmAnnotation(vm *kubevirtv1.VirtualMachine, toRemoveDiskNames []string) error {
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
	vmim := &kubevirtv1.VirtualMachineInstanceMigration{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: vmName + "-",
			Namespace:    namespace,
		},
		Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{
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
	apiGroup := kubevirtv1.SchemeGroupVersion.Group
	backup := &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.Name,
			Namespace: vmNamespace,
		},
		Spec: harvesterv1.VirtualMachineBackupSpec{
			Source: corev1.TypedLocalObjectReference{
				APIGroup: &apiGroup,
				Kind:     kubevirtv1.VirtualMachineGroupVersionKind.Kind,
				Name:     vmName,
			},
			Type: harvesterv1.Backup,
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
	apiGroup := kubevirtv1.SchemeGroupVersion.Group
	restore := &harvesterv1.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.Name,
			Namespace: vmNamespace,
		},
		Spec: harvesterv1.VirtualMachineRestoreSpec{
			Target: corev1.TypedLocalObjectReference{
				APIGroup: &apiGroup,
				Kind:     kubevirtv1.VirtualMachineGroupVersionKind.Kind,
				Name:     vmName,
			},
			VirtualMachineBackupNamespace: vmNamespace,
			VirtualMachineBackupName:      input.BackupName,
			NewVM:                         false,
		},
	}
	_, err := h.restores.Create(restore)
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

func getMigrationUID(vmi *kubevirtv1.VirtualMachineInstance) string {
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

	return h.createSecrets(vmtv, vm)
}

func (h *vmActionHandler) createSecrets(templateVersion *harvesterv1.VirtualMachineTemplateVersion, vm *kubevirtv1.VirtualMachine) error {
	for index, credential := range vm.Spec.Template.Spec.AccessCredentials {
		if sshPublicKey := credential.SSHPublicKey; sshPublicKey != nil && sshPublicKey.Source.Secret != nil {
			toCreateSecretName := getTemplateVersionSSHPublicKeySecretName(templateVersion.Name, index)
			if err := h.copySecret(sshPublicKey.Source.Secret.SecretName, toCreateSecretName, templateVersion); err != nil {
				return err
			}
		}
		if userPassword := credential.UserPassword; userPassword != nil && userPassword.Source.Secret != nil {
			toCreateSecretName := getTemplateVersionUserPasswordSecretName(templateVersion.Name, index)
			if err := h.copySecret(userPassword.Source.Secret.SecretName, toCreateSecretName, templateVersion); err != nil {
				return err
			}
		}
	}
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
	body, err := json.Marshal(kubevirtv1.AddVolumeOptions{
		Name: input.DiskName,
		Disk: &kubevirtv1.Disk{
			DiskDevice: kubevirtv1.DiskDevice{
				Disk: &kubevirtv1.DiskTarget{
					// KubeVirt only support SCSI for hotplug volume.
					Bus: "scsi",
				},
			},
		},
		VolumeSource: &kubevirtv1.HotplugVolumeSource{
			PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
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

	body, err := json.Marshal(kubevirtv1.RemoveVolumeOptions{
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

// cloneVM creates a VM which uses volume cloning from the source VM.
func (h *vmActionHandler) cloneVM(name string, namespace string, input CloneInput) error {
	vm, err := h.vmCache.Get(namespace, name)
	if err != nil {
		return fmt.Errorf("cannot get vm %s/%s, err: %w", namespace, name, err)
	}
	newVM := getClonedVMYamlFromSourceVM(input.TargetVM, vm)

	newPVCs, secretNameMap, err := h.cloneVolumes(newVM)
	if err != nil {
		return fmt.Errorf("clone volumes error for new vm %s/%s, err %w", newVM.Namespace, newVM.Name, err)
	}
	newPVCsString, err := json.Marshal(newPVCs)
	if err != nil {
		return fmt.Errorf("cannot marshal value %+v, err: %w", newPVCs, err)
	}

	newVM.ObjectMeta.Annotations[util.AnnotationVolumeClaimTemplates] = string(newPVCsString)
	if newVM, err = h.vms.Create(newVM); err != nil {
		return fmt.Errorf("cannot create newVM %+v, err: %w", newVM, err)
	}

	for oldSecretName, newSecretName := range secretNameMap {
		secret, err := h.secretCache.Get(namespace, oldSecretName)
		if err != nil {
			return fmt.Errorf("cannot get secret %s/%s, err: %w", namespace, oldSecretName, err)
		}

		newSecret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      newSecretName,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: newVM.APIVersion,
						Kind:       newVM.Kind,
						Name:       newVM.Name,
						UID:        newVM.UID,
					},
				},
			},
			Data:       secret.Data,
			StringData: secret.StringData,
			Type:       secret.Type,
		}
		if _, err = h.secretClient.Create(&newSecret); err != nil {
			return fmt.Errorf("cannot create a new secret from %s/%s, err: %w", namespace, oldSecretName, err)
		}
	}
	return nil
}

func (h *vmActionHandler) cloneVolumes(newVM *kubevirtv1.VirtualMachine) ([]corev1.PersistentVolumeClaim, map[string]string, error) {
	var (
		err           error
		newPVCs       []corev1.PersistentVolumeClaim
		secretNameMap = map[string]string{} // sourceVM secret name to newVM secret name
	)

	for i, volume := range newVM.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			var pvc *corev1.PersistentVolumeClaim
			pvc, err = h.pvcCache.Get(newVM.Namespace, volume.PersistentVolumeClaim.ClaimName)
			if err != nil {
				return nil, nil, fmt.Errorf("cannot get pvc %s, err: %w", volume.PersistentVolumeClaim.ClaimName, err)
			}

			annotations := map[string]string{}
			if imageId, ok := pvc.Annotations[util.AnnotationImageID]; ok {
				annotations[util.AnnotationImageID] = imageId
			}
			newPVC := corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   newVM.Namespace,
					Name:        names.SimpleNameGenerator.GenerateName(fmt.Sprintf("%s-%s-", newVM.Name, volume.Name)),
					Annotations: annotations,
				},
				Spec: *pvc.Spec.DeepCopy(),
			}
			newPVC.Spec.VolumeName = ""
			newPVC.Spec.DataSource = &corev1.TypedLocalObjectReference{
				Kind: "PersistentVolumeClaim",
				Name: pvc.Name,
			}
			newPVCs = append(newPVCs, newPVC)
			volume.PersistentVolumeClaim.ClaimName = newPVC.Name
		} else if volume.CloudInitNoCloud != nil {
			if volume.CloudInitNoCloud.UserDataSecretRef != nil {
				if _, ok := secretNameMap[volume.CloudInitNoCloud.UserDataSecretRef.Name]; !ok {
					secretNameMap[volume.CloudInitNoCloud.UserDataSecretRef.Name] = names.SimpleNameGenerator.GenerateName(fmt.Sprintf("%s-", newVM.Name))
				}
				volume.CloudInitNoCloud.UserDataSecretRef.Name = secretNameMap[volume.CloudInitNoCloud.UserDataSecretRef.Name]
			}
			if volume.CloudInitNoCloud.NetworkDataSecretRef != nil {
				if _, ok := secretNameMap[volume.CloudInitNoCloud.NetworkDataSecretRef.Name]; !ok {
					secretNameMap[volume.CloudInitNoCloud.NetworkDataSecretRef.Name] = names.SimpleNameGenerator.GenerateName(fmt.Sprintf("%s-", newVM.Name))
				}
				volume.CloudInitNoCloud.NetworkDataSecretRef.Name = secretNameMap[volume.CloudInitNoCloud.NetworkDataSecretRef.Name]
			}
		} else if volume.ContainerDisk != nil {
			continue
		} else {
			return nil, nil, fmt.Errorf("invalid volume %s, only support PersistentVolumeClaim, CloudInitNoCloud, and ContainerDisk", volume.Name)
		}
		newVM.Spec.Template.Spec.Volumes[i] = volume
	}
	return newPVCs, secretNameMap, nil
}

func sanitizeVirtualMachineForTemplateVersion(templateVersionName string, vm *kubevirtv1.VirtualMachine) harvesterv1.VirtualMachineSourceSpec {
	sanitizedVm := removeMacAddresses(vm)
	sanitizedVm = replaceSecrets(templateVersionName, sanitizedVm)

	return harvesterv1.VirtualMachineSourceSpec{
		ObjectMeta: sanitizedVm.ObjectMeta,
		Spec:       sanitizedVm.Spec,
	}
}

func replaceSecrets(templateVersionName string, vm *kubevirtv1.VirtualMachine) *kubevirtv1.VirtualMachine {
	sanitizedVm := vm.DeepCopy()
	for index, credential := range sanitizedVm.Spec.Template.Spec.AccessCredentials {
		if sshPublicKey := credential.SSHPublicKey; sshPublicKey != nil && sshPublicKey.Source.Secret != nil {
			sanitizedVm.Spec.Template.Spec.AccessCredentials[index].SSHPublicKey.Source.Secret.SecretName = getTemplateVersionSSHPublicKeySecretName(templateVersionName, index)
		}
		if userPassword := credential.UserPassword; userPassword != nil && userPassword.Source.Secret != nil {
			sanitizedVm.Spec.Template.Spec.AccessCredentials[index].UserPassword.Source.Secret.SecretName = getTemplateVersionUserPasswordSecretName(templateVersionName, index)
		}
	}
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
func removeMacAddresses(vm *kubevirtv1.VirtualMachine) *kubevirtv1.VirtualMachine {
	sanitizedVm := vm.DeepCopy()
	for index := range sanitizedVm.Spec.Template.Spec.Domain.Devices.Interfaces {
		sanitizedVm.Spec.Template.Spec.Domain.Devices.Interfaces[index].MacAddress = ""
	}
	return sanitizedVm
}

// getSSHKeysFromVMITemplateSpec first checks the given VirtualMachineInstanceTemplateSpec
// for ssh key annotation. If found, it attempts to parse it into a string slice and return
// it.
func getSSHKeysFromVMITemplateSpec(vmitSpec *kubevirtv1.VirtualMachineInstanceTemplateSpec) ([]string, error) {
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
	return wranglername.SafeConcatName("templateversion", templateVersionName, volumeName, "userdata")
}

func getTemplateVersionNetworkDataSecretName(templateVersionName, volumeName string) string {
	return wranglername.SafeConcatName("templateversion", templateVersionName, volumeName, "networkdata")
}

func getTemplateVersionSSHPublicKeySecretName(templateVersionName string, credentialIndex int) string {
	return wranglername.SafeConcatName("templateversion", templateVersionName, fmt.Sprintf("credential-%d", credentialIndex), "sshpublickey")
}

func getTemplateVersionUserPasswordSecretName(templateVersionName string, credentialIndex int) string {
	return wranglername.SafeConcatName("templateversion", templateVersionName, fmt.Sprintf("credential-%d", credentialIndex), "userpassword")
}

func getClonedVMYamlFromSourceVM(newVMName string, sourceVM *kubevirtv1.VirtualMachine) *kubevirtv1.VirtualMachine {
	newVM := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        newVMName,
			Namespace:   sourceVM.Namespace,
			Annotations: map[string]string{},
			Labels:      sourceVM.Labels,
		},
		Spec: *sourceVM.Spec.DeepCopy(),
	}
	newVM.Spec.Template.Spec.Hostname = newVM.Name
	newVM.Spec.Template.ObjectMeta.Labels[builder.LabelKeyVirtualMachineName] = newVM.Name
	for i := range newVM.Spec.Template.Spec.Domain.Devices.Interfaces {
		newVM.Spec.Template.Spec.Domain.Devices.Interfaces[i].MacAddress = ""
	}
	return newVM
}
