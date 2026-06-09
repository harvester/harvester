package backup

// Harvester VM backup & restore controllers helps to manage the VM backup & restore by leveraging
// the VolumeSnapshot functionality of Kubernetes CSI drivers with built-in storage driver longhorn.
// Currently, the following features are supported:
// 1. support VM live & offline backup to the supported backupTarget(i.e, nfs_v4 or s3 storage server).
// 2. restore a backup to a new VM or replacing it with the existing VM is supported.
import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	lhdatastore "github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/types"
	lhutil "github.com/longhorn/longhorn-manager/util"
	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
	"github.com/harvester/harvester/pkg/restore/engine"

	"github.com/harvester/harvester/pkg/restore/engine/longhorn"

	restoresnapshot "github.com/harvester/harvester/pkg/restore/engine/snapshot"
	"github.com/harvester/harvester/pkg/util"
)

const (
	restoreControllerName = "harvester-vm-restore-controller"

	lastRestoreAnnotation = "restore.harvesterhci.io/last-restore-uid"

	// restoreProgressPoll is the cadence at which we re-reconcile a VMRestore
	// while its volumes are still restoring, so progress is refreshed from
	// the engine on a known schedule.
	restoreProgressPoll = 5 * time.Second

	pvNamePrefix = "pvc"

	//not truncate or remove dashes pv name
	volumeNameUUIDNoTruncate = -1
)

type RestoreHandler struct {
	context context.Context

	// Controllers and clients still directly used
	vmrController ctlharvesterv1.VirtualMachineRestoreController
	vmrCache      ctlharvesterv1.VirtualMachineRestoreCache
	vmClient      ctlkubevirtv1.VirtualMachineClient
	vscClient     ctlsnapshotv1.VolumeSnapshotContentClient
	vscCache      ctlsnapshotv1.VolumeSnapshotContentCache
	volumeCache   ctllhv1.VolumeCache
	volumes       ctllhv1.VolumeClient
	scCache       ctlstoragev1.StorageClassCache
	pvcCache      ctlcorev1.PersistentVolumeClaimCache

	// Operators and engines
	vmbo    backupcommon.VMBackupOperator
	vmro    restorecommon.VMRestoreOperator
	engines map[harvesterv1.BackupType]engine.RestoreEngine
}

func RegisterRestore(ctx context.Context, management *config.Management, _ config.Options) error {
	// Get all required controllers and caches
	controllers := getRestoreControllers(management)

	// Initialize REST client for Kubevirt
	restClient, err := newKubevirtClient(management.RestConfig)
	if err != nil {
		return err
	}

	// Initialize operators
	vmbo, vmro := newRestoreOperators(controllers, restClient)

	// Initialize restore engines
	engines := newRestoreEngines(controllers, vmbo, vmro)

	// Let each engine wire up its own informer event handlers (e.g. Job
	// watchers) so engine-owned resource changes feed back into VMRestore
	// reconciles immediately instead of waiting on poll/requeue.
	for _, e := range engines {
		e.RegisterWatchers(ctx, controllers.vmrs.Enqueue)
	}

	// Create and configure handler
	handler := newRestoreHandler(ctx, controllers, vmbo, vmro, engines)

	// Register event handlers
	registerRestoreEventHandlers(ctx, controllers, handler)

	return nil
}

// restoreControllerSet holds all required controllers and caches
type restoreControllerSet struct {
	vmrs            ctlharvesterv1.VirtualMachineRestoreController
	vmbs            ctlharvesterv1.VirtualMachineBackupController
	vms             ctlkubevirtv1.VirtualMachineController
	vmis            ctlkubevirtv1.VirtualMachineInstanceController
	pvcs            ctlcorev1.PersistentVolumeClaimController
	pvs             ctlcorev1.PersistentVolumeController
	scs             ctlstoragev1.StorageClassController
	secrets         ctlcorev1.SecretController
	vss             ctlsnapshotv1.VolumeSnapshotController
	vscs            ctlsnapshotv1.VolumeSnapshotContentController
	lhbackups       ctllhv1.BackupController
	lhbackupVolumes ctllhv1.BackupVolumeController
	volumes         ctllhv1.VolumeController
	lhengines       ctllhv1.EngineController
	vsClasses       ctlsnapshotv1.VolumeSnapshotClassController
	jobs            ctlbatchv1.JobController
}

// getRestoreControllers extracts all required controllers from management
func getRestoreControllers(management *config.Management) *restoreControllerSet {
	return &restoreControllerSet{
		vmrs:            management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineRestore(),
		vmbs:            management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup(),
		vms:             management.VirtFactory.Kubevirt().V1().VirtualMachine(),
		vmis:            management.VirtFactory.Kubevirt().V1().VirtualMachineInstance(),
		pvcs:            management.CoreFactory.Core().V1().PersistentVolumeClaim(),
		pvs:             management.CoreFactory.Core().V1().PersistentVolume(),
		scs:             management.StorageFactory.Storage().V1().StorageClass(),
		secrets:         management.CoreFactory.Core().V1().Secret(),
		vss:             management.SnapshotFactory.Snapshot().V1().VolumeSnapshot(),
		vscs:            management.SnapshotFactory.Snapshot().V1().VolumeSnapshotContent(),
		lhbackups:       management.LonghornFactory.Longhorn().V1beta2().Backup(),
		lhbackupVolumes: management.LonghornFactory.Longhorn().V1beta2().BackupVolume(),
		volumes:         management.LonghornFactory.Longhorn().V1beta2().Volume(),
		lhengines:       management.LonghornFactory.Longhorn().V1beta2().Engine(),
		vsClasses:       management.SnapshotFactory.Snapshot().V1().VolumeSnapshotClass(),
		jobs:            management.BatchFactory.Batch().V1().Job(),
	}
}

// newRestoreOperators creates and configures VMBackup and VMRestore operators
func newRestoreOperators(
	controllers *restoreControllerSet,
	restClient *rest.RESTClient,
) (backupcommon.VMBackupOperator, restorecommon.VMRestoreOperator) {
	// client and virtSubresourceRestClient are not needed for restore ops.
	vmbo := backupcommon.NewVMBackupOperatorBuilder().
		WithCache(controllers.vmbs.Cache()).
		WithVSClassCache(controllers.vsClasses.Cache()).
		WithVMCache(controllers.vms.Cache()).
		WithVMICache(controllers.vmis.Cache()).
		WithPVCCache(controllers.pvcs.Cache()).
		WithPVCache(controllers.pvs.Cache()).
		WithSecretCache(controllers.secrets.Cache()).
		Build()

	vmro := restorecommon.NewVMRestoreOperatorBuilder().
		WithClient(controllers.vmrs).
		WithCache(controllers.vmrs.Cache()).
		WithVMCache(controllers.vms.Cache()).
		WithVMICache(controllers.vmis.Cache()).
		WithPVCClient(controllers.pvcs).
		WithPVCCache(controllers.pvcs.Cache()).
		WithSecretClient(controllers.secrets).
		WithSecretCache(controllers.secrets.Cache()).
		WithVMBackupCache(controllers.vmbs.Cache()).
		WithVMBackupOperator(vmbo).
		WithVirtSubresourceRestClient(restClient).
		Build()

	return vmbo, vmro
}

// newRestoreEngines creates restore engines for different backup types
func newRestoreEngines(
	controllers *restoreControllerSet,
	vmbo backupcommon.VMBackupOperator,
	vmro restorecommon.VMRestoreOperator,
) map[harvesterv1.BackupType]engine.RestoreEngine {
	return map[harvesterv1.BackupType]engine.RestoreEngine{
		harvesterv1.Backup: longhorn.GetRestoreEngine(
			vmbo,
			vmro,
			controllers.pvcs.Cache(),
			controllers.pvcs,
			controllers.scs.Cache(),
			controllers.vss.Cache(),
			controllers.vss,
			controllers.vscs.Cache(),
			controllers.vscs,
			controllers.lhbackups.Cache(),
			controllers.lhbackupVolumes.Cache(),
			controllers.lhengines.Cache(),
			controllers.lhengines,
			controllers.volumes.Cache(),
			controllers.vmbs,
			controllers.vmbs.Cache(),
			controllers.vmrs.Cache(),
		),
		harvesterv1.Snapshot: restoresnapshot.GetRestoreEngine(
			vmbo,
			vmro,
			controllers.pvcs.Cache(),
			controllers.pvcs,
			controllers.vss.Cache(),
		),
	}
}

// newRestoreHandler creates a new RestoreHandler with all dependencies
func newRestoreHandler(
	ctx context.Context,
	controllers *restoreControllerSet,
	vmbo backupcommon.VMBackupOperator,
	vmro restorecommon.VMRestoreOperator,
	engines map[harvesterv1.BackupType]engine.RestoreEngine,
) *RestoreHandler {
	return &RestoreHandler{
		context:       ctx,
		vmrController: controllers.vmrs,
		vmrCache:      controllers.vmrs.Cache(),
		vmClient:      controllers.vms,
		vscClient:     controllers.vscs,
		vscCache:      controllers.vscs.Cache(),
		volumeCache:   controllers.volumes.Cache(),
		volumes:       controllers.volumes,
		scCache:       controllers.scs.Cache(),
		pvcCache:      controllers.pvcs.Cache(),
		vmbo:          vmbo,
		vmro:          vmro,
		engines:       engines,
	}
}

// registerRestoreEventHandlers registers all event handlers for the restore controller
func registerRestoreEventHandlers(ctx context.Context, controllers *restoreControllerSet, handler *RestoreHandler) {
	controllers.vmrs.OnChange(ctx, restoreControllerName, handler.RestoreOnChanged)
	controllers.vmrs.OnRemove(ctx, restoreControllerName, handler.RestoreOnRemove)
	controllers.pvcs.OnChange(ctx, restoreControllerName, handler.PersistentVolumeClaimOnChange)
	controllers.vms.OnChange(ctx, restoreControllerName, handler.VMOnChange)
}

// getRestoreEngine selects the appropriate restore engine based on backup type
func (h *RestoreHandler) getRestoreEngine(backup *harvesterv1.VirtualMachineBackup) engine.RestoreEngine {
	if engine, exists := h.engines[backup.Spec.Type]; exists {
		return engine
	}
	return nil
}

// RestoreOnChanged handles vmRestore CRD object on change, it will help to create the new PVCs and either replace them
// with existing VM or used for the new VM.
func (h *RestoreHandler) RestoreOnChanged(_ string, vmr *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error) {
	if vmr == nil || h.vmro.IsDeleting(vmr) {
		return nil, nil
	}

	if !h.vmro.IsProgressing(vmr) {
		return nil, nil
	}

	if !h.vmro.HasStatus(vmr) {
		return nil, h.vmro.InitVMRestore(vmr)
	}

	vmb, err := h.vmro.ResolveVMBackup(vmr)
	if err != nil {
		return nil, h.vmro.UpdateError(vmr, err)
	}

	if h.vmro.IsMissingVolumes(vmr) {
		return nil, h.vmro.InitVolumesStatus(vmr, vmb)
	}

	// DeepCopy before any path that mutates status: engines call
	// SetVolRestoreProgress etc. on entries inside vmrCpy.Status, and the
	// informer cache must not be mutated. Persist-side operators take vmrCpy
	// as their basis so engine mutations flow through to client.Update; vmr
	// stays as the unmodified "old" side for diff comparisons.
	vmrCpy := vmr.DeepCopy()

	vm, isVolumesReady, err := h.reconcileResources(vmrCpy, vmb)

	// Refresh per-volume progress regardless of reconcile outcome so the user
	// sees the latest known progress even when an engine returns a hard error.
	if updErr := h.updateProgressMetrics(vmrCpy, vmb); updErr != nil {
		logrus.Warnf("failed to refresh restore progress for %s/%s: %v",
			vmr.Namespace, vmr.Name, updErr)
	}

	if err != nil {
		return nil, h.vmro.UpdateError(vmrCpy, err)
	}

	// set vmRestore owner reference to the target VM
	if !h.vmro.HasOwnerReference(vmrCpy) {
		return nil, h.vmro.UpdateOwnerRefAndTargetUID(vmrCpy, vm)
	}

	return nil, h.updateStatus(vmr, vmrCpy, vm, isVolumesReady)
}

// RestoreOnRemove delegates per-volume cleanup to the restore engine, mirroring
// the (engine, iterate volumes) pattern in reconcileVolumeRestores.
func (h *RestoreHandler) RestoreOnRemove(_ string, vmr *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error) {
	if vmr == nil || !h.vmro.HasStatus(vmr) {
		return nil, nil
	}

	re, err := h.resolveRestoreEngine(vmr)
	if err != nil {
		return nil, err
	}
	if re == nil {
		return nil, nil
	}

	for i := range h.vmro.GetVolRestores(vmr) {
		if err := re.Delete(vmr, i); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// resolveRestoreEngine picks the restore engine for a VMRestore. When the
// source VMBackup is gone (typically because the user deleted it ahead of the
// restore) we fall back to the Longhorn engine — the only engine with
// non-trivial cleanup (VolumeSnapshotContent is cluster-scoped, uses Retain
// policy, so it can't be garbage-collected via owner references).
func (h *RestoreHandler) resolveRestoreEngine(vmr *harvesterv1.VirtualMachineRestore) (engine.RestoreEngine, error) {
	vmb, err := h.vmro.ResolveVMBackup(vmr)
	if apierrors.IsNotFound(err) {
		return h.engines[harvesterv1.Backup], nil
	}
	if err != nil {
		return nil, err
	}
	return h.getRestoreEngine(vmb), nil
}

// pv naming convention from externel-provisioner
// porting from https://github.com/kubernetes-csi/external-provisioner/blob/90eae32d3a7352590500073b72b9a07f43adc881/pkg/controller/controller.go#L420-L436
func makeVolumeName(prefix, pvcUID string, volumeNameUUIDLength int) (string, error) {
	// create persistent name based on a volumeNamePrefix and volumeNameUUIDLength
	// of PVC's UID
	if len(prefix) == 0 {
		return "", fmt.Errorf("volume name prefix cannot be of length 0")
	}
	if len(pvcUID) == 0 {
		return "", fmt.Errorf("corrupted PVC object, it is missing UID")
	}
	if volumeNameUUIDLength == -1 {
		// Default behavior is to not truncate or remove dashes
		return fmt.Sprintf("%s-%s", prefix, pvcUID), nil
	}
	// Else we remove all dashes from UUID and truncate to volumeNameUUIDLength
	return fmt.Sprintf("%s-%s", prefix, strings.ReplaceAll(string(pvcUID), "-", "")[0:volumeNameUUIDLength]), nil

}

func getVolumeName(pvc *corev1.PersistentVolumeClaim) (string, error) {
	volumeName, err := makeVolumeName(pvNamePrefix, string(pvc.ObjectMeta.UID), volumeNameUUIDNoTruncate)
	if err != nil {
		return "", err
	}

	//sync with LH's naming convention on volume
	//https://github.com/longhorn/longhorn-manager/blob/88c792f7df38383634c2c8401f96d999385458c1/csi/controller_server.go#L83
	volumeName = lhutil.AutoCorrectName(volumeName, lhdatastore.NameMaximumLength)
	return volumeName, err
}

func (h *RestoreHandler) checkLHNotVolumeExist(pvc *corev1.PersistentVolumeClaim, restore string) (*corev1.PersistentVolumeClaim, error) {
	provisioner := util.GetProvisionedPVCProvisioner(pvc, h.scCache)
	if provisioner == types.LonghornDriverName {
		return nil, fmt.Errorf("LH pvc %s/%s missing volume", pvc.Namespace, pvc.Name)
	}

	// The storage provider is not LH, we should enqueue vmrestore
	logrus.WithFields(logrus.Fields{
		"namespace": pvc.Namespace,
		"name":      pvc.Name,
	}).Info("Non-LH PVC updating")

	h.vmrController.Enqueue(pvc.Namespace, restore)
	return nil, nil
}

// PersistentVolumeClaimOnChange watching the PVCs on change and enqueue the vmRestore if it has the restore annotation
func (h *RestoreHandler) PersistentVolumeClaimOnChange(_ string, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	if pvc == nil || pvc.DeletionTimestamp != nil {
		return nil, nil
	}

	restoreName, ok := pvc.Annotations[restorecommon.RestoreNameAnnotation]
	if !ok {
		return nil, nil
	}

	volumeName, err := getVolumeName(pvc)
	if err != nil {
		return nil, err
	}

	volume, err := h.volumeCache.Get(util.LonghornSystemNamespaceName, volumeName)
	if apierrors.IsNotFound(err) {
		return h.checkLHNotVolumeExist(pvc, restoreName)
	}

	if err != nil {
		return nil, err
	}

	volumeCopy := volume.DeepCopy()
	if volumeCopy.Annotations == nil {
		volumeCopy.Annotations = make(map[string]string)
	}

	volumeCopy.Annotations[restorecommon.PvcNameSpaceAnnotation] = pvc.Namespace
	volumeCopy.Annotations[restorecommon.PvcNameAnnotation] = pvc.Name
	volumeCopy.Annotations[restorecommon.RestoreNameAnnotation] = restoreName

	if !reflect.DeepEqual(volume, volumeCopy) {
		if _, err := h.volumes.Update(volumeCopy); err != nil {
			return nil, err
		}
	}

	logrus.Debugf("handling PVC updating %s/%s", pvc.Namespace, pvc.Name)
	h.vmrController.EnqueueAfter(pvc.Namespace, restoreName, 5*time.Second)
	return nil, nil
}

// VMOnChange watching the VM on change and enqueue the vmRestore if it has the restore annotation
func (h *RestoreHandler) VMOnChange(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil {
		return nil, nil
	}

	restoreName, ok := vm.Annotations[restorecommon.RestoreNameAnnotation]
	if !ok {
		return nil, nil
	}

	logrus.Debugf("handling VM updating %s/%s", vm.Namespace, vm.Name)
	h.vmrController.EnqueueAfter(vm.Namespace, restoreName, 5*time.Second)
	return nil, nil
}

func (h *RestoreHandler) reconcileResources(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
) (*kubevirtv1.VirtualMachine, bool, error) {
	// reconcile restoring volumes and create new PVC from CSI volumeSnapshot if not exist
	isVolumesReady, err := h.reconcileVolumeRestores(vmr, vmb)
	if err != nil {
		return nil, false, err
	}

	// reconcile VM
	vm, err := h.reconcileVM(vmr, vmb)
	if err != nil {
		return nil, false, err
	}

	//restore referenced secrets
	if err := h.vmro.RestoreSecrets(vmr, vmb, vm); err != nil {
		return nil, false, err
	}

	return vm, isVolumesReady, nil
}

func (h *RestoreHandler) reconcileVolumeRestores(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
) (bool, error) {
	// Select appropriate engine based on backup type
	re := h.getRestoreEngine(vmb)
	isVolumesReady := true

	// Reconcile each per-volume restore via the engine
	volRestores := h.vmro.GetVolRestores(vmr)
	for i := range volRestores {
		err := re.Reconcile(vmr, vmb, i)
		if err == nil {
			continue
		}

		if err == engine.ErrRetryLater {
			isVolumesReady = false
			continue
		}
		return false, err
	}

	return isVolumesReady, nil
}

func (h *RestoreHandler) reconcileVM(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
) (*kubevirtv1.VirtualMachine, error) {
	vm, err := h.vmro.ResolveTargetVM(vmr)
	if err != nil {
		return nil, err
	}

	if vm == nil {
		return h.createNewVM(vmr, vmb)
	}

	if h.isVMAlreadyRestored(vm, vmr) {
		return vm, nil
	}

	return h.updateExistingVM(vm, vmr, vmb)
}

// isVMAlreadyRestored checks if the VM has already been restored with the current restore ID
func (h *RestoreHandler) isVMAlreadyRestored(vm *kubevirtv1.VirtualMachine, vmr *harvesterv1.VirtualMachineRestore) bool {
	restoreID := h.vmro.GetRestoreID(vmr)
	lastRestoreID, ok := vm.Annotations[lastRestoreAnnotation]
	return ok && lastRestoreID == restoreID
}

// updateExistingVM updates an existing VM with the restore spec and volumes
func (h *RestoreHandler) updateExistingVM(
	vm *kubevirtv1.VirtualMachine,
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
) (*kubevirtv1.VirtualMachine, error) {
	sourceSpec := h.vmbo.GetSourceSpec(vmb)
	newVolumes, err := h.vmro.MapVolumesToRestoredPVCs(vmr, &sourceSpec.Spec)
	if err != nil {
		return nil, err
	}

	vmCpy := vm.DeepCopy()
	vmCpy.Spec = sourceSpec.Spec

	// if the source runStrategy is RerunOnFailure, Kubevirt will not start the new VMI
	// set the VM runStrategy as Halted, VMI will be kicked off in startVM()
	haltedRunStrategy := kubevirtv1.RunStrategyHalted
	vmCpy.Spec.RunStrategy = &haltedRunStrategy
	vmCpy.Spec.Template.Spec.Volumes = newVolumes

	if err := h.setRestoreAnnotations(vmCpy, vmr); err != nil {
		return nil, err
	}

	return h.vmClient.Update(vmCpy)
}

// setRestoreAnnotations sets the required restore annotations on the VM. The
// volumeClaimTemplates annotation is rebuilt from the actual restored PVCs so
// downstream features (storage migration, PVC management UI) see the current
// disks rather than the pre-restore template.
func (h *RestoreHandler) setRestoreAnnotations(vm *kubevirtv1.VirtualMachine, vmr *harvesterv1.VirtualMachineRestore) error {
	if vm.Annotations == nil {
		vm.Annotations = make(map[string]string)
	}
	vm.Annotations[lastRestoreAnnotation] = h.vmro.GetRestoreID(vmr)
	vm.Annotations[restorecommon.RestoreNameAnnotation] = h.vmro.GetName(vmr)

	volumeClaimTemplatesStr, err := h.buildVolumeClaimTemplates(vmr)
	if err != nil {
		return fmt.Errorf("failed to build volumeClaimTemplates annotation: %w", err)
	}
	vm.Annotations[util.AnnotationVolumeClaimTemplates] = volumeClaimTemplatesStr
	return nil
}

// buildVolumeClaimTemplates builds the volumeClaimTemplates annotation from
// the actual restored PVCs. This enables storage migration and other PVC
// management features for restored VMs.
func (h *RestoreHandler) buildVolumeClaimTemplates(vmr *harvesterv1.VirtualMachineRestore) (string, error) {
	vrs := h.vmro.GetVolRestores(vmr)
	entries := make([]util.VolumeClaimTemplateEntry, 0, len(vrs))
	namespace := h.vmro.GetNamespace(vmr)
	for i := range vrs {
		vr := &vrs[i]
		pvcName := h.vmro.GetVolRestorePVCName(vr)
		// Re-fetch the PVC so we pick up the latest annotations (e.g. imageID)
		// rather than the snapshot in vmr.Status.
		pvc, err := h.pvcCache.Get(namespace, pvcName)
		if err != nil {
			return "", fmt.Errorf("failed to get restored PVC %s/%s: %w", namespace, pvcName, err)
		}
		entries = append(entries, util.VolumeClaimTemplateEntry{
			PersistentVolumeClaim: corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:        pvc.Name,
					Annotations: buildVolumeClaimTemplateAnnotations(pvc),
				},
				Spec: sanitizeVolumeClaimTemplateSpec(pvc),
			},
		})
	}

	volumeClaimTemplatesStr, err := util.MarshalVolumeClaimTemplates(entries)
	if err != nil {
		return "", fmt.Errorf("failed to marshal volumeClaimTemplates: %w", err)
	}
	return volumeClaimTemplatesStr, nil
}

// sanitizeVolumeClaimTemplateSpec returns the minimal PVC spec we want to
// persist in the volumeClaimTemplates annotation — enough for downstream
// consumers to recreate or migrate the volume, without dragging the entire
// status-tainted spec across.
func sanitizeVolumeClaimTemplateSpec(pvc *corev1.PersistentVolumeClaim) corev1.PersistentVolumeClaimSpec {
	spec := corev1.PersistentVolumeClaimSpec{
		AccessModes:      append([]corev1.PersistentVolumeAccessMode(nil), pvc.Spec.AccessModes...),
		StorageClassName: pvc.Spec.StorageClassName,
		VolumeMode:       pvc.Spec.VolumeMode,
	}
	if len(pvc.Spec.Resources.Requests) > 0 || len(pvc.Spec.Resources.Limits) > 0 {
		spec.Resources = corev1.VolumeResourceRequirements{}
		if len(pvc.Spec.Resources.Requests) > 0 {
			spec.Resources.Requests = pvc.Spec.Resources.Requests.DeepCopy()
		}
		if len(pvc.Spec.Resources.Limits) > 0 {
			spec.Resources.Limits = pvc.Spec.Resources.Limits.DeepCopy()
		}
	}
	return spec
}

// buildVolumeClaimTemplateAnnotations returns the only annotation we want to
// surface on each volumeClaimTemplate entry — the image ID, when present.
func buildVolumeClaimTemplateAnnotations(pvc *corev1.PersistentVolumeClaim) map[string]string {
	if val, ok := pvc.Annotations[util.AnnotationImageID]; ok {
		return map[string]string{util.AnnotationImageID: val}
	}
	return nil
}

func (h *RestoreHandler) createNewVM(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
) (*kubevirtv1.VirtualMachine, error) {
	targetName := h.vmro.GetTargetName(vmr)
	namespace := h.vmro.GetNamespace(vmr)
	keepMacAddress := h.vmro.IsKeepMacAddress(vmr)

	logrus.Infof("restore target does not exist, creating a new vm %s", targetName)

	sourceSpec := h.vmbo.GetSourceSpec(vmb)
	vm, err := h.buildVMFromRestore(vmr, sourceSpec, targetName, namespace)
	if err != nil {
		return nil, err
	}

	if !keepMacAddress {
		h.removeMacAddresses(vm)
	}

	return h.vmClient.Create(vm)
}

// buildVMFromRestore constructs a new VM object from restore and backup specs
func (h *RestoreHandler) buildVMFromRestore(
	vmr *harvesterv1.VirtualMachineRestore,
	sourceSpec *harvesterv1.VirtualMachineSourceSpec,
	targetName, namespace string,
) (*kubevirtv1.VirtualMachine, error) {
	vmAnnotations := h.buildVMAnnotations(vmr, sourceSpec)

	specAnnotations, err := h.buildVMSpecAnnotations(vmr, sourceSpec)
	if err != nil {
		return nil, err
	}

	// Create the VM Halted so KubeVirt doesn't spawn a VMI that races our
	// per-volume restore Jobs for still-being-populated PVCs. For fast restore
	// types (snapshot/longhorn) this is a no-op, but engines that write the
	// volume out of band (restic/kopia) need the VM to wait until their Jobs
	// have finished and the PVC is released — otherwise VirtLauncher attaches
	// the PVC while the restore is mid-flight and the guest boots from
	// inconsistent data. ensureVMStartedAndReady starts the VM via the /start
	// subresource once isVolumesReady becomes true.
	initialRunStrategy := kubevirtv1.RunStrategyHalted

	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        targetName,
			Namespace:   namespace,
			Annotations: vmAnnotations,
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			RunStrategy: &initialRunStrategy,
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: specAnnotations,
					Labels: map[string]string{
						util.LabelVMCreator: "harvester",
						util.LabelVMName:    targetName,
					},
				},
				Spec: h.vmro.SanitizeVMSpec(vmr, sourceSpec.Spec.Template.Spec),
			},
		},
	}

	newVolumes, err := h.vmro.MapVolumesToRestoredPVCs(vmr, &vm.Spec)
	if err != nil {
		return nil, err
	}
	vm.Spec.Template.Spec.Volumes = newVolumes

	// Populate volumeClaimTemplates from the restored PVCs so storage migration
	// and other PVC-management features see the actual disks on the new VM.
	volumeClaimTemplatesStr, err := h.buildVolumeClaimTemplates(vmr)
	if err != nil {
		return nil, fmt.Errorf("failed to build volumeClaimTemplates annotation: %w", err)
	}
	vm.Annotations[util.AnnotationVolumeClaimTemplates] = volumeClaimTemplatesStr

	return vm, nil
}

// buildVMAnnotations creates annotations for the new VM
func (h *RestoreHandler) buildVMAnnotations(
	vmr *harvesterv1.VirtualMachineRestore,
	sourceSpec *harvesterv1.VirtualMachineSourceSpec,
) map[string]string {
	restoreID := h.vmro.GetRestoreID(vmr)
	restoreName := h.vmro.GetName(vmr)

	annotations := map[string]string{
		lastRestoreAnnotation:               restoreID,
		restorecommon.RestoreNameAnnotation: restoreName,
	}

	// Preserve specific annotations from source VM
	preservedAnnotations := []string{
		util.AnnotationReservedMemory,
		util.AnnotationEnableCPUAndMemoryHotplug,
	}

	for _, annotation := range preservedAnnotations {
		if value, ok := sourceSpec.ObjectMeta.Annotations[annotation]; ok {
			annotations[annotation] = value
		}
	}

	return annotations
}

// buildVMSpecAnnotations creates annotations for the VM spec template
func (h *RestoreHandler) buildVMSpecAnnotations(
	vmr *harvesterv1.VirtualMachineRestore,
	sourceSpec *harvesterv1.VirtualMachineSourceSpec,
) (map[string]string, error) {
	return h.vmro.SanitizeVMAnnotations(vmr, sourceSpec.Spec.Template.ObjectMeta.Annotations)
}

// getDefaultRunStrategy returns the run strategy for the restored VM.
// HaltAfterRestore wins over the source VM's run strategy; if neither is set
// we fall back to RerunOnFailure.
func (h *RestoreHandler) getDefaultRunStrategy(
	vmr *harvesterv1.VirtualMachineRestore,
	sourceSpec *harvesterv1.VirtualMachineSourceSpec,
) kubevirtv1.VirtualMachineRunStrategy {
	if h.vmro.IsKeepHaltedAfterRestore(vmr) {
		return kubevirtv1.RunStrategyHalted
	}
	if sourceSpec != nil && sourceSpec.Spec.RunStrategy != nil {
		return *sourceSpec.Spec.RunStrategy
	}
	return kubevirtv1.RunStrategyRerunOnFailure
}

// removeMacAddresses removes MAC addresses from all network interfaces
func (h *RestoreHandler) removeMacAddresses(vm *kubevirtv1.VirtualMachine) {
	for i := range vm.Spec.Template.Spec.Domain.Devices.Interfaces {
		vm.Spec.Template.Spec.Domain.Devices.Interfaces[i].MacAddress = ""
	}
}

// updateStatus receives the original cache vmr plus the engine-mutated
// vmrCpy. vmrCpy is the basis for any further status mutations and the "new"
// side of operator Updates; vmr stays as the unmodified "old" side so diff
// checks compare against actual etcd state.
func (h *RestoreHandler) updateStatus(
	vmr *harvesterv1.VirtualMachineRestore,
	vmrCpy *harvesterv1.VirtualMachineRestore,
	vm *kubevirtv1.VirtualMachine,
	isVolumesReady bool,
) error {
	// Wait for volumes if not ready
	if !isVolumesReady {
		return h.handleVolumesNotReady(vmr, vmrCpy)
	}

	// Ensure VM is started and ready
	if err := h.ensureVMStartedAndReady(vmr, vmrCpy, vm); err != nil {
		return err
	}

	// 4. Cleanup and complete
	return h.finalizeRestore(vmr, vmrCpy, vm)
}

// updateProgressMetrics calculates and updates the restore progress based on volume restore status
func (h *RestoreHandler) updateProgressMetrics(
	vmrCpy *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
) error {
	re := h.getRestoreEngine(vmb)
	if re == nil {
		return fmt.Errorf("unsupported backup type: %s", vmb.Spec.Type)
	}

	// Update progress for each volume restore
	vrs := h.vmro.GetVolRestores(vmrCpy)
	for i := range vrs {
		vr := h.vmro.GetVolRestore(vmrCpy, i)
		progress, err := re.UpdateProgress(vr)
		if err != nil {
			return err
		}
		if err := h.vmro.SetVolRestoreProgress(vr, int(progress)); err != nil {
			return err
		}
	}

	return nil
}

// handleVolumesNotReady persists the "Creating new PVCs" progressing condition
// and schedules a fixed-interval re-reconcile so progress is sampled regularly
// instead of through the workqueue's exponential backoff (which quickly grows
// to multi-minute intervals during a long restore and leaves status.progress
// stuck at its initial value until the Job's terminal event fires).
func (h *RestoreHandler) handleVolumesNotReady(
	vmr *harvesterv1.VirtualMachineRestore,
	vmrCpy *harvesterv1.VirtualMachineRestore,
) error {
	h.vmro.RectifyProgressBeforeVMStart(vmrCpy)
	vmrCpy = h.vmro.SetProcessingCondition(vmrCpy, "Creating new PVCs")
	if _, err := h.vmro.Update(vmr, vmrCpy); err != nil {
		return err
	}
	h.vmrController.EnqueueAfter(vmr.Namespace, vmr.Name, restoreProgressPoll)
	return nil
}

// ensureVMStartedAndReady starts the VM if needed and waits for it to be ready.
// When HaltAfterRestore is set the VM is created Halted and we intentionally
// skip both the start subresource call and the ready wait, allowing the
// pipeline to fall through to finalizeRestore.
func (h *RestoreHandler) ensureVMStartedAndReady(
	vmr *harvesterv1.VirtualMachineRestore,
	vmrCpy *harvesterv1.VirtualMachineRestore,
	vm *kubevirtv1.VirtualMachine,
) error {
	if h.vmro.IsKeepHaltedAfterRestore(vmr) {
		return nil
	}

	// Start VM before checking status
	if err := h.vmro.StartVM(h.context, vm); err != nil {
		return h.vmro.UpdateError(vmr, fmt.Errorf("failed to start vm, err:%s", err.Error()))
	}

	if vm.Status.Ready {
		return nil
	}

	// VM not ready yet: persist progressing condition and halt the pipeline so
	// updateStatus does NOT fall through to finalizeRestore. The VMOnChange
	// handler will requeue the restore once the VM transitions to Ready.
	h.vmro.RectifyProgressBeforeVMStart(vmrCpy)
	vmrCpy = h.vmro.SetProcessingCondition(vmrCpy, "Waiting for target vm to be ready")
	if _, err := h.vmro.Update(vmr, vmrCpy); err != nil {
		return err
	}
	return fmt.Errorf("vm %s/%s is not ready yet", vm.Namespace, vm.Name)
}

// finalizeRestore performs cleanup and marks the restore as complete
func (h *RestoreHandler) finalizeRestore(
	vmr *harvesterv1.VirtualMachineRestore,
	vmrCpy *harvesterv1.VirtualMachineRestore,
	vm *kubevirtv1.VirtualMachine,
) error {
	if err := h.vmro.DeleteOldPVCs(vmrCpy, vm); err != nil {
		return h.vmro.UpdateError(vmr, fmt.Errorf("error cleaning up: %w", err))
	}
	vmrCpy = h.vmro.SetCompleteCondition(vmrCpy)
	_, err := h.vmro.Update(vmr, vmrCpy)
	return err
}
