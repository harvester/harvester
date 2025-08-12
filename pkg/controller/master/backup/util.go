package backup

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/longhorn/backupstore"
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	wranglername "github.com/rancher/wrangler/v3/pkg/name"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctllonghornv2 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
)

const (
	// UI stores the mapping between access credential secret and ssh keys in the annotation `harvesterhci.io/dynamic-ssh-key-names`
	// example: '{"secretname1":["sshkeyname1","sshkeyname2"],"secretname2":["sshkeyname3","sshkeyname4"]}'
	dynamicSSHKeyNamesAnnotation = "harvesterhci.io/dynamic-ssh-key-names"

	// The following definition need to be synced with LH
	// https://github.com/longhorn/longhorn-manager/blob/1f343ee4c467de1264682ecb069d8f2a62850977/csi/controller_server.go#L43-L45
	csiSnapshotTypeLonghornBackingImage     = "bi"
	csiSnapshotTypeLonghornBackup           = "bak"
	deprecatedCSISnapshotTypeLonghornBackup = "bs"
)

func IsBackupReady(backup *harvesterv1.VirtualMachineBackup) bool {
	return backup.Status != nil && backup.Status.ReadyToUse != nil && *backup.Status.ReadyToUse
}

func IsBackupProgressing(backup *harvesterv1.VirtualMachineBackup) bool {
	return GetVMBackupError(backup) == nil &&
		(backup.Status == nil || backup.Status.ReadyToUse == nil || !*backup.Status.ReadyToUse)
}

func isBackupMissingStatus(backup *harvesterv1.VirtualMachineBackup) bool {
	return backup.Status == nil || backup.Status.SourceSpec == nil || backup.Status.VolumeBackups == nil
}

func isBackupTargetOnAnnotation(backup *harvesterv1.VirtualMachineBackup) bool {
	return backup.Annotations != nil &&
		(backup.Annotations[backupTargetAnnotation] != "" ||
			backup.Annotations[backupBucketNameAnnotation] != "" ||
			backup.Annotations[backupBucketRegionAnnotation] != "")
}

func isVMRestoreProgressing(vmRestore *harvesterv1.VirtualMachineRestore) bool {
	return vmRestore.Status == nil || vmRestore.Status.Complete == nil || !*vmRestore.Status.Complete
}

func isVMRestoreMissingVolumes(vmRestore *harvesterv1.VirtualMachineRestore) bool {
	return len(vmRestore.Status.VolumeRestores) == 0 ||
		(!IsNewVMOrHasRetainPolicy(vmRestore) && len(vmRestore.Status.DeletedVolumes) == 0)
}

func IsNewVMOrHasRetainPolicy(vmRestore *harvesterv1.VirtualMachineRestore) bool {
	return vmRestore.Spec.NewVM || vmRestore.Spec.DeletionPolicy == harvesterv1.VirtualMachineRestoreRetain
}

func GetVMBackupError(vmBackup *harvesterv1.VirtualMachineBackup) *harvesterv1.Error {
	if vmBackup.Status != nil && vmBackup.Status.Error != nil {
		return vmBackup.Status.Error
	}
	return nil
}

func newReadyCondition(status corev1.ConditionStatus, reason string, message string) harvesterv1.Condition {
	return harvesterv1.Condition{
		Type:               harvesterv1.BackupConditionReady,
		Status:             status,
		Message:            message,
		Reason:             reason,
		LastTransitionTime: currentTime().Format(time.RFC3339),
	}
}

func newProgressingCondition(status corev1.ConditionStatus, reason string, message string) harvesterv1.Condition {
	return harvesterv1.Condition{
		Type:   harvesterv1.BackupConditionProgressing,
		Status: status,
		// wrangler use Reason to determine whether an object is in error state.
		// ref: https://github.com/rancher/wrangler/blob/6970ad98ba7bd2755312ccfc6540a92bc9a9e316/pkg/summary/summarizers.go#L220-L243
		Reason:             reason,
		Message:            message,
		LastTransitionTime: currentTime().Format(time.RFC3339),
	}
}

func updateBackupCondition(ss *harvesterv1.VirtualMachineBackup, c harvesterv1.Condition) {
	ss.Status.Conditions = updateCondition(ss.Status.Conditions, c)
}

func updateCondition(conditions []harvesterv1.Condition, c harvesterv1.Condition) []harvesterv1.Condition {
	found := false
	for i := range conditions {
		if conditions[i].Type == c.Type {
			if conditions[i].Status != c.Status || (conditions[i].Reason != c.Reason) || (conditions[i].Message != c.Message) {
				conditions[i] = c
			}
			found = true
			break
		}
	}

	if !found {
		conditions = append(conditions, c)
	}

	return conditions
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

func updateRestoreCondition(r *harvesterv1.VirtualMachineRestore, c harvesterv1.Condition) {
	r.Status.Conditions = updateCondition(r.Status.Conditions, c)
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

func getBackupTargetHash(value string) (string, error) {
	hash := sha256.New224()
	if _, err := io.Copy(hash, io.MultiReader(strings.NewReader(value))); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// This util function is from LH
// https://github.com/longhorn/longhorn-manager/blob/1f343ee4c467de1264682ecb069d8f2a62850977/csi/controller_server.go#L1004-L1010
func normalizeCSISnapshotType(cSISnapshotType string) string {
	if cSISnapshotType == deprecatedCSISnapshotTypeLonghornBackup {
		return csiSnapshotTypeLonghornBackup
	}
	return cSISnapshotType
}

// This util function is from LH
// https://github.com/longhorn/longhorn-manager/blob/1f343ee4c467de1264682ecb069d8f2a62850977/csi/controller_server.go#L963-L980
func decodeSnapshotID(snapshotID string) (csiSnapshotType, sourceVolumeName, id string) {
	split := strings.Split(snapshotID, "://")
	if len(split) < 2 {
		return "", "", ""
	}
	csiSnapshotType = split[0]
	if normalizeCSISnapshotType(csiSnapshotType) == csiSnapshotTypeLonghornBackingImage {
		return csiSnapshotTypeLonghornBackingImage, "", ""
	}

	split = strings.Split(split[1], "/")
	if len(split) < 2 {
		return "", "", ""
	}
	sourceVolumeName = split[0]
	id = split[1]
	return normalizeCSISnapshotType(csiSnapshotType), sourceVolumeName, id
}

func getVolumeSnapshotContentName(volumeBackup harvesterv1.VolumeBackup) string {
	return fmt.Sprintf("%s-vsc", *volumeBackup.Name)
}

func checkLHBackup(backupCache ctllonghornv2.BackupCache, name string) (string, error) {
	backup, err := backupCache.Get(util.LonghornSystemNamespaceName, name)
	if err != nil {
		return "", err
	}

	if backup.Status.State != lhv1beta2.BackupStateCompleted {
		return fmt.Sprintf("backup %s is not completed", name), nil
	}

	if backup.DeletionTimestamp != nil {
		return fmt.Sprintf("backup %s is being deleted", name), nil
	}
	return "", nil
}

func checkStorageClass(storageClassCache ctlstoragev1.StorageClassCache, name string) error {
	storageClass, err := storageClassCache.Get(name)
	if err != nil {
		return err
	}

	if storageClass.DeletionTimestamp != nil {
		return fmt.Errorf("storage class %s is being deleted", name)
	}
	return nil
}

func checkVolumeInBackupTarget(vmBackup *harvesterv1.VirtualMachineBackup, volumeBackup *harvesterv1.VolumeBackup, target *settings.BackupTarget) (string, error) {
	volumeName := volumeBackup.PersistentVolumeClaim.Spec.VolumeName
	volumes, err := backupstore.List(volumeName, backuputil.ConstructEndpoint(target), false)
	if err != nil {
		// The backup target may be offline. In this case, we don't want to trigger reconciliation.
		return err.Error(), nil
	}

	if volumes[volumeName] == nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"namespace": vmBackup.Namespace,
			"name":      vmBackup.Name,
			"volume":    volumeName,
		}).Warn("cannot find volume in the backup target for a ready VMBackup, change the VMBackup to not ready")
		volumeBackup.ReadyToUse = ptr.To(false)
		return fmt.Sprintf("cannot find volume %s in the backup target", volumeName), nil
	}
	if volumes[volumeName].Backups[*volumeBackup.LonghornBackupName] == nil {
		logrus.WithFields(logrus.Fields{
			"namespace":      vmBackup.Namespace,
			"name":           vmBackup.Name,
			"volume":         volumeName,
			"longhornBackup": *volumeBackup.LonghornBackupName,
		}).Warn("cannot find longhorn backup in the backup target for a ready VMBackup, change the VMBackup to not ready")
		volumeBackup.ReadyToUse = ptr.To(false)
		return fmt.Sprintf("cannot find longhorn backup %s in the backup target", *volumeBackup.LonghornBackupName), nil
	}
	return "", nil
}
