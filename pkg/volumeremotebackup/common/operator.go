package common

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/rancher/wrangler/v3/pkg/condition"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	ConditionReady condition.Cond = "Ready"
)

var (
	ErrRetryLater = errors.New("retry later error")
)

func IsRetryLater(err error) bool {
	return errors.Is(err, ErrRetryLater)
}

// currentTime is extracted as a variable for testing
var currentTime = func() *metav1.Time {
	t := metav1.Now()
	return &t
}

// newReadyCondition creates a new Ready condition
func newReadyCondition(status corev1.ConditionStatus, reason string, message string) harvesterv1.Condition {
	return harvesterv1.Condition{
		Type:               ConditionReady,
		Status:             status,
		Message:            message,
		Reason:             reason,
		LastTransitionTime: currentTime().Format(time.RFC3339),
	}
}

// applyConditionToStatus applies a condition to a conditions slice
func applyConditionToStatus(conditions []harvesterv1.Condition, c harvesterv1.Condition) []harvesterv1.Condition {
	for i := range conditions {
		if conditions[i].Type != c.Type {
			continue
		}

		// Condition found - check if anything has changed
		statusChanged := conditions[i].Status != c.Status
		reasonChanged := conditions[i].Reason != c.Reason
		messageChanged := conditions[i].Message != c.Message

		// No changes, return as-is
		if !statusChanged && !reasonChanged && !messageChanged {
			return conditions
		}

		// Preserve the existing timestamp if only reason/message changed (not status)
		if !statusChanged {
			c.LastTransitionTime = conditions[i].LastTransitionTime
		}

		conditions[i] = c
		return conditions
	}

	// Condition not found, append it
	return append(conditions, c)
}

type BackupOperator interface {
	// Getters
	GetNamespace(vrb *harvesterv1.VolumeRemoteBackup) string
	GetName(vrb *harvesterv1.VolumeRemoteBackup) string
	GetKind(vrb *harvesterv1.VolumeRemoteBackup) string
	GetUID(vrb *harvesterv1.VolumeRemoteBackup) types.UID
	GetType(vrb *harvesterv1.VolumeRemoteBackup) harvesterv1.VolumeRemoteBackupType
	GetSource(vrb *harvesterv1.VolumeRemoteBackup) string
	GetHandle(vrb *harvesterv1.VolumeRemoteBackup) string
	GetSuccess(vrb *harvesterv1.VolumeRemoteBackup) bool
	GetSnapshotClassInfo(vrb *harvesterv1.VolumeRemoteBackup) (*settings.CSIDriverInfo, error)
	GetCSIProvider(vrb *harvesterv1.VolumeRemoteBackup) string
	GetSourceSpec(vrb *harvesterv1.VolumeRemoteBackup) corev1.PersistentVolumeClaimSpec

	// Update
	Update(oldVrb, newVrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error)
	Finalize(vrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error)
	UpdateCSIProvider(vrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error)
	UpdateSourceSpec(vrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error)

	// Setters
	SetSuccess(vrb *harvesterv1.VolumeRemoteBackup, success bool) (*harvesterv1.VolumeRemoteBackup, error)
	SetHandle(vrb *harvesterv1.VolumeRemoteBackup, handle string) (*harvesterv1.VolumeRemoteBackup, error)
	SetReady(vrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error)
	SetProcessing(vrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error)
	SetError(vrb *harvesterv1.VolumeRemoteBackup, err error) (*harvesterv1.VolumeRemoteBackup, error)
}

type backupOperator struct {
	client       ctlharvesterv1.VolumeRemoteBackupClient
	pvcCache     ctlcorev1.PersistentVolumeClaimCache
	scCache      ctlstoragev1.StorageClassCache
	settingCache ctlharvesterv1.SettingCache
}

func NewBackupOperator(
	client ctlharvesterv1.VolumeRemoteBackupClient,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	scCache ctlstoragev1.StorageClassCache,
	settingCache ctlharvesterv1.SettingCache,
) BackupOperator {
	return &backupOperator{
		client:       client,
		pvcCache:     pvcCache,
		scCache:      scCache,
		settingCache: settingCache,
	}
}

type backupMutator func(*harvesterv1.VolumeRemoteBackup) error

func (bo *backupOperator) GetNamespace(vrb *harvesterv1.VolumeRemoteBackup) string {
	return vrb.Namespace
}

func (bo *backupOperator) GetName(vrb *harvesterv1.VolumeRemoteBackup) string {
	return vrb.Name
}

func (bo *backupOperator) GetKind(vrb *harvesterv1.VolumeRemoteBackup) string {
	return vrb.Kind
}

func (bo *backupOperator) GetUID(vrb *harvesterv1.VolumeRemoteBackup) types.UID {
	return vrb.UID
}

func (bo *backupOperator) GetSuccess(vrb *harvesterv1.VolumeRemoteBackup) bool {
	return vrb.Status.Success
}

func (bo *backupOperator) Update(oldVrb, newVrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error) {
	if reflect.DeepEqual(oldVrb, newVrb) {
		return newVrb, nil
	}
	return bo.client.Update(newVrb)
}

func (bo *backupOperator) apply(
	vrb *harvesterv1.VolumeRemoteBackup,
	mutators ...backupMutator,
) (*harvesterv1.VolumeRemoteBackup, error) {
	newVrb := vrb.DeepCopy()
	for _, mutator := range mutators {
		if err := mutator(newVrb); err != nil {
			return vrb, err
		}
	}
	return bo.Update(vrb, newVrb)
}

func (bo *backupOperator) GetType(vrb *harvesterv1.VolumeRemoteBackup) harvesterv1.VolumeRemoteBackupType {
	return vrb.Spec.Type
}

func (bo *backupOperator) GetSource(vrb *harvesterv1.VolumeRemoteBackup) string {
	return vrb.Spec.Source
}

func (bo *backupOperator) GetHandle(vrb *harvesterv1.VolumeRemoteBackup) string {
	return vrb.Status.Handle
}

func (bo *backupOperator) GetSnapshotClassInfo(vrb *harvesterv1.VolumeRemoteBackup) (*settings.CSIDriverInfo, error) {
	srcPVC, err := bo.pvcCache.Get(bo.GetNamespace(vrb), bo.GetSource(vrb))
	if err != nil {
		return nil, err
	}

	// Get CSI driver from PVC
	provider := util.GetProvisionedPVCProvisioner(srcPVC, bo.scCache)
	if provider == "" {
		return nil, fmt.Errorf("CSI driver not found for PVC %s/%s", bo.GetNamespace(vrb), bo.GetSource(vrb))
	}

	// Load CSI driver config
	csiDriverConfig, err := util.LoadCSIDriverConfig(bo.settingCache)
	if err != nil {
		return nil, err
	}

	// Get driver info
	driverInfo, ok := csiDriverConfig[provider]
	if !ok {
		return nil, fmt.Errorf("CSI driver %q not found in CSIDriverInfo settings for PVC %s/%s",
			provider, bo.GetNamespace(vrb), bo.GetSource(vrb))
	}
	return &driverInfo, nil
}

func (bo *backupOperator) GetCSIProvider(vrb *harvesterv1.VolumeRemoteBackup) string {
	return vrb.Status.CSIProvider
}

func (bo *backupOperator) GetSourceSpec(vrb *harvesterv1.VolumeRemoteBackup) corev1.PersistentVolumeClaimSpec {
	return vrb.Status.SourceSpec
}

func (bo *backupOperator) SetHandle(vrb *harvesterv1.VolumeRemoteBackup, handle string) (*harvesterv1.VolumeRemoteBackup, error) {
	return bo.apply(vrb, bo.withHandle(handle))
}

func (bo *backupOperator) Finalize(vrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error) {
	return bo.apply(
		vrb,
		bo.withCondition(newReadyCondition(corev1.ConditionTrue, "", "Operation complete")),
		bo.withSuccess(true),
		bo.withCSIProvider(),
		bo.withSourceSpec(),
	)
}

func (bo *backupOperator) UpdateCSIProvider(vrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error) {
	return bo.apply(vrb, bo.withCSIProvider())
}

func (bo *backupOperator) UpdateSourceSpec(vrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error) {
	return bo.apply(vrb, bo.withSourceSpec())
}

func (bo *backupOperator) SetSuccess(vrb *harvesterv1.VolumeRemoteBackup, success bool) (*harvesterv1.VolumeRemoteBackup, error) {
	return bo.apply(vrb, bo.withSuccess(success))
}

func (bo *backupOperator) SetReady(vrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error) {
	return bo.setCondition(vrb, newReadyCondition(corev1.ConditionTrue, "", "Operation complete"))
}

func (bo *backupOperator) SetProcessing(vrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error) {
	return bo.setCondition(vrb, newReadyCondition(corev1.ConditionFalse, "", "Operation in progress"))
}

func (bo *backupOperator) SetError(vrb *harvesterv1.VolumeRemoteBackup, err error) (*harvesterv1.VolumeRemoteBackup, error) {
	return bo.setCondition(vrb, newReadyCondition(corev1.ConditionFalse, "", err.Error()))
}

func (bo *backupOperator) setCondition(vrb *harvesterv1.VolumeRemoteBackup, c harvesterv1.Condition) (*harvesterv1.VolumeRemoteBackup, error) {
	return bo.apply(vrb, bo.withCondition(c))
}

func (bo *backupOperator) withHandle(handle string) backupMutator {
	return func(vrb *harvesterv1.VolumeRemoteBackup) error {
		vrb.Status.Handle = handle
		return nil
	}
}

func (bo *backupOperator) withSuccess(success bool) backupMutator {
	return func(vrb *harvesterv1.VolumeRemoteBackup) error {
		vrb.Status.Success = success
		return nil
	}
}

func (bo *backupOperator) withCondition(c harvesterv1.Condition) backupMutator {
	return func(vrb *harvesterv1.VolumeRemoteBackup) error {
		vrb.Status.Conditions = applyConditionToStatus(vrb.Status.Conditions, c)
		return nil
	}
}

func (bo *backupOperator) withCSIProvider() backupMutator {
	return func(vrb *harvesterv1.VolumeRemoteBackup) error {
		srcPVC, err := bo.pvcCache.Get(bo.GetNamespace(vrb), bo.GetSource(vrb))
		if err != nil {
			return err
		}

		provider := util.GetProvisionedPVCProvisioner(srcPVC, bo.scCache)
		if provider == "" {
			return fmt.Errorf("CSI driver not found for PVC %s/%s", bo.GetNamespace(vrb), bo.GetSource(vrb))
		}

		vrb.Status.CSIProvider = provider
		return nil
	}
}

func (bo *backupOperator) withSourceSpec() backupMutator {
	return func(vrb *harvesterv1.VolumeRemoteBackup) error {
		srcPVC, err := bo.pvcCache.Get(bo.GetNamespace(vrb), bo.GetSource(vrb))
		if err != nil {
			return err
		}

		vrb.Status.SourceSpec = srcPVC.Spec
		return nil
	}
}

type RestoreOperator interface {
	// Getters
	GetNamespace(vrr *harvesterv1.VolumeRemoteRestore) string
	GetName(vrr *harvesterv1.VolumeRemoteRestore) string
	GetKind(vrr *harvesterv1.VolumeRemoteRestore) string
	GetUID(vrr *harvesterv1.VolumeRemoteRestore) types.UID
	GetType(vrr *harvesterv1.VolumeRemoteRestore) harvesterv1.VolumeRemoteRestoreType
	GetFrom(vrr *harvesterv1.VolumeRemoteRestore) string
	GetSuccess(vrr *harvesterv1.VolumeRemoteRestore) bool
	GetSnapshotClassInfo(vrr *harvesterv1.VolumeRemoteRestore) (*settings.CSIDriverInfo, error)

	// Update
	Update(oldVrr, newVrr *harvesterv1.VolumeRemoteRestore) (*harvesterv1.VolumeRemoteRestore, error)

	// Setters
	SetSuccess(vrr *harvesterv1.VolumeRemoteRestore, success bool) (*harvesterv1.VolumeRemoteRestore, error)
	SetReady(vrr *harvesterv1.VolumeRemoteRestore) (*harvesterv1.VolumeRemoteRestore, error)
	SetProcessing(vrr *harvesterv1.VolumeRemoteRestore) (*harvesterv1.VolumeRemoteRestore, error)
	SetError(vrr *harvesterv1.VolumeRemoteRestore, err error) (*harvesterv1.VolumeRemoteRestore, error)
}

type restoreOperator struct {
	client       ctlharvesterv1.VolumeRemoteRestoreClient
	settingCache ctlharvesterv1.SettingCache
	vrbCache     ctlharvesterv1.VolumeRemoteBackupCache
	bo           BackupOperator
}

func NewRestoreOperator(
	client ctlharvesterv1.VolumeRemoteRestoreClient,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	scCache ctlstoragev1.StorageClassCache,
	settingCache ctlharvesterv1.SettingCache,
	vrbCache ctlharvesterv1.VolumeRemoteBackupCache,
	bo BackupOperator,
) RestoreOperator {
	return &restoreOperator{
		client:       client,
		settingCache: settingCache,
		vrbCache:     vrbCache,
		bo:           bo,
	}
}

type restoreMutator func(*harvesterv1.VolumeRemoteRestore) error

func (ro *restoreOperator) GetNamespace(vrr *harvesterv1.VolumeRemoteRestore) string {
	return vrr.Namespace
}

func (ro *restoreOperator) GetName(vrr *harvesterv1.VolumeRemoteRestore) string {
	return vrr.Name
}

func (ro *restoreOperator) GetKind(vrr *harvesterv1.VolumeRemoteRestore) string {
	return vrr.Kind
}

func (ro *restoreOperator) GetUID(vrr *harvesterv1.VolumeRemoteRestore) types.UID {
	return vrr.UID
}

func (ro *restoreOperator) GetSuccess(vrr *harvesterv1.VolumeRemoteRestore) bool {
	return vrr.Status.Success
}

func (ro *restoreOperator) Update(oldVrr, newVrr *harvesterv1.VolumeRemoteRestore) (*harvesterv1.VolumeRemoteRestore, error) {
	if reflect.DeepEqual(oldVrr, newVrr) {
		return newVrr, nil
	}
	return ro.client.Update(newVrr)
}

func (ro *restoreOperator) apply(vrr *harvesterv1.VolumeRemoteRestore, mutators ...restoreMutator) (*harvesterv1.VolumeRemoteRestore, error) {
	newVrr := vrr.DeepCopy()
	for _, mutator := range mutators {
		if err := mutator(newVrr); err != nil {
			return vrr, err
		}
	}
	return ro.Update(vrr, newVrr)
}

func (ro *restoreOperator) GetType(vrr *harvesterv1.VolumeRemoteRestore) harvesterv1.VolumeRemoteRestoreType {
	return vrr.Spec.Type
}

func (ro *restoreOperator) GetFrom(vrr *harvesterv1.VolumeRemoteRestore) string {
	return vrr.Spec.From
}

func (ro *restoreOperator) GetSnapshotClassInfo(vrr *harvesterv1.VolumeRemoteRestore) (*settings.CSIDriverInfo, error) {
	vrbNamespace, vrbName := ref.Parse(ro.GetFrom(vrr))
	vrb, err := ro.vrbCache.Get(vrbNamespace, vrbName)
	if err != nil {
		return nil, err
	}

	provider := ro.bo.GetCSIProvider(vrb)

	// Load CSI driver config
	csiDriverConfig, err := util.LoadCSIDriverConfig(ro.settingCache)
	if err != nil {
		return nil, err
	}

	// Get driver info
	driverInfo, ok := csiDriverConfig[provider]
	if !ok {
		return nil, fmt.Errorf("CSI driver %q not found in CSIDriverInfo settings for resource %s/%s",
			provider, ro.GetNamespace(vrr), ro.GetFrom(vrr))
	}
	return &driverInfo, nil
}

func (ro *restoreOperator) SetSuccess(vrr *harvesterv1.VolumeRemoteRestore, success bool) (*harvesterv1.VolumeRemoteRestore, error) {
	return ro.apply(vrr, ro.withSuccess(success))
}

func (ro *restoreOperator) SetReady(vrr *harvesterv1.VolumeRemoteRestore) (*harvesterv1.VolumeRemoteRestore, error) {
	return ro.setCondition(vrr, newReadyCondition(corev1.ConditionTrue, "", "Operation complete"))
}

func (ro *restoreOperator) SetProcessing(vrr *harvesterv1.VolumeRemoteRestore) (*harvesterv1.VolumeRemoteRestore, error) {
	return ro.setCondition(vrr, newReadyCondition(corev1.ConditionFalse, "", "Operation in progress"))
}

func (ro *restoreOperator) SetError(vrr *harvesterv1.VolumeRemoteRestore, err error) (*harvesterv1.VolumeRemoteRestore, error) {
	return ro.setCondition(vrr, newReadyCondition(corev1.ConditionFalse, "", err.Error()))
}

func (ro *restoreOperator) setCondition(vrr *harvesterv1.VolumeRemoteRestore, c harvesterv1.Condition) (*harvesterv1.VolumeRemoteRestore, error) {
	return ro.apply(vrr, ro.withCondition(c))
}

func (ro *restoreOperator) withSuccess(success bool) restoreMutator {
	return func(vrr *harvesterv1.VolumeRemoteRestore) error {
		vrr.Status.Success = success
		return nil
	}
}

func (ro *restoreOperator) withCondition(c harvesterv1.Condition) restoreMutator {
	return func(vrr *harvesterv1.VolumeRemoteRestore) error {
		vrr.Status.Conditions = applyConditionToStatus(vrr.Status.Conditions, c)
		return nil
	}
}
