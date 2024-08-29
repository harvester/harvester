package backup

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	wranglername "github.com/rancher/wrangler/v3/pkg/name"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
)

const (
	// UI stores the mapping between access credential secret and ssh keys in the annotation `harvesterhci.io/dynamic-ssh-key-names`
	// example: '{"secretname1":["sshkeyname1","sshkeyname2"],"secretname2":["sshkeyname3","sshkeyname4"]}'
	dynamicSSHKeyNamesAnnotation = "harvesterhci.io/dynamic-ssh-key-names"
)

func IsBackupReady(backup *harvesterv1.VirtualMachineBackup) bool {
	return backup.Status.ReadyToUse != nil && *backup.Status.ReadyToUse
}

func IsBackupProgressing(backup *harvesterv1.VirtualMachineBackup) bool {
	return GetVMBackupError(backup) == nil &&
		(backup.Status.ReadyToUse == nil || !*backup.Status.ReadyToUse)
}

func isBackupMissingStatus(backup *harvesterv1.VirtualMachineBackup) bool {
	return backup.Status.SourceSpec == nil || backup.Status.VolumeBackups == nil
}

func IsBackupTargetSame(vmBackupTarget *harvesterv1.BackupTarget, target *settings.BackupTarget) bool {
	return vmBackupTarget.Endpoint == target.Endpoint && vmBackupTarget.BucketName == target.BucketName && vmBackupTarget.BucketRegion == target.BucketRegion
}

func isBackupTargetOnAnnotation(backup *harvesterv1.VirtualMachineBackup) bool {
	return backup.Annotations != nil &&
		(backup.Annotations[backupTargetAnnotation] != "" ||
			backup.Annotations[backupBucketNameAnnotation] != "" ||
			backup.Annotations[backupBucketRegionAnnotation] != "")
}

func isVMRestoreProgressing(vmRestore *harvesterv1.VirtualMachineRestore) bool {
	return vmRestore.Status.Complete == nil || !*vmRestore.Status.Complete
}

func isVMRestoreMissingVolumes(vmRestore *harvesterv1.VirtualMachineRestore) bool {
	return len(vmRestore.Status.VolumeRestores) == 0 ||
		(!IsNewVMOrHasRetainPolicy(vmRestore) && len(vmRestore.Status.DeletedVolumes) == 0)
}

func IsNewVMOrHasRetainPolicy(vmRestore *harvesterv1.VirtualMachineRestore) bool {
	return vmRestore.Spec.NewVM || vmRestore.Spec.DeletionPolicy == harvesterv1.VirtualMachineRestoreRetain
}

func GetVMBackupError(vmBackup *harvesterv1.VirtualMachineBackup) *harvesterv1.Error {
	if vmBackup.Status.Error != nil {
		return vmBackup.Status.Error
	}
	return nil
}

func translateError(e *snapshotv1.VolumeSnapshotError) *harvesterv1.Error {
	if e == nil {
		return nil
	}

	return &harvesterv1.Error{
		Message: e.Message,
		Time:    e.Time,
	}
}

// variable so can be overridden in tests
var currentTime = func() *metav1.Time {
	t := metav1.Now()
	return &t
}

func getRestoreID(vmRestore *harvesterv1.VirtualMachineRestore) string {
	return fmt.Sprintf("%s-%s", vmRestore.Name, vmRestore.UID)
}

func getNewVolumes(vm *kubevirtv1.VirtualMachineSpec, vmRestore *harvesterv1.VirtualMachineRestore) ([]kubevirtv1.Volume, error) {
	var newVolumes = make([]kubevirtv1.Volume, len(vm.Template.Spec.Volumes))
	copy(newVolumes, vm.Template.Spec.Volumes)

	for j, vol := range vm.Template.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			for _, vr := range vmRestore.Status.VolumeRestores {
				if vr.VolumeName != vol.Name {
					continue
				}

				nv := vol.DeepCopy()
				nv.PersistentVolumeClaim.ClaimName = vr.PersistentVolumeClaim.ObjectMeta.Name
				newVolumes[j] = *nv
			}
		}
	}
	return newVolumes, nil
}

func getRestorePVCName(vmRestore *harvesterv1.VirtualMachineRestore, name string) string {
	s := fmt.Sprintf("restore-%s-%s-%s", vmRestore.Spec.VirtualMachineBackupName, vmRestore.UID, name)
	return s
}

func configVMOwner(vm *kubevirtv1.VirtualMachine) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         kubevirtv1.SchemeGroupVersion.String(),
			Kind:               kubevirtv1.VirtualMachineGroupVersionKind.Kind,
			Name:               vm.Name,
			UID:                vm.UID,
			Controller:         pointer.BoolPtr(true),
			BlockOwnerDeletion: pointer.BoolPtr(true),
		},
	}
}

func sanitizeVirtualMachineAnnotationsForRestore(restore *harvesterv1.VirtualMachineRestore, annotations map[string]string) (map[string]string, error) {
	dynamicSSHKeyNames := map[string][]string{}
	newDynamicSSHKeyNames := map[string][]string{}
	for key, value := range annotations {
		if key != dynamicSSHKeyNamesAnnotation {
			continue
		}
		if value == "" {
			continue
		}
		if err := json.Unmarshal([]byte(value), &dynamicSSHKeyNames); err != nil {
			return nil, err
		}
		for secretName, sshKeyNames := range dynamicSSHKeyNames {
			newSecretName := getSecretRefName(restore.Spec.Target.Name, secretName)
			newDynamicSSHKeyNames[newSecretName] = sshKeyNames
		}
		newValue, err := json.Marshal(newDynamicSSHKeyNames)
		if err != nil {
			return nil, err
		}
		annotations[key] = string(newValue)
	}
	return annotations, nil
}

func sanitizeVirtualMachineForRestore(restore *harvesterv1.VirtualMachineRestore, spec kubevirtv1.VirtualMachineInstanceSpec) kubevirtv1.VirtualMachineInstanceSpec {
	for index, credential := range spec.AccessCredentials {
		if sshPublicKey := credential.SSHPublicKey; sshPublicKey != nil && sshPublicKey.Source.Secret != nil {
			spec.AccessCredentials[index].SSHPublicKey.Source.Secret.SecretName = getSecretRefName(restore.Spec.Target.Name, credential.SSHPublicKey.Source.Secret.SecretName)
		}
		if userPassword := credential.UserPassword; userPassword != nil && userPassword.Source.Secret != nil {
			spec.AccessCredentials[index].UserPassword.Source.Secret.SecretName = getSecretRefName(restore.Spec.Target.Name, credential.UserPassword.Source.Secret.SecretName)
		}
	}
	for index, volume := range spec.Volumes {
		if volume.CloudInitNoCloud != nil && volume.CloudInitNoCloud.UserDataSecretRef != nil {
			spec.Volumes[index].CloudInitNoCloud.UserDataSecretRef.Name = getSecretRefName(restore.Spec.Target.Name, volume.CloudInitNoCloud.UserDataSecretRef.Name)
		}
		if volume.CloudInitNoCloud != nil && volume.CloudInitNoCloud.NetworkDataSecretRef != nil {
			spec.Volumes[index].CloudInitNoCloud.NetworkDataSecretRef.Name = getSecretRefName(restore.Spec.Target.Name, volume.CloudInitNoCloud.NetworkDataSecretRef.Name)
		}
	}
	return spec
}

func getSecretRefName(vmName string, secretName string) string {
	// Use secret Hex to avoid the length of secret name exceeding the K8s limit caused by repeated backup and restore
	return fmt.Sprintf("vm-%s-%s-ref", vmName, wranglername.Hex(secretName, 8))
}

func getVMBackupMetadataFilePath(vmBackupNamespace, vmBackupName string) string {
	return filepath.Join(vmBackupMetadataFolderPath, vmBackupNamespace, fmt.Sprintf("%s.cfg", vmBackupName))
}

func getVMImageMetadataFilePath(vmImageNamespace, vmImageName string) string {
	return filepath.Join(vmImageMetadataFolderPath, vmImageNamespace, fmt.Sprintf("%s.cfg", vmImageName))
}
