package backup

// Harvester VM backup & restore controllers helps to manage the VM backup & restore by leveraging
// the VolumeSnapshot functionality of Kubernetes CSI drivers with built-in storage driver longhorn.
// Currently, the following features are supported:
// 1. support VM live & offline backup to the supported backupTarget(i.e, nfs_v4 or s3 storage server).
// 2. restore a backup to a new VM or replacing it with the existing VM is supported.
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	// Although we don't use following drivers directly, we need to import them to register drivers.
	// NFS Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/nfs/nfs.go#L47-L51
	// S3 Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/s3/s3.go#L33-L37
	_ "github.com/longhorn/backupstore/nfs" //nolint
	_ "github.com/longhorn/backupstore/s3"  //nolint
	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/backup/engine"
	"github.com/harvester/harvester/pkg/backup/engine/longhorn"
	"github.com/harvester/harvester/pkg/backup/engine/snapshot"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/scheme"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllonghornv2 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/settings"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
)

const (
	backupControllerName   = "harvester-vm-backup-controller"
	snapshotControllerName = "volume-snapshot-controller"

	vmBackupKindName = "VirtualMachineBackup"

	// engineRetryDelay is the delay before retrying volume backup creation when engine returns ErrRetryLater
	engineRetryDelay = 5 * time.Second
)

var vmBackupKind = harvesterv1.SchemeGroupVersion.WithKind(vmBackupKindName)

// RegisterBackup register the vmBackup and volumeSnapshot controller
func RegisterBackup(ctx context.Context, management *config.Management, _ config.Options) error {
	// Get all required controllers and caches
	controllers := getBackupControllers(management)

	// Initialize REST client for Kubevirt
	restClient, err := newKubevirtClient(management.RestConfig)
	if err != nil {
		return err
	}

	// Initialize operators
	vmbo := newBackupOperator(controllers, restClient)

	// Initialize backup engines
	engines := newBackupEngines(controllers, vmbo)

	// Let each engine wire up its own informer event handlers (e.g. Job
	// watchers) so engine-owned resource changes feed back into VMBackup
	// reconciles immediately instead of waiting on poll/requeue.
	for _, e := range engines {
		e.RegisterWatchers(ctx, controllers.vmbs.Enqueue)
	}

	// Create and configure handler
	handler := newBackupHandler(controllers, vmbo, engines)

	// Register event handlers
	registerBackupEventHandlers(ctx, controllers, handler)

	return nil
}

// backupControllerSet holds all required controllers and caches for backup operations
type backupControllerSet struct {
	vmbs           ctlharvesterv1.VirtualMachineBackupController
	vms            ctlkubevirtv1.VirtualMachineController
	vmis           ctlkubevirtv1.VirtualMachineInstanceController
	pvs            ctlcorev1.PersistentVolumeController
	pvcs           ctlcorev1.PersistentVolumeClaimController
	secrets        ctlcorev1.SecretController
	storageClasses ctlstoragev1.StorageClassController
	lhbackups      ctllonghornv2.BackupController
	volumes        ctllonghornv2.VolumeController
	vss            ctlsnapshotv1.VolumeSnapshotController
	vscs           ctlsnapshotv1.VolumeSnapshotContentController
	vsClasses      ctlsnapshotv1.VolumeSnapshotClassController
	jobs           ctlbatchv1.JobController
}

// getBackupControllers extracts all required controllers from management
func getBackupControllers(management *config.Management) *backupControllerSet {
	return &backupControllerSet{
		vmbs:           management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup(),
		vms:            management.VirtFactory.Kubevirt().V1().VirtualMachine(),
		vmis:           management.VirtFactory.Kubevirt().V1().VirtualMachineInstance(),
		pvs:            management.CoreFactory.Core().V1().PersistentVolume(),
		pvcs:           management.CoreFactory.Core().V1().PersistentVolumeClaim(),
		secrets:        management.CoreFactory.Core().V1().Secret(),
		storageClasses: management.StorageFactory.Storage().V1().StorageClass(),
		lhbackups:      management.LonghornFactory.Longhorn().V1beta2().Backup(),
		volumes:        management.LonghornFactory.Longhorn().V1beta2().Volume(),
		vss:            management.SnapshotFactory.Snapshot().V1().VolumeSnapshot(),
		vscs:           management.SnapshotFactory.Snapshot().V1().VolumeSnapshotContent(),
		vsClasses:      management.SnapshotFactory.Snapshot().V1().VolumeSnapshotClass(),
		jobs:           management.BatchFactory.Batch().V1().Job(),
	}
}

// newKubevirtClient creates a REST client for the KubeVirt subresources API
// (used for filesystem freeze/unfreeze during backup, and VM start during restore).
func newKubevirtClient(restConfig *rest.Config) (*rest.RESTClient, error) {
	cfg := rest.CopyConfig(restConfig)
	cfg.GroupVersion = &k8sschema.GroupVersion{
		Group:   kubevirtv1.SubresourceGroupName,
		Version: kubevirtv1.ApiLatestVersion,
	}
	cfg.APIPath = "/apis"
	cfg.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	return rest.RESTClientFor(cfg)
}

// newBackupOperator creates and configures the VMBackup operator
func newBackupOperator(
	controllers *backupControllerSet,
	restClient *rest.RESTClient,
) common.VMBackupOperator {
	return common.NewVMBackupOperatorBuilder().
		WithClient(controllers.vmbs).
		WithCache(controllers.vmbs.Cache()).
		WithVSClassCache(controllers.vsClasses.Cache()).
		WithVMCache(controllers.vms.Cache()).
		WithVMICache(controllers.vmis.Cache()).
		WithPVCCache(controllers.pvcs.Cache()).
		WithPVCache(controllers.pvs.Cache()).
		WithSecretCache(controllers.secrets.Cache()).
		WithVirtSubresourceRestClient(restClient).
		Build()
}

// newBackupEngines creates backup engines for different backup types
func newBackupEngines(
	controllers *backupControllerSet,
	vmbo common.VMBackupOperator,
) map[harvesterv1.BackupType]engine.BackupEngine {
	return map[harvesterv1.BackupType]engine.BackupEngine{
		harvesterv1.Snapshot: snapshot.GetBackupEngine(
			vmbo,
			controllers.vss.Cache(),
			controllers.vss,
			controllers.pvcs.Cache(),
			controllers.storageClasses.Cache(),
		),
		harvesterv1.Backup: longhorn.GetBackupEngine(
			vmbo,
			controllers.vss.Cache(),
			controllers.vss,
			controllers.vscs.Cache(),
			controllers.vscs,
			controllers.pvcs.Cache(),
			controllers.storageClasses.Cache(),
			controllers.lhbackups.Cache(),
			controllers.lhbackups,
			controllers.vmbs.Cache(),
		),
	}
}

// newBackupHandler creates a new Handler with all dependencies
func newBackupHandler(
	controllers *backupControllerSet,
	vmbo common.VMBackupOperator,
	engines map[harvesterv1.BackupType]engine.BackupEngine,
) *Handler {
	return &Handler{
		vmbCache:      controllers.vmbs.Cache(),
		vmbController: controllers.vmbs,
		secretCache:   controllers.secrets.Cache(),
		vsCache:       controllers.vss.Cache(),
		vscCache:      controllers.vscs.Cache(),
		vmbo:          vmbo,
		engines:       engines,
	}
}

// registerBackupEventHandlers registers all event handlers for the backup controller.
func registerBackupEventHandlers(ctx context.Context, controllers *backupControllerSet, handler *Handler) {
	controllers.vmbs.OnChange(ctx, backupControllerName, handler.OnBackupChange)
	controllers.vmbs.OnRemove(ctx, backupControllerName, handler.OnBackupRemove)
	controllers.vss.OnChange(ctx, snapshotControllerName, handler.updateVolumeSnapshotChanged)
}

type Handler struct {
	vmbCache      ctlharvesterv1.VirtualMachineBackupCache
	vmbController ctlharvesterv1.VirtualMachineBackupController
	secretCache   ctlcorev1.SecretCache
	vsCache       ctlsnapshotv1.VolumeSnapshotCache
	vscCache      ctlsnapshotv1.VolumeSnapshotContentCache
	vmbo          common.VMBackupOperator
	engines       map[harvesterv1.BackupType]engine.BackupEngine
}

// getBackupEngine selects the appropriate backup engine based on backup type
func (h *Handler) getBackupEngine(vmb *harvesterv1.VirtualMachineBackup) engine.BackupEngine {
	if engine, exists := h.engines[h.vmbo.GetType(vmb)]; exists {
		return engine
	}
	return nil
}

// OnBackupChange handles vm backup object on change and reconcile vm backup status
func (h *Handler) OnBackupChange(_ string, vmb *harvesterv1.VirtualMachineBackup) (*harvesterv1.VirtualMachineBackup, error) {
	if vmb == nil || h.vmbo.GetDeletionTimestap(vmb) != nil {
		return nil, nil
	}

	if h.vmbo.IsReady(vmb) {
		return nil, h.handleBackupReady(vmb)
	}

	logrus.Debugf("OnBackupChange: vmBackup name:%s", vmb.Name)
	// set vmBackup init status
	if h.vmbo.IsMissingStatus(vmb) {
		// We cannot get VM outside this block, because we also can sync VMBackup from remote target.
		// A VMBackup without status is a new VMBackup, so it must have sourceVM.
		sourceVM, err := h.vmbo.ResolveSourceVM(vmb)
		if err != nil {
			return nil, h.vmbo.UpdateError(vmb, err)
		}

		if err = h.vmbo.InitVMBackup(vmb, sourceVM); err != nil {
			return nil, h.vmbo.UpdateError(vmb, err)
		}

		return nil, nil
	}

	// TODO, make sure status is initialized, and "Lock" the source VM by adding a finalizer and setting snapshotInProgress in status

	_, csiVSClassMap, err := h.vmbo.BuildCSIDriverMap(vmb)
	if err != nil {
		return nil, h.vmbo.UpdateError(vmb, err)
	}

	// create volume snapshots if not exist
	if err := h.CreateVolumeBackups(vmb, csiVSClassMap); err != nil {
		return nil, h.vmbo.UpdateError(vmb, err)
	}

	// reconcile backup status of volume backups, validate if those volumeSnapshots are ready to use
	if err := h.updateConditions(vmb); err != nil {
		return nil, err
	}

	return nil, nil
}

// OnBackupRemove removes remote vm backup metadata when a VMBackup is deleted.
// It handles two scenarios:
// 1. Delete backup metadata from the backup target if it's not the default target
// 2. Delete volume backups if the backup target has changed
func (h *Handler) OnBackupRemove(_ string, vmb *harvesterv1.VirtualMachineBackup) (*harvesterv1.VirtualMachineBackup, error) {
	// Skip if backup is nil or doesn't have status/target information
	if vmb == nil || h.vmbo.GetStatus(vmb) == nil || h.vmbo.GetBackupTarget(vmb) == nil {
		return nil, nil
	}

	// Get the current backup target configuration
	currentTarget, err := settings.DecodeBackupTarget(settings.BackupTargetSet.Get())
	if err != nil {
		return nil, fmt.Errorf("failed to decode backup target: %w", err)
	}

	// Delete metadata from backup target if it's configured (not default)
	if !currentTarget.IsDefaultBackupTarget() {
		if err := h.deleteVMBackupMetadata(vmb, currentTarget); err != nil {
			return nil, fmt.Errorf("failed to delete VM backup metadata: %w", err)
		}
	}

	// Force-delete volume backups in two cases:
	// 1) Target changed since creation — orphaned vol backups need cleanup
	//    regardless of type (legacy behavior for native Backup).
	// 2) The backup type owns remote state nothing else GCs for us (restic/
	//    kopia), so the engine must always get a chance to forget remote data.
	if !h.vmbo.IsTargetConsistent(vmb, currentTarget) || h.vmbo.GetType(vmb).OwnsExternalState() {
		if err := h.forceDeleteVolBackups(vmb); err != nil {
			return nil, fmt.Errorf("failed to delete volume backups: %w", err)
		}
	}

	return nil, nil
}

// CreateVolumeBackups creates volume snapshots for all volumes that need backup.
// For VM backups from an existing VM, we create volume snapshots from PVCs.
// For VM backups from syncing metadata, we create volume snapshots from volume snapshot content.
func (h *Handler) CreateVolumeBackups(
	vmb *harvesterv1.VirtualMachineBackup,
	csiVSClassMap map[string]snapshotv1.VolumeSnapshotClass,
) error {
	// Create a copy to track status changes during the loop
	vmbCpy := vmb.DeepCopy()

	vbs := h.vmbo.GetVolBackups(vmbCpy)
	for index, vb := range vbs {
		// Skip volumes that don't have a name assigned yet
		if h.vmbo.GetVolBackupName(&vb) == nil {
			continue
		}

		// Reconcile the per-volume backup using the appropriate engine
		backupEngine := h.getBackupEngine(vmbCpy)
		err := backupEngine.Reconcile(vmbCpy, index, csiVSClassMap)

		// Handle retry case - engine needs more time to complete the operation
		if err == engine.ErrRetryLater {
			namespace := h.vmbo.GetNamespace(vmbCpy)
			name := h.vmbo.GetName(vmbCpy)
			h.vmbController.EnqueueAfter(namespace, name, engineRetryDelay)
			return nil
		}

		if err != nil {
			return err
		}
	}

	// Update the VM backup status if any changes were made
	_, err := h.vmbo.UpdateByStatus(vmb, vmbCpy)
	return err
}

func (h *Handler) deleteVMBackupMetadata(vmb *harvesterv1.VirtualMachineBackup, target *settings.BackupTarget) error {
	var err error
	if target == nil {
		if target, err = settings.DecodeBackupTarget(settings.BackupTargetSet.Get()); err != nil {
			return err
		}
	}

	// when backup target has been reset to default, skip following
	if target.IsDefaultBackupTarget() {
		logrus.Debugf("vmBackup delete:%s, backup target is default, skip", vmb.Name)
		return nil
	}

	if !h.vmbo.IsTargetConsistent(vmb, target) {
		return nil
	}

	bsDriver, err := backuputil.GetBackupStoreDriver(h.secretCache, target)
	if err != nil {
		return err
	}

	destURL := getVMBackupMetadataFilePath(vmb.Namespace, vmb.Name)
	if exist := bsDriver.FileExists(destURL); exist {
		logrus.Debugf("delete vm backup metadata %s/%s in backup target %s", vmb.Namespace, vmb.Name, target.Type)
		return bsDriver.Remove(destURL)
	}

	return nil
}

func (h *Handler) uploadVMBackupMetadata(vmb *harvesterv1.VirtualMachineBackup) error {
	// if users don't update VMBackup CRD, we may lose backup target data.
	if h.vmbo.GetBackupTarget(vmb) == nil {
		return fmt.Errorf("no backup target in vmbackup.status")
	}

	target, err := settings.DecodeBackupTarget(settings.BackupTargetSet.Get())
	if err != nil {
		return err
	}

	// when current backup target is default, skip following steps
	// if backup target is default, IsBackupTargetSame is true when vmBackup.Status.BackupTarget is also default value
	if target.IsDefaultBackupTarget() {
		return nil
	}

	if !backuputil.IsBackupTargetSame(h.vmbo.GetBackupTarget(vmb), target) {
		return nil
	}

	bsDriver, err := backuputil.GetBackupStoreDriver(h.secretCache, target)
	if err != nil {
		return err
	}

	vmBackupMetadata := &VirtualMachineBackupMetadata{
		Name:          h.vmbo.GetName(vmb),
		Namespace:     h.vmbo.GetNamespace(vmb),
		BackupSpec:    h.vmbo.GetSpec(vmb),
		VMSourceSpec:  h.vmbo.GetSourceSpec(vmb),
		VolumeBackups: h.vmbo.GetSanitizeVolBackups(vmb),
		SecretBackups: h.vmbo.GetSecretBackups(vmb),
	}
	if h.vmbo.GetNamespace(vmb) == "" {
		vmBackupMetadata.Namespace = metav1.NamespaceDefault
	}

	j, err := json.Marshal(vmBackupMetadata)
	if err != nil {
		return err
	}

	destURL := getVMBackupMetadataFilePath(h.vmbo.GetNamespace(vmb), h.vmbo.GetName(vmb))

	// Decide whether the metadata file needs to be (re)written. Skip the write
	// if the remote already holds an identical payload — typical for a backup
	// recovered from the target.
	needsUpload := true
	if bsDriver.FileExists(destURL) {
		remote, err := loadBackupMetadataInBackupTarget(destURL, bsDriver)
		if err != nil {
			return err
		}
		if reflect.DeepEqual(vmBackupMetadata, remote) {
			needsUpload = false
		}
	}

	if needsUpload {
		logrus.Debugf("upload vm backup metadata %s/%s to backup target %s", vmb.Namespace, vmb.Name, target.Type)
		if err := bsDriver.Write(destURL, bytes.NewReader(j)); err != nil {
			return err
		}
	}

	// Always mark MetadataReady=True once the local view matches the remote,
	// even when the upload was skipped. Otherwise recovered backups never get
	// the MetadataReady condition set.
	return h.vmbo.UpdateMetadataReady(vmb)
}

func (h *Handler) forceDeleteVolBackups(vmb *harvesterv1.VirtualMachineBackup) error {
	backupEngine := h.getBackupEngine(vmb)

	for i, vb := range h.vmbo.GetVolBackups(vmb) {
		if h.vmbo.GetVolBackupName(&vb) == nil {
			continue
		}

		err := backupEngine.ForceDelete(vmb, i)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"vmBackup":     h.vmbo.GetName(vmb),
				"volumeBackup": *h.vmbo.GetVolBackupName(&vb),
				"namespace":    h.vmbo.GetNamespace(vmb),
				"backupType":   h.vmbo.GetType(vmb),
			}).Warn("failed to force delete volume backup")
			return err
		}
	}
	return nil
}

func (h *Handler) handleBackupReady(vmb *harvesterv1.VirtualMachineBackup) error {
	// We add CSIDriverVolumeSnapshotClassNameMap since v1.1.0.
	// For backport to v1.0.x, we construct the map from VolumeBackups.
	var err error
	if vmb, err = h.vmbo.ConfigureCSISnapClasses(vmb); err != nil {
		return err
	}

	// only backup type needs to configure backup target and upload metadata
	if h.vmbo.GetType(vmb) == harvesterv1.Snapshot {
		return nil
	}

	// We've changed backup target information to status since v1.0.0.
	// For backport to v0.3.0, we move backup target information from annotation to status.
	if vmb, err = h.vmbo.ConfigureBackupTargetOnStatus(vmb); err != nil {
		return err
	}

	// generate vm backup metadata and upload to backup target
	return h.uploadVMBackupMetadata(vmb)
}
