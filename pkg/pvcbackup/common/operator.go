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
	PVCBackupRestoreCondition condition.Cond = "Ready"
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
		Type:               PVCBackupRestoreCondition,
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
	*harvesterv1.PVCBackupStatus | *harvesterv1.PVCRestoreStatus
}

// ResourceFields defines a constraint for resource types
type ResourceFields interface {
	*harvesterv1.PVCBackup | *harvesterv1.PVCRestore
}

// ResourceClient defines common client operations
type ResourceClient[T ResourceFields] interface {
	Update(obj T) (T, error)
}

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
	case *harvesterv1.PVCBackupStatus:
		return s.Success
	case *harvesterv1.PVCRestoreStatus:
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

// SetSuccess sets the success status
func (ro *ResourceOperator[T, S]) SetSuccess(obj T, success bool) (T, error) {
	newObj := ro.deepCopy(obj)
	status := ro.getStatus(newObj)
	// Both PVCBackupStatus and PVCRestoreStatus have the same Success field
	switch s := any(status).(type) {
	case *harvesterv1.PVCBackupStatus:
		s.Success = success
	case *harvesterv1.PVCRestoreStatus:
		s.Success = success
	}
	newObj = ro.setStatus(newObj, status)
	return ro.Update(obj, newObj)
}

// setCondition sets a condition on the resource
func (ro *ResourceOperator[T, S]) setCondition(obj T, c harvesterv1.Condition) (T, error) {
	newObj := ro.deepCopy(obj)
	status := ro.getStatus(newObj)

	// Both PVCBackupStatus and PVCRestoreStatus have the same Conditions field
	// Type switch is necessary due to Go generics limitations
	switch s := any(status).(type) {
	case *harvesterv1.PVCBackupStatus:
		s.Conditions = applyConditionToStatus(s.Conditions, c)
	case *harvesterv1.PVCRestoreStatus:
		s.Conditions = applyConditionToStatus(s.Conditions, c)
	}

	newObj = ro.setStatus(newObj, status)
	return ro.Update(obj, newObj)
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

// PVCBackupOperator defines the interface for PVCBackup operations
type PVCBackupOperator interface {
	// Getters
	GetNamespace(pb *harvesterv1.PVCBackup) string
	GetName(pb *harvesterv1.PVCBackup) string
	GetKind(pb *harvesterv1.PVCBackup) string
	GetUID(pb *harvesterv1.PVCBackup) types.UID
	GetType(pb *harvesterv1.PVCBackup) harvesterv1.PVCBackupType
	GetSource(pb *harvesterv1.PVCBackup) string
	GetHandle(pb *harvesterv1.PVCBackup) string
	GetSuccess(pb *harvesterv1.PVCBackup) bool
	GetVSClassInfo(pb *harvesterv1.PVCBackup) (*settings.CSIDriverInfo, error)
	GetCSIProvider(pb *harvesterv1.PVCBackup) string
	GetSourceSpec(pb *harvesterv1.PVCBackup) corev1.PersistentVolumeClaimSpec

	// Update
	Update(oldPb, newPb *harvesterv1.PVCBackup) (*harvesterv1.PVCBackup, error)
	UpdateCSIProvider(pb *harvesterv1.PVCBackup) (*harvesterv1.PVCBackup, error)
	UpdateSourceSpec(pb *harvesterv1.PVCBackup) (*harvesterv1.PVCBackup, error)

	// Setters
	SetSuccess(pb *harvesterv1.PVCBackup, success bool) (*harvesterv1.PVCBackup, error)
	SetHandle(pb *harvesterv1.PVCBackup, handle string) (*harvesterv1.PVCBackup, error)
	SetReady(pb *harvesterv1.PVCBackup) (*harvesterv1.PVCBackup, error)
	SetProcessing(pb *harvesterv1.PVCBackup) (*harvesterv1.PVCBackup, error)
	SetError(pb *harvesterv1.PVCBackup, err error) (*harvesterv1.PVCBackup, error)
}

type pvcBackupOperator struct {
	*ResourceOperator[*harvesterv1.PVCBackup, *harvesterv1.PVCBackupStatus]
	client ctlharvesterv1.PVCBackupClient
}

// GetPVCBackupOperator creates a new PVCBackupOperator instance
func GetPVCBackupOperator(
	client ctlharvesterv1.PVCBackupClient,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	scCache ctlstoragev1.StorageClassCache,
	settingCache ctlharvesterv1.SettingCache,
) PVCBackupOperator {
	return &pvcBackupOperator{
		ResourceOperator: NewResourceOperator(
			ResourceOperatorConfig[*harvesterv1.PVCBackup, *harvesterv1.PVCBackupStatus]{
				Client:       client,
				PVCCache:     pvcCache,
				SCCache:      scCache,
				SettingCache: settingCache,
				GetNamespace: func(pb *harvesterv1.PVCBackup) string { return pb.Namespace },
				GetName:      func(pb *harvesterv1.PVCBackup) string { return pb.Name },
				GetKind:      func(pb *harvesterv1.PVCBackup) string { return pb.Kind },
				GetUID:       func(pb *harvesterv1.PVCBackup) types.UID { return pb.UID },
				DeepCopy:     func(pb *harvesterv1.PVCBackup) *harvesterv1.PVCBackup { return pb.DeepCopy() },
				GetStatus:    func(pb *harvesterv1.PVCBackup) *harvesterv1.PVCBackupStatus { return &pb.Status },
				SetStatus: func(pb *harvesterv1.PVCBackup, s *harvesterv1.PVCBackupStatus) *harvesterv1.PVCBackup {
					pb.Status = *s
					return pb
				},
			},
		),
		client: client,
	}
}

// PVCBackup-specific methods
func (pbo *pvcBackupOperator) GetType(pb *harvesterv1.PVCBackup) harvesterv1.PVCBackupType {
	return pb.Spec.Type
}

func (pbo *pvcBackupOperator) GetSource(pb *harvesterv1.PVCBackup) string {
	return pb.Spec.Source
}

func (pbo *pvcBackupOperator) GetHandle(pb *harvesterv1.PVCBackup) string {
	return pb.Status.Handle
}

func (pbo *pvcBackupOperator) GetVSClassInfo(pb *harvesterv1.PVCBackup) (*settings.CSIDriverInfo, error) {
	srcPVC, err := pbo.pvcCache.Get(pbo.getNamespace(pb), pbo.GetSource(pb))
	if err != nil {
		return nil, err
	}

	// Get CSI driver from PVC
	provider := util.GetProvisionedPVCProvisioner(srcPVC, pbo.scCache)
	if provider == "" {
		return nil, fmt.Errorf("CSI driver not found for PVC %s/%s", pbo.getNamespace(pb), pbo.GetSource(pb))
	}

	// Load CSI driver config
	csiDriverConfig, err := util.LoadCSIDriverConfig(pbo.settingCache)
	if err != nil {
		return nil, err
	}

	// Get driver info
	driverInfo, ok := csiDriverConfig[provider]
	if !ok {
		return nil, fmt.Errorf("CSI driver %q not found in CSIDriverInfo settings for PVC %s/%s",
			provider, pbo.getNamespace(pb), pbo.GetSource(pb))
	}
	return &driverInfo, nil
}

func (pbo *pvcBackupOperator) GetCSIProvider(pb *harvesterv1.PVCBackup) string {
	return pb.Status.CSIProvider
}

func (pbo *pvcBackupOperator) GetSourceSpec(pb *harvesterv1.PVCBackup) corev1.PersistentVolumeClaimSpec {
	return pb.Status.SourceSpec
}

func (pbo *pvcBackupOperator) SetHandle(pb *harvesterv1.PVCBackup, handle string) (*harvesterv1.PVCBackup, error) {
	newPb := pb.DeepCopy()
	newPb.Status.Handle = handle
	return pbo.Update(pb, newPb)
}

func (pbo *pvcBackupOperator) UpdateCSIProvider(pb *harvesterv1.PVCBackup) (*harvesterv1.PVCBackup, error) {
	srcPVC, err := pbo.pvcCache.Get(pbo.getNamespace(pb), pbo.GetSource(pb))
	if err != nil {
		return nil, err
	}

	// Get CSI driver from PVC
	provider := util.GetProvisionedPVCProvisioner(srcPVC, pbo.scCache)
	if provider == "" {
		return nil, fmt.Errorf("CSI driver not found for PVC %s/%s", pbo.getNamespace(pb), pbo.GetSource(pb))
	}

	newPb := pb.DeepCopy()
	newPb.Status.CSIProvider = provider
	return pbo.Update(pb, newPb)
}

func (pbo *pvcBackupOperator) UpdateSourceSpec(pb *harvesterv1.PVCBackup) (*harvesterv1.PVCBackup, error) {
	srcPVC, err := pbo.pvcCache.Get(pbo.getNamespace(pb), pbo.GetSource(pb))
	if err != nil {
		return nil, err
	}

	newPb := pb.DeepCopy()
	newPb.Status.SourceSpec = srcPVC.Spec
	return pbo.Update(pb, newPb)
}

// PVCRestoreOperator defines the interface for PVCRestore operations
type PVCRestoreOperator interface {
	// Getters
	GetNamespace(pr *harvesterv1.PVCRestore) string
	GetName(pr *harvesterv1.PVCRestore) string
	GetKind(pr *harvesterv1.PVCRestore) string
	GetUID(pr *harvesterv1.PVCRestore) types.UID
	GetType(pr *harvesterv1.PVCRestore) harvesterv1.PVCRestoreType
	GetFrom(pr *harvesterv1.PVCRestore) string
	GetSuccess(pr *harvesterv1.PVCRestore) bool
	GetVSClassInfo(pr *harvesterv1.PVCRestore) (*settings.CSIDriverInfo, error)

	// Update
	Update(oldPr, newPr *harvesterv1.PVCRestore) (*harvesterv1.PVCRestore, error)

	// Setters
	SetSuccess(pr *harvesterv1.PVCRestore, success bool) (*harvesterv1.PVCRestore, error)
	SetReady(pr *harvesterv1.PVCRestore) (*harvesterv1.PVCRestore, error)
	SetProcessing(pr *harvesterv1.PVCRestore) (*harvesterv1.PVCRestore, error)
	SetError(pr *harvesterv1.PVCRestore, err error) (*harvesterv1.PVCRestore, error)
}

type pvcRestoreOperator struct {
	*ResourceOperator[*harvesterv1.PVCRestore, *harvesterv1.PVCRestoreStatus]
	client  ctlharvesterv1.PVCRestoreClient
	pbCache ctlharvesterv1.PVCBackupCache
	pbo     PVCBackupOperator
}

func GetPVCRestoreOperator(
	client ctlharvesterv1.PVCRestoreClient,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	scCache ctlstoragev1.StorageClassCache,
	settingCache ctlharvesterv1.SettingCache,
	pbCache ctlharvesterv1.PVCBackupCache,
	pbo PVCBackupOperator,
) PVCRestoreOperator {
	return &pvcRestoreOperator{
		ResourceOperator: NewResourceOperator(
			ResourceOperatorConfig[*harvesterv1.PVCRestore, *harvesterv1.PVCRestoreStatus]{
				Client:       client,
				PVCCache:     pvcCache,
				SCCache:      scCache,
				SettingCache: settingCache,
				GetNamespace: func(pr *harvesterv1.PVCRestore) string { return pr.Namespace },
				GetName:      func(pr *harvesterv1.PVCRestore) string { return pr.Name },
				GetKind:      func(pr *harvesterv1.PVCRestore) string { return pr.Kind },
				GetUID:       func(pr *harvesterv1.PVCRestore) types.UID { return pr.UID },
				DeepCopy:     func(pr *harvesterv1.PVCRestore) *harvesterv1.PVCRestore { return pr.DeepCopy() },
				GetStatus:    func(pr *harvesterv1.PVCRestore) *harvesterv1.PVCRestoreStatus { return &pr.Status },
				SetStatus: func(pr *harvesterv1.PVCRestore, s *harvesterv1.PVCRestoreStatus) *harvesterv1.PVCRestore {
					pr.Status = *s
					return pr
				},
			},
		),
		client:  client,
		pbCache: pbCache,
		pbo:     pbo,
	}
}

// PVCRestore-specific methods
func (pro *pvcRestoreOperator) GetType(pr *harvesterv1.PVCRestore) harvesterv1.PVCRestoreType {
	return pr.Spec.Type
}

func (pro *pvcRestoreOperator) GetFrom(pr *harvesterv1.PVCRestore) string {
	return pr.Spec.From
}

func (pro *pvcRestoreOperator) GetVSClassInfo(pr *harvesterv1.PVCRestore) (*settings.CSIDriverInfo, error) {
	pbNamespace, pbName := ref.Parse(pro.GetFrom(pr))
	pb, err := pro.pbCache.Get(pbNamespace, pbName)
	if err != nil {
		return nil, err
	}

	provider := pro.pbo.GetCSIProvider(pb)

	// Load CSI driver config
	csiDriverConfig, err := util.LoadCSIDriverConfig(pro.settingCache)
	if err != nil {
		return nil, err
	}

	// Get driver info
	driverInfo, ok := csiDriverConfig[provider]
	if !ok {
		return nil, fmt.Errorf("CSI driver %q not found in CSIDriverInfo settings for resource %s/%s",
			provider, pro.GetNamespace(pr), pro.GetFrom(pr))
	}
	return &driverInfo, nil
}
