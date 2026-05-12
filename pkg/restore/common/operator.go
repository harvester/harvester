package common

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

const (
	RestoreNameAnnotation  = "restore.harvesterhci.io/name"
	PvcNameAnnotation      = "pvc.harvesterhci.io/name"
	PvcNameSpaceAnnotation = "pvc.harvesterhci.io/namespace"

	// RestoreProgressBeforeVMStart is the maximum progress value before VM starts
	RestoreProgressBeforeVMStart = 90

	// RestoreProgressComplete is the terminal progress value once a restore finalizes.
	RestoreProgressComplete = 100
)

// VMRestoreOperator provides a unified interface for managing VirtualMachineRestore operations.
// It abstracts restore lifecycle management, volume restore operations, status updates, and persistence.
//
// Method Naming Conventions:
// - Get*: Retrieve data from restore objects (read-only, in-memory)
// - Set*: Modify in-memory objects without persisting to etcd
// - Is*: Check status/conditions (read-only, in-memory)
// - Update*: Persist changes to etcd
// - Configure*: Initialize or update configuration with persistence
// - Init*: Initialize restore components with persistence
//
// Typical Usage Pattern:
//  1. Use Get* methods to inspect current state
//  2. Use Update* methods to persist changes to etcd

type VMRestoreOperator interface {
	// VolumeRestore accessors
	GetVolRestores(vmr *harvesterv1.VirtualMachineRestore) []harvesterv1.VolumeRestore
	GetVolRestore(vmr *harvesterv1.VirtualMachineRestore, index int) *harvesterv1.VolumeRestore
	GetVolRestorePVCName(vr *harvesterv1.VolumeRestore) string
	GetVolRestorePVCNamespace(vr *harvesterv1.VolumeRestore) string
	GetVolRestoreVolumeName(vr *harvesterv1.VolumeRestore) string
	GetVolRestoreVolumeBackupName(vr *harvesterv1.VolumeRestore) string
	GetVolRestoreLHEngineName(vr *harvesterv1.VolumeRestore) *string
	GetVolRestoreVolumeSize(vr *harvesterv1.VolumeRestore) int64
	GetVolRestoreProgress(vr *harvesterv1.VolumeRestore) int

	// VolumeRestore mutators
	SetVolRestoreLHEngineName(vr *harvesterv1.VolumeRestore, name string) error
	SetVolRestoreVolumeSize(vr *harvesterv1.VolumeRestore, size int64) error
	SetVolRestoreProgress(vr *harvesterv1.VolumeRestore, progress int) error

	// VMRestore accessors
	GetName(vmr *harvesterv1.VirtualMachineRestore) string
	GetNamespace(vmr *harvesterv1.VirtualMachineRestore) string
	GetUID(vmr *harvesterv1.VirtualMachineRestore) types.UID
	GetRestoreID(vmr *harvesterv1.VirtualMachineRestore) string
	GetDeletionTimestamp(vmr *harvesterv1.VirtualMachineRestore) *metav1.Time
	GetTargetKind(vmr *harvesterv1.VirtualMachineRestore) string
	GetTargetName(vmr *harvesterv1.VirtualMachineRestore) string
	GetVMBackupName(vmr *harvesterv1.VirtualMachineRestore) string
	GetVMBackupNamespace(vmr *harvesterv1.VirtualMachineRestore) string
	GetDeletedVolumes(vmr *harvesterv1.VirtualMachineRestore) []string
	GetTargetUID(vmr *harvesterv1.VirtualMachineRestore) *types.UID
	GetRestoreTime(vmr *harvesterv1.VirtualMachineRestore) *metav1.Time
	GetProgress(vmr *harvesterv1.VirtualMachineRestore) int

	IsNewVM(vmr *harvesterv1.VirtualMachineRestore) bool
	IsComplete(vmr *harvesterv1.VirtualMachineRestore) bool
	IsProgressing(vmr *harvesterv1.VirtualMachineRestore) bool
	IsRetainPolicy(vmr *harvesterv1.VirtualMachineRestore) bool
	IsKeepMacAddress(vmr *harvesterv1.VirtualMachineRestore) bool
	IsKeepHaltedAfterRestore(vmr *harvesterv1.VirtualMachineRestore) bool
	IsMissingVolumes(vmr *harvesterv1.VirtualMachineRestore) bool
	IsDeleting(vmr *harvesterv1.VirtualMachineRestore) bool
	HasStatus(vmr *harvesterv1.VirtualMachineRestore) bool
	HasOwnerReference(vmr *harvesterv1.VirtualMachineRestore) bool
	IsNewVMOrHasRetainPolicy(vmr *harvesterv1.VirtualMachineRestore) bool

	// VMRestore mutators
	SetTargetUID(vmr *harvesterv1.VirtualMachineRestore, uid *types.UID) error
	SetRestoreTime(vmr *harvesterv1.VirtualMachineRestore, t *metav1.Time) error
	SetProgress(vmr *harvesterv1.VirtualMachineRestore, progress int) error
	SetComplete(vmr *harvesterv1.VirtualMachineRestore, complete bool) error
	SetDeletedVolumes(vmr *harvesterv1.VirtualMachineRestore, volumes []string) error
	SetVolumeRestores(vmr *harvesterv1.VirtualMachineRestore, volumeRestores []harvesterv1.VolumeRestore) error

	SetProcessingCondition(vmr *harvesterv1.VirtualMachineRestore, message string) *harvesterv1.VirtualMachineRestore
	SetCompleteCondition(vmr *harvesterv1.VirtualMachineRestore) *harvesterv1.VirtualMachineRestore
	SetErrorCondition(vmr *harvesterv1.VirtualMachineRestore, err error) *harvesterv1.VirtualMachineRestore

	// Update operations
	Update(old, new *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error)
	UpdateByStatus(old, new *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error)
	UpdateOwnerRefAndTargetUID(vmr *harvesterv1.VirtualMachineRestore, vm *kubevirtv1.VirtualMachine) error

	// Condition operations
	UpdateCondition(old *harvesterv1.VirtualMachineRestore, c harvesterv1.Condition) (*harvesterv1.VirtualMachineRestore, error)
	UpdateError(old *harvesterv1.VirtualMachineRestore, err error) error

	// Target VM operations
	GetTargetVM(vmr *harvesterv1.VirtualMachineRestore) (*kubevirtv1.VirtualMachine, error)
	StartVM(ctx context.Context, vm *kubevirtv1.VirtualMachine) error

	// Initialization operations
	InitVMRestore(vmr *harvesterv1.VirtualMachineRestore) error
	InitVolumesStatus(vmr *harvesterv1.VirtualMachineRestore, vmb *harvesterv1.VirtualMachineBackup) error

	// Volume mapping operations
	MapVolumesToRestoredPVCs(vmr *harvesterv1.VirtualMachineRestore, vmSpec *kubevirtv1.VirtualMachineSpec) ([]kubevirtv1.Volume, error)

	// RectifyProgressBeforeVMStart ensures progress doesn't exceed the threshold before VM starts
	// VMRestore progress stays at 90% before VM start
	RectifyProgressBeforeVMStart(vmr *harvesterv1.VirtualMachineRestore)

	// Secret backup operations
	RestoreSecrets(vmr *harvesterv1.VirtualMachineRestore, vmb *harvesterv1.VirtualMachineBackup, vm *kubevirtv1.VirtualMachine) error
	GetSecretRefName(targetVMName, originalSecretName string) string
	GetVMOwnerReferences(vm *kubevirtv1.VirtualMachine) []metav1.OwnerReference

	// BuildOwnerReference creates an owner reference pointing at the VMRestore.
	// Use this when creating namespaced child resources that should be GC'd
	// when the VMRestore is deleted (e.g. VolumeSnapshot).
	BuildOwnerReference(vmr *harvesterv1.VirtualMachineRestore) metav1.OwnerReference

	// GetVMBackup retrieves and validates the VMBackup referenced in VMRestore
	// It ensures the backup is ready and has a valid source spec
	GetVMBackup(vmr *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineBackup, error)

	// DeleteOldPVCs deletes old PVCs that are being replaced during restore
	// It respects the NewVM and DeletionPolicy settings to determine if PVCs should be deleted
	DeleteOldPVCs(vmr *harvesterv1.VirtualMachineRestore, vm *kubevirtv1.VirtualMachine) error

	// SanitizeVMAnnotations sanitizes VM template annotations for restore
	// It handles dynamic SSH key mappings and secret references
	SanitizeVMAnnotations(vmr *harvesterv1.VirtualMachineRestore, annotations map[string]string) (map[string]string, error)

	// SanitizeVMSpec sanitizes VM instance spec for restore
	// It updates secret references in access credentials and volumes
	SanitizeVMSpec(vmr *harvesterv1.VirtualMachineRestore, spec kubevirtv1.VirtualMachineInstanceSpec) kubevirtv1.VirtualMachineInstanceSpec
}

type vmrestoreOperator struct {
	client    ctlharvesterv1.VirtualMachineRestoreClient
	cache     ctlharvesterv1.VirtualMachineRestoreCache
	vmCache   ctlkubevirtv1.VirtualMachineCache
	vmiCache  ctlkubevirtv1.VirtualMachineInstanceCache
	pvcClient ctlcorev1.PersistentVolumeClaimClient
	pvcCache  ctlcorev1.PersistentVolumeClaimCache

	secretClient ctlcorev1.SecretClient
	secretCache  ctlcorev1.SecretCache

	vmBackupCache ctlharvesterv1.VirtualMachineBackupCache
	vmbo          backupcommon.VMBackupOperator

	virtSubresourceRestClient rest.Interface
}

func GetVMRestoreOperator(
	client ctlharvesterv1.VirtualMachineRestoreClient,
	cache ctlharvesterv1.VirtualMachineRestoreCache,
	vmCache ctlkubevirtv1.VirtualMachineCache,
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache,
	pvcClient ctlcorev1.PersistentVolumeClaimClient,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	secretClient ctlcorev1.SecretClient,
	secretCache ctlcorev1.SecretCache,
	vmBackupCache ctlharvesterv1.VirtualMachineBackupCache,
	vmbo backupcommon.VMBackupOperator,
	virtSubresourceRestClient rest.Interface,
) VMRestoreOperator {
	return &vmrestoreOperator{
		client:                    client,
		cache:                     cache,
		vmCache:                   vmCache,
		vmiCache:                  vmiCache,
		pvcClient:                 pvcClient,
		pvcCache:                  pvcCache,
		secretClient:              secretClient,
		secretCache:               secretCache,
		vmBackupCache:             vmBackupCache,
		vmbo:                      vmbo,
		virtSubresourceRestClient: virtSubresourceRestClient,
	}
}

// VolumeRestore accessors
func (vmro *vmrestoreOperator) GetVolRestores(vmr *harvesterv1.VirtualMachineRestore) []harvesterv1.VolumeRestore {
	return vmr.Status.VolumeRestores
}

func (vmro *vmrestoreOperator) GetVolRestore(vmr *harvesterv1.VirtualMachineRestore, index int) *harvesterv1.VolumeRestore {
	if index < 0 || index >= len(vmr.Status.VolumeRestores) {
		return nil
	}
	return &vmr.Status.VolumeRestores[index]
}

func (vmro *vmrestoreOperator) GetVolRestorePVCName(vr *harvesterv1.VolumeRestore) string {
	if vr == nil {
		return ""
	}
	return vr.PersistentVolumeClaim.ObjectMeta.Name
}

func (vmro *vmrestoreOperator) GetVolRestorePVCNamespace(vr *harvesterv1.VolumeRestore) string {
	if vr == nil {
		return ""
	}
	return vr.PersistentVolumeClaim.ObjectMeta.Namespace
}

func (vmro *vmrestoreOperator) GetVolRestoreVolumeName(vr *harvesterv1.VolumeRestore) string {
	if vr == nil {
		return ""
	}
	return vr.VolumeName
}

func (vmro *vmrestoreOperator) GetVolRestoreVolumeBackupName(vr *harvesterv1.VolumeRestore) string {
	if vr == nil {
		return ""
	}
	return vr.VolumeBackupName
}

func (vmro *vmrestoreOperator) GetVolRestoreLHEngineName(vr *harvesterv1.VolumeRestore) *string {
	if vr == nil {
		return nil
	}
	return vr.LonghornEngineName
}

func (vmro *vmrestoreOperator) GetVolRestoreVolumeSize(vr *harvesterv1.VolumeRestore) int64 {
	if vr == nil {
		return 0
	}
	return vr.VolumeSize
}

func (vmro *vmrestoreOperator) GetVolRestoreProgress(vr *harvesterv1.VolumeRestore) int {
	if vr == nil {
		return 0
	}
	return vr.Progress
}

// VolumeRestore mutators
func (vmro *vmrestoreOperator) SetVolRestoreLHEngineName(vr *harvesterv1.VolumeRestore, name string) error {
	if vr == nil {
		return fmt.Errorf("volume restore is nil")
	}
	vr.LonghornEngineName = ptr.To(name)
	return nil
}

func (vmro *vmrestoreOperator) SetVolRestoreVolumeSize(vr *harvesterv1.VolumeRestore, size int64) error {
	if vr == nil {
		return fmt.Errorf("volume restore is nil")
	}
	vr.VolumeSize = size
	return nil
}

func (vmro *vmrestoreOperator) SetVolRestoreProgress(vr *harvesterv1.VolumeRestore, progress int) error {
	if vr == nil {
		return fmt.Errorf("volume restore is nil")
	}
	vr.Progress = progress
	return nil
}

// VMRestore accessors
func (vmro *vmrestoreOperator) GetName(vmr *harvesterv1.VirtualMachineRestore) string {
	return vmr.Name
}

func (vmro *vmrestoreOperator) GetNamespace(vmr *harvesterv1.VirtualMachineRestore) string {
	return vmr.Namespace
}

func (vmro *vmrestoreOperator) GetUID(vmr *harvesterv1.VirtualMachineRestore) types.UID {
	return vmr.UID
}

func (vmro *vmrestoreOperator) GetRestoreID(vmr *harvesterv1.VirtualMachineRestore) string {
	return fmt.Sprintf("%s-%s", vmr.Name, vmr.UID)
}

func (vmro *vmrestoreOperator) GetDeletionTimestamp(vmr *harvesterv1.VirtualMachineRestore) *metav1.Time {
	return vmr.DeletionTimestamp
}

func (vmro *vmrestoreOperator) GetTargetKind(vmr *harvesterv1.VirtualMachineRestore) string {
	return vmr.Spec.Target.Kind
}

func (vmro *vmrestoreOperator) GetTargetName(vmr *harvesterv1.VirtualMachineRestore) string {
	return vmr.Spec.Target.Name
}

func (vmro *vmrestoreOperator) GetVMBackupName(vmr *harvesterv1.VirtualMachineRestore) string {
	return vmr.Spec.VirtualMachineBackupName
}

func (vmro *vmrestoreOperator) GetVMBackupNamespace(vmr *harvesterv1.VirtualMachineRestore) string {
	return vmr.Spec.VirtualMachineBackupNamespace
}

func (vmro *vmrestoreOperator) GetDeletedVolumes(vmr *harvesterv1.VirtualMachineRestore) []string {
	return vmr.Status.DeletedVolumes
}

func (vmro *vmrestoreOperator) GetTargetUID(vmr *harvesterv1.VirtualMachineRestore) *types.UID {
	return vmr.Status.TargetUID
}

func (vmro *vmrestoreOperator) GetRestoreTime(vmr *harvesterv1.VirtualMachineRestore) *metav1.Time {
	return vmr.Status.RestoreTime
}

func (vmro *vmrestoreOperator) GetProgress(vmr *harvesterv1.VirtualMachineRestore) int {
	return vmr.Status.Progress
}

func (vmro *vmrestoreOperator) IsNewVM(vmr *harvesterv1.VirtualMachineRestore) bool {
	return vmr.Spec.NewVM
}

func (vmro *vmrestoreOperator) IsComplete(vmr *harvesterv1.VirtualMachineRestore) bool {
	return vmr.Status.Complete != nil && *vmr.Status.Complete
}

func (vmro *vmrestoreOperator) IsProgressing(vmr *harvesterv1.VirtualMachineRestore) bool {
	return vmr.Status.Complete == nil || !*vmr.Status.Complete
}

func (vmro *vmrestoreOperator) IsRetainPolicy(vmr *harvesterv1.VirtualMachineRestore) bool {
	return vmr.Spec.DeletionPolicy == harvesterv1.VirtualMachineRestoreRetain
}

func (vmro *vmrestoreOperator) IsKeepMacAddress(vmr *harvesterv1.VirtualMachineRestore) bool {
	return vmr.Spec.KeepMacAddress
}

func (vmro *vmrestoreOperator) IsKeepHaltedAfterRestore(vmr *harvesterv1.VirtualMachineRestore) bool {
	return vmr.Spec.HaltAfterRestore
}

func (vmro *vmrestoreOperator) IsMissingVolumes(vmr *harvesterv1.VirtualMachineRestore) bool {
	volRestores := vmro.GetVolRestores(vmr)
	deletedVolumes := vmro.GetDeletedVolumes(vmr)

	return len(volRestores) == 0 ||
		(!isNewVMOrHasRetainPolicy(vmr) && len(deletedVolumes) == 0)
}

func (vmro *vmrestoreOperator) IsDeleting(vmr *harvesterv1.VirtualMachineRestore) bool {
	return vmr.DeletionTimestamp != nil
}

// HasStatus returns true if the VirtualMachineRestore status has been initialized.
// We use Complete != nil as the marker because InitVMRestore sets it to false.
func (vmro *vmrestoreOperator) HasStatus(vmr *harvesterv1.VirtualMachineRestore) bool {
	return vmr.Status.Complete != nil
}

func (vmro *vmrestoreOperator) HasOwnerReference(vmr *harvesterv1.VirtualMachineRestore) bool {
	return len(vmr.OwnerReferences) > 0
}

func (vmro *vmrestoreOperator) IsNewVMOrHasRetainPolicy(vmr *harvesterv1.VirtualMachineRestore) bool {
	return vmr.Spec.NewVM || vmr.Spec.DeletionPolicy == harvesterv1.VirtualMachineRestoreRetain
}

// VMRestore mutators
func (vmro *vmrestoreOperator) SetTargetUID(vmr *harvesterv1.VirtualMachineRestore, uid *types.UID) error {
	vmr.Status.TargetUID = uid
	return nil
}

func (vmro *vmrestoreOperator) SetRestoreTime(vmr *harvesterv1.VirtualMachineRestore, t *metav1.Time) error {
	vmr.Status.RestoreTime = t
	return nil
}

func (vmro *vmrestoreOperator) SetProgress(vmr *harvesterv1.VirtualMachineRestore, progress int) error {
	vmr.Status.Progress = progress
	return nil
}

func (vmro *vmrestoreOperator) SetComplete(vmr *harvesterv1.VirtualMachineRestore, complete bool) error {
	vmr.Status.Complete = ptr.To(complete)
	return nil
}

func (vmro *vmrestoreOperator) SetDeletedVolumes(vmr *harvesterv1.VirtualMachineRestore, volumes []string) error {
	vmr.Status.DeletedVolumes = volumes
	return nil
}

func (vmro *vmrestoreOperator) SetVolumeRestores(vmr *harvesterv1.VirtualMachineRestore, vrs []harvesterv1.VolumeRestore) error {
	vmr.Status.VolumeRestores = vrs
	return nil
}

// Update operations
func (vmro *vmrestoreOperator) Update(old, newVMr *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error) {
	if reflect.DeepEqual(old, newVMr) {
		return newVMr, nil
	}
	return vmro.client.Update(newVMr)
}

func (vmro *vmrestoreOperator) UpdateByStatus(old, newVMr *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error) {
	if reflect.DeepEqual(old.Status, newVMr.Status) {
		return newVMr, nil
	}
	return vmro.client.Update(newVMr)
}

// UpdateOwnerRefAndTargetUID updates the VMRestore's owner reference and target UID
// It sets the target VM as the owner and records the VM's UID in the restore status
func (vmro *vmrestoreOperator) UpdateOwnerRefAndTargetUID(vmr *harvesterv1.VirtualMachineRestore, vm *kubevirtv1.VirtualMachine) error {
	if vm == nil {
		return fmt.Errorf("target VM cannot be nil")
	}

	targetUID := vmro.GetTargetUID(vmr)
	ownerRefs := vmro.GetVMOwnerReferences(vm)

	// Check if update is needed
	needsUpdate := targetUID == nil || !reflect.DeepEqual(vmr.OwnerReferences, ownerRefs)
	if !needsUpdate {
		return nil
	}

	vmrCpy := vmr.DeepCopy()

	// Set target UID if not already set
	if targetUID == nil {
		if err := vmro.SetTargetUID(vmrCpy, &vm.UID); err != nil {
			return fmt.Errorf("failed to set target UID: %w", err)
		}
	}

	// Set vmRestore owner reference to the target VM
	vmrCpy.SetOwnerReferences(ownerRefs)

	_, err := vmro.Update(vmr, vmrCpy)
	if err != nil {
		return fmt.Errorf("failed to update VMRestore owner reference and target UID: %w", err)
	}
	return nil
}

var currentTime = func() *metav1.Time {
	t := metav1.Now()
	return &t
}

func newReadyCondition(status corev1.ConditionStatus, reason string, message string) harvesterv1.Condition {
	now := currentTime().Format(time.RFC3339)
	return harvesterv1.Condition{
		Type:               harvesterv1.RestoreConditionReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastUpdateTime:     now,
		LastTransitionTime: now,
	}
}

func newProgressingCondition(status corev1.ConditionStatus, reason string, message string) harvesterv1.Condition {
	now := currentTime().Format(time.RFC3339)
	return harvesterv1.Condition{
		Type:               harvesterv1.RestoreConditionProgressing,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastUpdateTime:     now,
		LastTransitionTime: now,
	}
}

func (vmro *vmrestoreOperator) applyCondition(vmr *harvesterv1.VirtualMachineRestore, c harvesterv1.Condition) *harvesterv1.VirtualMachineRestore {
	conditions := vmr.Status.Conditions
	found := false
	for i := range conditions {
		if conditions[i].Type != c.Type {
			continue
		}
		// Per Kubernetes convention, LastTransitionTime only changes when Status flips.
		// Reason/message changes should bump LastUpdateTime only.
		if conditions[i].Status == c.Status {
			c.LastTransitionTime = conditions[i].LastTransitionTime
		}
		if conditions[i].Status != c.Status || conditions[i].Reason != c.Reason || conditions[i].Message != c.Message {
			conditions[i] = c
		}
		found = true
		break
	}

	if !found {
		conditions = append(conditions, c)
	}

	vmr.Status.Conditions = conditions
	return vmr
}

// Status update helpers (no commit)
func (vmro *vmrestoreOperator) SetProcessingCondition(vmr *harvesterv1.VirtualMachineRestore, message string) *harvesterv1.VirtualMachineRestore {
	vmr = vmro.applyCondition(vmr, newProgressingCondition(corev1.ConditionTrue, "", message))
	vmr = vmro.applyCondition(vmr, newReadyCondition(corev1.ConditionFalse, "", message))
	return vmr
}

func (vmro *vmrestoreOperator) SetCompleteCondition(vmr *harvesterv1.VirtualMachineRestore) *harvesterv1.VirtualMachineRestore {
	vmr.Status.RestoreTime = currentTime()
	vmr.Status.Complete = ptr.To(true)
	// Pin progress to 100%. The Longhorn engine's UpdateProgress can return 0
	// once all replicas finish restoring (IsRestoring=false), so per-volume
	// progress can flip back to 0 right before completion.
	vmr.Status.Progress = RestoreProgressComplete
	for i := range vmr.Status.VolumeRestores {
		vmr.Status.VolumeRestores[i].Progress = RestoreProgressComplete
	}
	vmr = vmro.applyCondition(vmr, newProgressingCondition(corev1.ConditionFalse, "", "Operation complete"))
	vmr = vmro.applyCondition(vmr, newReadyCondition(corev1.ConditionTrue, "", "Operation complete"))
	return vmr
}

func (vmro *vmrestoreOperator) SetErrorCondition(vmr *harvesterv1.VirtualMachineRestore, err error) *harvesterv1.VirtualMachineRestore {
	vmr = vmro.applyCondition(vmr, newProgressingCondition(corev1.ConditionFalse, "Error", err.Error()))
	vmr = vmro.applyCondition(vmr, newReadyCondition(corev1.ConditionFalse, "Error", err.Error()))
	return vmr
}

// Condition operations
func (vmro *vmrestoreOperator) UpdateCondition(old *harvesterv1.VirtualMachineRestore, c harvesterv1.Condition) (*harvesterv1.VirtualMachineRestore, error) {
	newVMr := old.DeepCopy()
	newVMr = vmro.applyCondition(newVMr, c)
	return vmro.Update(old, newVMr)
}

func (vmro *vmrestoreOperator) UpdateError(old *harvesterv1.VirtualMachineRestore, err error) error {
	newVMr := old.DeepCopy()
	newVMr = vmro.SetErrorCondition(newVMr, err)
	_, updateErr := vmro.Update(old, newVMr)
	return updateErr
}

// Target VM operations

// GetTargetVM retrieves the target VM for the restore operation
// Returns (nil, nil) if the VM does not exist (not an error condition)
// Returns (nil, error) if there's an error retrieving the VM
func (vmro *vmrestoreOperator) GetTargetVM(vmr *harvesterv1.VirtualMachineRestore) (*kubevirtv1.VirtualMachine, error) {
	if vmr == nil {
		return nil, fmt.Errorf("VirtualMachineRestore cannot be nil")
	}

	if vmr.Spec.Target.Kind != kubevirtv1.VirtualMachineGroupVersionKind.Kind {
		return nil, fmt.Errorf("unsupported target kind: %s", vmr.Spec.Target.Kind)
	}

	vm, err := vmro.vmCache.Get(vmr.Namespace, vmr.Spec.Target.Name)
	if apierrors.IsNotFound(err) {
		logrus.Debugf("target VM %s/%s not found for restore %s", vmr.Namespace, vmr.Spec.Target.Name, vmr.Name)
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get target VM %s/%s: %w", vmr.Namespace, vmr.Spec.Target.Name, err)
	}

	logrus.Debugf("found target VM %s/%s for restore %s", vmr.Namespace, vm.Name, vmr.Name)
	return vm, nil
}

func (vmro *vmrestoreOperator) StartVM(ctx context.Context, vm *kubevirtv1.VirtualMachine) error {
	runStrategy, err := vm.RunStrategy()
	if err != nil {
		return err
	}

	logrus.Infof("starting the vm %s, current state running:%v", vm.Name, runStrategy)

	switch runStrategy {
	case kubevirtv1.RunStrategyAlways:
		return nil
	case kubevirtv1.RunStrategyRerunOnFailure:
		return vmro.handleRerunOnFailure(ctx, vm)
	case kubevirtv1.RunStrategyManual:
		return vmro.handleManualStrategy(ctx, vm)
	case kubevirtv1.RunStrategyHalted:
		return vmro.startVMSubresource(ctx, vm)
	default:
		return nil
	}
}

func (vmro *vmrestoreOperator) handleRerunOnFailure(ctx context.Context, vm *kubevirtv1.VirtualMachine) error {
	vmi, err := vmro.vmiCache.Get(vm.Namespace, vm.Name)
	if apierrors.IsNotFound(err) {
		return vmro.startVMSubresource(ctx, vm)
	}
	if err != nil {
		return err
	}

	if vmi.Status.Phase == kubevirtv1.Succeeded {
		logrus.Infof("restart vmi %s in phase %v", vmi.Name, vmi.Status.Phase)
		return vmro.startVMSubresource(ctx, vm)
	}

	return nil
}

func (vmro *vmrestoreOperator) handleManualStrategy(ctx context.Context, vm *kubevirtv1.VirtualMachine) error {
	vmi, err := vmro.vmiCache.Get(vm.Namespace, vm.Name)
	if err == nil && vmro.isVMIRunning(vmi) {
		// VM is already running
		return nil
	}
	return vmro.startVMSubresource(ctx, vm)
}

func (vmro *vmrestoreOperator) isVMIRunning(vmi *kubevirtv1.VirtualMachineInstance) bool {
	return vmi != nil &&
		!vmi.IsFinal() &&
		vmi.Status.Phase != kubevirtv1.Unknown &&
		vmi.Status.Phase != kubevirtv1.VmPhaseUnset
}

func (vmro *vmrestoreOperator) startVMSubresource(ctx context.Context, vm *kubevirtv1.VirtualMachine) error {
	return vmro.virtSubresourceRestClient.
		Put().
		Namespace(vm.Namespace).
		Resource("virtualmachines").
		SubResource("start").
		Name(vm.Name).
		Do(ctx).
		Error()
}

// Initialization operations

// InitVMRestore initializes the VMRestore status with basic information
func (vmro *vmrestoreOperator) InitVMRestore(old *harvesterv1.VirtualMachineRestore) error {
	newVMr := old.DeepCopy()
	newVMr.Status = harvesterv1.VirtualMachineRestoreStatus{
		Complete: ptr.To(false),
	}
	newVMr = vmro.SetProcessingCondition(newVMr, "Initializing VirtualMachineRestore")

	_, err := vmro.Update(old, newVMr)
	return err
}

// getDeletedVolumes extracts PVC volume names from the current VM referenced in backup's spec.source
func (vmro *vmrestoreOperator) getDeletedVolumes(vmb *harvesterv1.VirtualMachineBackup) ([]string, error) {
	if vmro.vmbo.GetSourceKind(vmb) != kubevirtv1.VirtualMachineGroupVersionKind.Kind {
		return nil, fmt.Errorf("unsupported backup source kind: %s, expected %s", vmro.vmbo.GetSourceKind(vmb), kubevirtv1.VirtualMachineGroupVersionKind.Kind)
	}

	currentVM, err := vmro.vmCache.Get(vmro.vmbo.GetNamespace(vmb), vmro.vmbo.GetSourceName(vmb))
	if apierrors.IsNotFound(err) {
		// VM not found, return empty list
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}

	var deletedVolumes []string
	for _, volume := range currentVM.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			deletedVolumes = append(deletedVolumes, volume.PersistentVolumeClaim.ClaimName)
		}
	}

	return deletedVolumes, nil
}

// getRestorePVCName generates a PVC name for restore.
// Format: restore-<backupName>-<vmrestoreUID>-<diskName>
// This matches the historical Harvester convention: the VMRestore UID
// guarantees uniqueness across repeated restores (including retain-policy
// restores against the same target VM), and the "restore-" prefix marks the
// PVC as a restore artifact rather than something a user created.
func getRestorePVCName(vmr *harvesterv1.VirtualMachineRestore, volumeName string) string {
	return fmt.Sprintf("restore-%s-%s-%s", vmr.Spec.VirtualMachineBackupName, vmr.UID, volumeName)
}

// findExistingVolumeRestore searches for an existing VolumeRestore by volume name
func (vmro *vmrestoreOperator) findExistingVolumeRestore(vmr *harvesterv1.VirtualMachineRestore, volumeName string) *harvesterv1.VolumeRestore {
	for _, vr := range vmro.GetVolRestores(vmr) {
		if volumeName == vr.VolumeName {
			return &vr
		}
	}
	return nil
}

// createNewVolumeRestore creates a new VolumeRestore from a VolumeBackup
func (vmro *vmrestoreOperator) createNewVolumeRestore(vmr *harvesterv1.VirtualMachineRestore, vb *harvesterv1.VolumeBackup) (harvesterv1.VolumeRestore, error) {
	if vb.Name == nil {
		return harvesterv1.VolumeRestore{}, fmt.Errorf("VolumeSnapshotName missing %+v", vb)
	}

	volumeName := vmro.vmbo.GetVolBackupVolumeName(vb)
	return harvesterv1.VolumeRestore{
		VolumeName: volumeName,
		PersistentVolumeClaim: harvesterv1.PersistentVolumeClaimSourceSpec{
			ObjectMeta: metav1.ObjectMeta{
				Name:      getRestorePVCName(vmr, volumeName),
				Namespace: vmro.GetNamespace(vmr),
			},
			Spec: vmro.vmbo.GetVolBackupPVCSpec(vb),
		},
		VolumeBackupName: *vmro.vmbo.GetVolBackupName(vb),
	}, nil
}

// buildVolumeRestoresFromBackup creates an array of VolumeRestore objects from the backup
// It reuses existing VolumeRestores if they already exist, or creates new ones
func (vmro *vmrestoreOperator) buildVolumeRestoresFromBackup(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
) ([]harvesterv1.VolumeRestore, error) {
	vrs := make([]harvesterv1.VolumeRestore, 0, len(vmb.Status.VolumeBackups))

	for _, vb := range vmb.Status.VolumeBackups {
		// Check if we already have a VolumeRestore for this volume
		if existingVR := vmro.findExistingVolumeRestore(vmr, vb.VolumeName); existingVR != nil {
			vrs = append(vrs, *existingVR)
			continue
		}

		// Create a new VolumeRestore
		newVR, err := vmro.createNewVolumeRestore(vmr, &vb)
		if err != nil {
			return nil, err
		}
		vrs = append(vrs, newVR)
	}
	return vrs, nil
}

// isNewVMOrHasRetainPolicy checks if restore is for a new VM or has retain policy
func isNewVMOrHasRetainPolicy(vmr *harvesterv1.VirtualMachineRestore) bool {
	return vmr.Spec.NewVM || vmr.Spec.DeletionPolicy == harvesterv1.VirtualMachineRestoreRetain
}

// InitVolumesStatus initializes volume restores and deleted volumes in the status
func (vmro *vmrestoreOperator) InitVolumesStatus(old *harvesterv1.VirtualMachineRestore, vmb *harvesterv1.VirtualMachineBackup) error {
	vmrCpy := old.DeepCopy()

	if vmrCpy.Status.VolumeRestores == nil {
		vrs, err := vmro.buildVolumeRestoresFromBackup(vmrCpy, vmb)
		if err != nil {
			return err
		}
		vmrCpy.Status.VolumeRestores = vrs
	}

	deletedVolumes := vmro.GetDeletedVolumes(vmrCpy)
	if !isNewVMOrHasRetainPolicy(vmrCpy) && len(deletedVolumes) == 0 {
		deletedVolumes, err := vmro.getDeletedVolumes(vmb)
		if err != nil {
			return err
		}
		vmrCpy.Status.DeletedVolumes = deletedVolumes
	}

	_, err := vmro.Update(old, vmrCpy)
	return err
}

// MapVolumesToRestoredPVCs maps volume names to restored PVC names based on VolumeRestores
// It takes a VM spec and returns a new volume list with updated PVC claim names
func (vmro *vmrestoreOperator) MapVolumesToRestoredPVCs(
	vmr *harvesterv1.VirtualMachineRestore,
	vmSpec *kubevirtv1.VirtualMachineSpec,
) ([]kubevirtv1.Volume, error) {
	newVolumes := make([]kubevirtv1.Volume, len(vmSpec.Template.Spec.Volumes))
	copy(newVolumes, vmSpec.Template.Spec.Volumes)

	vrs := vmro.GetVolRestores(vmr)

	for j, vol := range vmSpec.Template.Spec.Volumes {
		// Skip non-PVC volumes
		if vol.PersistentVolumeClaim == nil {
			continue
		}

		// Find matching VolumeRestore for this volume
		restoredPVCName := vmro.findRestoredPVCName(&vol, vrs)
		if restoredPVCName == "" {
			continue
		}

		// Update volume with restored PVC name
		nv := vol.DeepCopy()
		nv.PersistentVolumeClaim.ClaimName = restoredPVCName
		newVolumes[j] = *nv
	}

	return newVolumes, nil
}

// findRestoredPVCName finds the restored PVC name for a given volume from VolumeRestores
func (vmro *vmrestoreOperator) findRestoredPVCName(vol *kubevirtv1.Volume, vrs []harvesterv1.VolumeRestore) string {
	for _, vr := range vrs {
		if vmro.GetVolRestoreVolumeName(&vr) == vol.Name {
			return vmro.GetVolRestorePVCName(&vr)
		}
	}
	return ""
}

// RectifyProgressBeforeVMStart ensures that the restore progress doesn't exceed
// the threshold (90%) before the VM starts. This provides better UX by preventing
// the progress from jumping to 100% prematurely when volumes are ready but VM is not yet started.
func (vmro *vmrestoreOperator) RectifyProgressBeforeVMStart(vmr *harvesterv1.VirtualMachineRestore) {
	currentProgress := vmro.GetProgress(vmr)
	if currentProgress <= RestoreProgressBeforeVMStart {
		return
	}

	_ = vmro.SetProgress(vmr, RestoreProgressBeforeVMStart)
}

// Secret backup operations

// RestoreSecrets restores secrets from the backup to the target namespace
// It handles both new VM (with renamed secrets) and existing VM (with original secret names) scenarios
func (vmro *vmrestoreOperator) RestoreSecrets(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
	vm *kubevirtv1.VirtualMachine,
) error {
	ownerRefs := vmro.GetVMOwnerReferences(vm)
	isNewVM := vmro.IsNewVM(vmr)
	targetName := vmro.GetTargetName(vmr)
	namespace := vmro.GetNamespace(vmr)

	if !isNewVM {
		// Restore secrets with original names for existing VM
		for _, secretBackup := range vmb.Status.SecretBackups {
			if err := vmro.buildAndApplySecret(namespace, secretBackup.Name, secretBackup.Data, ownerRefs); err != nil {
				return err
			}
		}
		return nil
	}

	// Create new secrets with renamed names for new VM
	for _, secretBackup := range vmb.Status.SecretBackups {
		newSecretName := vmro.GetSecretRefName(targetName, secretBackup.Name)
		if err := vmro.buildAndApplySecret(namespace, newSecretName, secretBackup.Data, ownerRefs); err != nil {
			return err
		}
	}
	return nil
}

// buildAndApplySecret builds a secret object and creates or updates it in the cluster
func (vmro *vmrestoreOperator) buildAndApplySecret(
	namespace, name string,
	data map[string][]byte,
	ownerRefs []metav1.OwnerReference,
) error {
	secret, err := vmro.secretCache.Get(namespace, name)
	if apierrors.IsNotFound(err) {
		logrus.Infof("create secret %s/%s", namespace, name)
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       namespace,
				OwnerReferences: ownerRefs,
			},
			Data: data,
		}
		if _, err := vmro.secretClient.Create(secret); err != nil {
			return err
		}
		return nil
	}

	if err != nil {
		return err
	}

	secretCpy := secret.DeepCopy()
	secretCpy.Data = data

	if !reflect.DeepEqual(secret, secretCpy) {
		logrus.Infof("update secret %s/%s", namespace, name)
		if _, err := vmro.secretClient.Update(secretCpy); err != nil {
			return err
		}
	}
	return nil
}

// GetSecretRefName generates a new secret name for a restored VM
// Format: <targetVMName>-<originalSecretName>
func (vmro *vmrestoreOperator) GetSecretRefName(targetVMName, originalSecretName string) string {
	return fmt.Sprintf("%s-%s", targetVMName, originalSecretName)
}

// GetVMOwnerReferences returns owner references for a VM
func (vmro *vmrestoreOperator) GetVMOwnerReferences(vm *kubevirtv1.VirtualMachine) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         kubevirtv1.SchemeGroupVersion.String(),
			Kind:               kubevirtv1.VirtualMachineGroupVersionKind.Kind,
			Name:               vm.Name,
			UID:                vm.UID,
			Controller:         ptr.To(true),
			BlockOwnerDeletion: ptr.To(true),
		},
	}
}

// BuildOwnerReference returns an owner reference pointing at the VMRestore.
// Use a literal Kind because objects fetched from the cache often have an
// empty TypeMeta. BlockOwnerDeletion forces foreground GC so the VMRestore
// is not removed from etcd until child resources (e.g. the cross-namespace
// VolumeSnapshot we create) are gone — matches pre-patch behavior.
func (vmro *vmrestoreOperator) BuildOwnerReference(vmr *harvesterv1.VirtualMachineRestore) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion:         harvesterv1.SchemeGroupVersion.String(),
		Kind:               "VirtualMachineRestore",
		Name:               vmr.Name,
		UID:                vmr.UID,
		BlockOwnerDeletion: ptr.To(true),
	}
}

// GetVMBackup retrieves and validates the VMBackup referenced in VMRestore
// It ensures the backup is ready and has a valid source spec
func (vmro *vmrestoreOperator) GetVMBackup(vmr *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineBackup, error) {
	vmb, err := vmro.vmBackupCache.Get(vmr.Spec.VirtualMachineBackupNamespace, vmr.Spec.VirtualMachineBackupName)
	if err != nil {
		return nil, err
	}

	if !vmro.vmbo.IsReady(vmb) {
		return nil, fmt.Errorf("VMBackup %s/%s is not ready", vmb.Namespace, vmb.Name)
	}

	if vmro.vmbo.GetSourceKind(vmb) != kubevirtv1.VirtualMachineGroupVersionKind.Kind {
		return nil, fmt.Errorf("unsupported backup source kind: %s", vmro.vmbo.GetSourceKind(vmb))
	}

	return vmb, nil
}

// DeleteOldPVCs deletes the original PVCs replaced by this restore.
// Iterate Status.DeletedVolumes directly — the VM's current volume list has
// already been swapped to the new restored PVCs by updateExistingVM, so it no
// longer references the names we need to delete.
func (vmro *vmrestoreOperator) DeleteOldPVCs(vmr *harvesterv1.VirtualMachineRestore, vm *kubevirtv1.VirtualMachine) error {
	if vmro.IsNewVM(vmr) || vmro.IsRetainPolicy(vmr) {
		return nil
	}

	namespace := vm.Namespace
	for _, pvcName := range vmro.GetDeletedVolumes(vmr) {
		pvc, err := vmro.pvcCache.Get(namespace, pvcName)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to get PVC %s/%s: %w", namespace, pvcName, err)
		}

		logrus.Infof("deleting old PVC %s/%s for restore %s/%s", pvc.Namespace, pvc.Name, vmr.Namespace, vmr.Name)
		if err := vmro.pvcClient.Delete(pvc.Namespace, pvc.Name, &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete PVC %s/%s: %w", pvc.Namespace, pvc.Name, err)
		}
	}
	return nil
}

// SanitizeVMAnnotations sanitizes VM template annotations for restore
// It handles dynamic SSH key mappings and secret references
func (vmro *vmrestoreOperator) SanitizeVMAnnotations(
	vmr *harvesterv1.VirtualMachineRestore,
	annotations map[string]string,
) (map[string]string, error) {
	const dynamicSSHKeyNamesAnnotation = "harvesterhci.io/dynamic-ssh-key-names"

	value, ok := annotations[dynamicSSHKeyNamesAnnotation]
	if !ok || value == "" {
		return annotations, nil
	}

	var dynamicSSHKeyNames map[string][]string
	if err := json.Unmarshal([]byte(value), &dynamicSSHKeyNames); err != nil {
		return nil, err
	}

	newDynamicSSHKeyNames := vmro.remapSecretNames(vmr, dynamicSSHKeyNames)

	newValue, err := json.Marshal(newDynamicSSHKeyNames)
	if err != nil {
		return nil, err
	}

	annotations[dynamicSSHKeyNamesAnnotation] = string(newValue)
	return annotations, nil
}

// remapSecretNames creates a new map with renamed secret names for the target VM
func (vmro *vmrestoreOperator) remapSecretNames(
	vmr *harvesterv1.VirtualMachineRestore,
	secretMapping map[string][]string,
) map[string][]string {
	targetName := vmro.GetTargetName(vmr)
	result := make(map[string][]string, len(secretMapping))

	for secretName, sshKeyNames := range secretMapping {
		newSecretName := vmro.GetSecretRefName(targetName, secretName)
		result[newSecretName] = sshKeyNames
	}

	return result
}

// SanitizeVMSpec sanitizes VM instance spec for restore
// It updates secret references in access credentials and volumes
func (vmro *vmrestoreOperator) SanitizeVMSpec(vmr *harvesterv1.VirtualMachineRestore, spec kubevirtv1.VirtualMachineInstanceSpec) kubevirtv1.VirtualMachineInstanceSpec {
	targetName := vmro.GetTargetName(vmr)

	vmro.sanitizeAccessCredentials(targetName, spec.AccessCredentials)
	vmro.sanitizeVolumes(targetName, spec.Volumes)

	return spec
}

// sanitizeAccessCredentials updates secret references in access credentials
func (vmro *vmrestoreOperator) sanitizeAccessCredentials(targetName string, credentials []kubevirtv1.AccessCredential) {
	for i := range credentials {
		vmro.sanitizeSSHPublicKey(targetName, &credentials[i])
		vmro.sanitizeUserPassword(targetName, &credentials[i])
	}
}

// sanitizeSSHPublicKey updates SSH public key secret reference
func (vmro *vmrestoreOperator) sanitizeSSHPublicKey(targetName string, credential *kubevirtv1.AccessCredential) {
	if credential.SSHPublicKey == nil || credential.SSHPublicKey.Source.Secret == nil {
		return
	}

	originalName := credential.SSHPublicKey.Source.Secret.SecretName
	credential.SSHPublicKey.Source.Secret.SecretName = vmro.GetSecretRefName(targetName, originalName)
}

// sanitizeUserPassword updates user password secret reference
func (vmro *vmrestoreOperator) sanitizeUserPassword(targetName string, credential *kubevirtv1.AccessCredential) {
	if credential.UserPassword == nil || credential.UserPassword.Source.Secret == nil {
		return
	}

	originalName := credential.UserPassword.Source.Secret.SecretName
	credential.UserPassword.Source.Secret.SecretName = vmro.GetSecretRefName(targetName, originalName)
}

// sanitizeVolumes updates secret references in volumes
func (vmro *vmrestoreOperator) sanitizeVolumes(targetName string, volumes []kubevirtv1.Volume) {
	for i := range volumes {
		vmro.sanitizeCloudInitSecrets(targetName, &volumes[i])
	}
}

// sanitizeCloudInitSecrets updates cloud-init secret references in a volume
func (vmro *vmrestoreOperator) sanitizeCloudInitSecrets(targetName string, volume *kubevirtv1.Volume) {
	if volume.CloudInitNoCloud == nil {
		return
	}

	if volume.CloudInitNoCloud.UserDataSecretRef != nil {
		originalName := volume.CloudInitNoCloud.UserDataSecretRef.Name
		volume.CloudInitNoCloud.UserDataSecretRef.Name = vmro.GetSecretRefName(targetName, originalName)
	}

	if volume.CloudInitNoCloud.NetworkDataSecretRef != nil {
		originalName := volume.CloudInitNoCloud.NetworkDataSecretRef.Name
		volume.CloudInitNoCloud.NetworkDataSecretRef.Name = vmro.GetSecretRefName(targetName, originalName)
	}
}
