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

// StatusFields defines a constraint for types that have common status fields
type StatusFields interface {
	*harvesterv1.VolumeRemoteBackupStatus | *harvesterv1.VolumeRemoteRestoreStatus
}

// ResourceFields defines a constraint for resource types
type ResourceFields interface {
	*harvesterv1.VolumeRemoteBackup | *harvesterv1.VolumeRemoteRestore
}

// ResourceClient defines common client operations
type ResourceClient[T ResourceFields] interface {
	Update(obj T) (T, error)
}

type ResourceMutator[T ResourceFields] func(T) error

// ResourceOperatorConfig wraps all configuration needed to create a ResourceOperator
type ResourceOperatorConfig[T ResourceFields, S StatusFields] struct {
	// Clients and caches
	Client       ResourceClient[T]
	PVCCache     ctlcorev1.PersistentVolumeClaimCache
	SCCache      ctlstoragev1.StorageClassCache
	SettingCache ctlharvesterv1.SettingCache

	// Accessor functions
	GetNamespace func(T) string
	GetName      func(T) string
	GetKind      func(T) string
	GetUID       func(T) types.UID
	DeepCopy     func(T) T
	GetStatus    func(T) S
	SetStatus    func(T, S) T
}

// ResourceOperator provides common operations for PVCBackup/PVCRestore
type ResourceOperator[T ResourceFields, S StatusFields] struct {
	client       ResourceClient[T]
	pvcCache     ctlcorev1.PersistentVolumeClaimCache
	scCache      ctlstoragev1.StorageClassCache
	settingCache ctlharvesterv1.SettingCache

	// Accessor functions
	getNamespace func(T) string
	getName      func(T) string
	getKind      func(T) string
	getUID       func(T) types.UID
	deepCopy     func(T) T
	getStatus    func(T) S
	setStatus    func(T, S) T
}

// NewResourceOperator creates a generic resource operator
func NewResourceOperator[T ResourceFields, S StatusFields](config ResourceOperatorConfig[T, S]) *ResourceOperator[T, S] {
	return &ResourceOperator[T, S]{
		client:       config.Client,
		pvcCache:     config.PVCCache,
		scCache:      config.SCCache,
		settingCache: config.SettingCache,
		getNamespace: config.GetNamespace,
		getName:      config.GetName,
		getKind:      config.GetKind,
		getUID:       config.GetUID,
		deepCopy:     config.DeepCopy,
		getStatus:    config.GetStatus,
		setStatus:    config.SetStatus,
	}
}

// Common accessor methods
func (ro *ResourceOperator[T, S]) GetNamespace(obj T) string {
	return ro.getNamespace(obj)
}

func (ro *ResourceOperator[T, S]) GetName(obj T) string {
	return ro.getName(obj)
}

func (ro *ResourceOperator[T, S]) GetKind(obj T) string {
	return ro.getKind(obj)
}

func (ro *ResourceOperator[T, S]) GetUID(obj T) types.UID {
	return ro.getUID(obj)
}

func (ro *ResourceOperator[T, S]) GetSuccess(obj T) bool {
	status := ro.getStatus(obj)
	// Use type switch to access the Success field directly
	switch s := any(status).(type) {
	case *harvesterv1.VolumeRemoteBackupStatus:
		return s.Success
	case *harvesterv1.VolumeRemoteRestoreStatus:
		return s.Success
	}
	return false
}

// Update updates the resource
func (ro *ResourceOperator[T, S]) Update(oldObj, newObj T) (T, error) {
	if reflect.DeepEqual(oldObj, newObj) {
		return newObj, nil
	}
	return ro.client.Update(newObj)
}

// Apply mutates a deep copy in memory, then commits the combined changes with one update.
func (ro *ResourceOperator[T, S]) Apply(obj T, mutators ...ResourceMutator[T]) (T, error) {
	newObj := ro.deepCopy(obj)
	for _, mutator := range mutators {
		if err := mutator(newObj); err != nil {
			return obj, err
		}
	}
	return ro.Update(obj, newObj)
}

func (ro *ResourceOperator[T, S]) withSuccess(success bool) ResourceMutator[T] {
	return func(obj T) error {
		status := ro.getStatus(obj)
		switch s := any(status).(type) {
		case *harvesterv1.VolumeRemoteBackupStatus:
			s.Success = success
		case *harvesterv1.VolumeRemoteRestoreStatus:
			s.Success = success
		}

		ro.setStatus(obj, status)
		return nil
	}
}

// SetSuccess sets the success status
func (ro *ResourceOperator[T, S]) SetSuccess(obj T, success bool) (T, error) {
	return ro.Apply(obj, ro.withSuccess(success))
}

// setCondition sets a condition on the resource
func (ro *ResourceOperator[T, S]) setCondition(obj T, c harvesterv1.Condition) (T, error) {
	return ro.Apply(obj, ro.withCondition(c))
}

func (ro *ResourceOperator[T, S]) withCondition(c harvesterv1.Condition) ResourceMutator[T] {
	return func(obj T) error {
		status := ro.getStatus(obj)

		// Both PVCBackupStatus and PVCRestoreStatus have the same Conditions field
		// Type switch is necessary due to Go generics limitations
		switch s := any(status).(type) {
		case *harvesterv1.VolumeRemoteBackupStatus:
			s.Conditions = applyConditionToStatus(s.Conditions, c)
		case *harvesterv1.VolumeRemoteRestoreStatus:
			s.Conditions = applyConditionToStatus(s.Conditions, c)
		}

		ro.setStatus(obj, status)
		return nil
	}
}

// SetReady sets the ready condition
func (ro *ResourceOperator[T, S]) SetReady(obj T) (T, error) {
	return ro.setCondition(obj, newReadyCondition(corev1.ConditionTrue, "", "Operation complete"))
}

// SetProcessing sets the processing condition
func (ro *ResourceOperator[T, S]) SetProcessing(obj T) (T, error) {
	return ro.setCondition(obj, newReadyCondition(corev1.ConditionFalse, "", "Operation in progress"))
}

// SetError sets the error condition
func (ro *ResourceOperator[T, S]) SetError(obj T, err error) (T, error) {
	return ro.setCondition(obj, newReadyCondition(corev1.ConditionFalse, "", err.Error()))
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
	*ResourceOperator[*harvesterv1.VolumeRemoteBackup, *harvesterv1.VolumeRemoteBackupStatus]
	client ctlharvesterv1.VolumeRemoteBackupClient
}

func NewBackupOperator(
	client ctlharvesterv1.VolumeRemoteBackupClient,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	scCache ctlstoragev1.StorageClassCache,
	settingCache ctlharvesterv1.SettingCache,
) BackupOperator {
	return &backupOperator{
		ResourceOperator: NewResourceOperator(
			ResourceOperatorConfig[*harvesterv1.VolumeRemoteBackup, *harvesterv1.VolumeRemoteBackupStatus]{
				Client:       client,
				PVCCache:     pvcCache,
				SCCache:      scCache,
				SettingCache: settingCache,
				GetNamespace: func(vrb *harvesterv1.VolumeRemoteBackup) string { return vrb.Namespace },
				GetName:      func(vrb *harvesterv1.VolumeRemoteBackup) string { return vrb.Name },
				GetKind:      func(vrb *harvesterv1.VolumeRemoteBackup) string { return vrb.Kind },
				GetUID:       func(vrb *harvesterv1.VolumeRemoteBackup) types.UID { return vrb.UID },
				DeepCopy:     func(vrb *harvesterv1.VolumeRemoteBackup) *harvesterv1.VolumeRemoteBackup { return vrb.DeepCopy() },
				GetStatus: func(vrb *harvesterv1.VolumeRemoteBackup) *harvesterv1.VolumeRemoteBackupStatus {
					return &vrb.Status
				},
				SetStatus: func(vrb *harvesterv1.VolumeRemoteBackup, s *harvesterv1.VolumeRemoteBackupStatus) *harvesterv1.VolumeRemoteBackup {
					vrb.Status = *s
					return vrb
				},
			},
		),
		client: client,
	}
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
	srcPVC, err := bo.pvcCache.Get(bo.getNamespace(vrb), bo.GetSource(vrb))
	if err != nil {
		return nil, err
	}

	// Get CSI driver from PVC
	provider := util.GetProvisionedPVCProvisioner(srcPVC, bo.scCache)
	if provider == "" {
		return nil, fmt.Errorf("CSI driver not found for PVC %s/%s", bo.getNamespace(vrb), bo.GetSource(vrb))
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
			provider, bo.getNamespace(vrb), bo.GetSource(vrb))
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
	return bo.Apply(vrb, bo.withHandle(handle))
}

func (bo *backupOperator) Finalize(vrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error) {
	return bo.Apply(
		vrb,
		bo.withCondition(newReadyCondition(corev1.ConditionTrue, "", "Operation complete")),
		bo.withSuccess(true),
		bo.withCSIProvider(),
		bo.withSourceSpec(),
	)
}

func (bo *backupOperator) UpdateCSIProvider(vrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error) {
	return bo.Apply(vrb, bo.withCSIProvider())
}

func (bo *backupOperator) UpdateSourceSpec(vrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error) {
	return bo.Apply(vrb, bo.withSourceSpec())
}

func (bo *backupOperator) withHandle(handle string) ResourceMutator[*harvesterv1.VolumeRemoteBackup] {
	return func(vrb *harvesterv1.VolumeRemoteBackup) error {
		vrb.Status.Handle = handle
		return nil
	}
}

func (bo *backupOperator) withCSIProvider() ResourceMutator[*harvesterv1.VolumeRemoteBackup] {
	return func(vrb *harvesterv1.VolumeRemoteBackup) error {
		srcPVC, err := bo.pvcCache.Get(bo.getNamespace(vrb), bo.GetSource(vrb))
		if err != nil {
			return err
		}

		provider := util.GetProvisionedPVCProvisioner(srcPVC, bo.scCache)
		if provider == "" {
			return fmt.Errorf("CSI driver not found for PVC %s/%s", bo.getNamespace(vrb), bo.GetSource(vrb))
		}

		vrb.Status.CSIProvider = provider
		return nil
	}
}

func (bo *backupOperator) withSourceSpec() ResourceMutator[*harvesterv1.VolumeRemoteBackup] {
	return func(vrb *harvesterv1.VolumeRemoteBackup) error {
		srcPVC, err := bo.pvcCache.Get(bo.getNamespace(vrb), bo.GetSource(vrb))
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
	*ResourceOperator[*harvesterv1.VolumeRemoteRestore, *harvesterv1.VolumeRemoteRestoreStatus]
	client   ctlharvesterv1.VolumeRemoteRestoreClient
	vrbCache ctlharvesterv1.VolumeRemoteBackupCache
	bo       BackupOperator
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
		ResourceOperator: NewResourceOperator(
			ResourceOperatorConfig[*harvesterv1.VolumeRemoteRestore, *harvesterv1.VolumeRemoteRestoreStatus]{
				Client:       client,
				PVCCache:     pvcCache,
				SCCache:      scCache,
				SettingCache: settingCache,
				GetNamespace: func(vrr *harvesterv1.VolumeRemoteRestore) string { return vrr.Namespace },
				GetName:      func(vrr *harvesterv1.VolumeRemoteRestore) string { return vrr.Name },
				GetKind:      func(vrr *harvesterv1.VolumeRemoteRestore) string { return vrr.Kind },
				GetUID:       func(vrr *harvesterv1.VolumeRemoteRestore) types.UID { return vrr.UID },
				DeepCopy: func(vrr *harvesterv1.VolumeRemoteRestore) *harvesterv1.VolumeRemoteRestore {
					return vrr.DeepCopy()
				},
				GetStatus: func(vrr *harvesterv1.VolumeRemoteRestore) *harvesterv1.VolumeRemoteRestoreStatus {
					return &vrr.Status
				},
				SetStatus: func(
					vrr *harvesterv1.VolumeRemoteRestore,
					s *harvesterv1.VolumeRemoteRestoreStatus,
				) *harvesterv1.VolumeRemoteRestore {
					vrr.Status = *s
					return vrr
				},
			},
		),
		client:   client,
		vrbCache: vrbCache,
		bo:       bo,
	}
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
