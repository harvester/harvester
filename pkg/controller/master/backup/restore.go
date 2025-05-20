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
	"strconv"
	"strings"
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	lhdatastore "github.com/longhorn/longhorn-manager/datastore"
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/types"
	lhutil "github.com/longhorn/longhorn-manager/util"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/rancher/wrangler/v3/pkg/name"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/scheme"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

var (
	restoreAnnotationsToDelete = []string{
		"pv.kubernetes.io",
	}
)

const (
	restoreControllerName = "harvester-vm-restore-controller"

	volumeSnapshotKindName = "VolumeSnapshot"
	vmRestoreKindName      = "VirtualMachineRestore"

	restoreNameAnnotation  = "restore.harvesterhci.io/name"
	lastRestoreAnnotation  = "restore.harvesterhci.io/last-restore-uid"
	pvcNameSpaceAnnotation = "pvc.harvesterhci.io/namespace"
	pvcNameAnnotation      = "pvc.harvesterhci.io/name"

	restoreErrorEvent    = "VirtualMachineRestoreError"
	restoreCompleteEvent = "VirtualMachineRestoreComplete"

	restoreProgressBeforeVMStart = 90
	restoreProgressComplete      = 100

	pvNamePrefix = "pvc"

	//not truncate or remove dashes pv name
	volumeNameUUIDNoTruncate = -1
)

type RestoreHandler struct {
	context context.Context

	restores             ctlharvesterv1.VirtualMachineRestoreClient
	restoreController    ctlharvesterv1.VirtualMachineRestoreController
	restoreCache         ctlharvesterv1.VirtualMachineRestoreCache
	vmBackupController   ctlharvesterv1.VirtualMachineBackupController
	vmBackupClient       ctlharvesterv1.VirtualMachineBackupClient
	backupCache          ctlharvesterv1.VirtualMachineBackupCache
	vms                  ctlkubevirtv1.VirtualMachineClient
	vmCache              ctlkubevirtv1.VirtualMachineCache
	vmis                 ctlkubevirtv1.VirtualMachineInstanceClient
	vmiCache             ctlkubevirtv1.VirtualMachineInstanceCache
	pvcClient            ctlcorev1.PersistentVolumeClaimClient
	pvcCache             ctlcorev1.PersistentVolumeClaimCache
	pvCache              ctlcorev1.PersistentVolumeCache
	scCache              ctlstoragev1.StorageClassCache
	secretClient         ctlcorev1.SecretClient
	secretCache          ctlcorev1.SecretCache
	snapshots            ctlsnapshotv1.VolumeSnapshotClient
	snapshotCache        ctlsnapshotv1.VolumeSnapshotCache
	snapshotContents     ctlsnapshotv1.VolumeSnapshotContentClient
	snapshotContentCache ctlsnapshotv1.VolumeSnapshotContentCache
	lhbackupCache        ctllhv1.BackupCache
	lhbackupVolumeCache  ctllhv1.BackupVolumeCache
	volumeCache          ctllhv1.VolumeCache
	volumes              ctllhv1.VolumeClient
	lhengineCache        ctllhv1.EngineCache

	recorder   record.EventRecorder
	restClient *rest.RESTClient
}

func RegisterRestore(ctx context.Context, management *config.Management, _ config.Options) error {
	restores := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineRestore()
	backups := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	vmis := management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	pvcs := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	pvs := management.CoreFactory.Core().V1().PersistentVolume()
	scs := management.StorageFactory.Storage().V1().StorageClass()
	secrets := management.CoreFactory.Core().V1().Secret()
	snapshots := management.SnapshotFactory.Snapshot().V1().VolumeSnapshot()
	snapshotContents := management.SnapshotFactory.Snapshot().V1().VolumeSnapshotContent()
	lhbackups := management.LonghornFactory.Longhorn().V1beta2().Backup()
	lhbackupVolumes := management.LonghornFactory.Longhorn().V1beta2().BackupVolume()
	volumes := management.LonghornFactory.Longhorn().V1beta2().Volume()
	lhengines := management.LonghornFactory.Longhorn().V1beta2().Engine()

	copyConfig := rest.CopyConfig(management.RestConfig)
	copyConfig.GroupVersion = &k8sschema.GroupVersion{Group: kubevirtv1.SubresourceGroupName, Version: kubevirtv1.ApiLatestVersion}
	copyConfig.APIPath = "/apis"
	copyConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	restClient, err := rest.RESTClientFor(copyConfig)
	if err != nil {
		return err
	}

	handler := &RestoreHandler{
		context:              ctx,
		restores:             restores,
		restoreController:    restores,
		restoreCache:         restores.Cache(),
		vmBackupClient:       backups,
		vmBackupController:   backups,
		backupCache:          backups.Cache(),
		vms:                  vms,
		vmCache:              vms.Cache(),
		vmis:                 vmis,
		vmiCache:             vmis.Cache(),
		pvcClient:            pvcs,
		pvcCache:             pvcs.Cache(),
		pvCache:              pvs.Cache(),
		scCache:              scs.Cache(),
		secretClient:         secrets,
		secretCache:          secrets.Cache(),
		snapshots:            snapshots,
		snapshotCache:        snapshots.Cache(),
		snapshotContents:     snapshotContents,
		snapshotContentCache: snapshotContents.Cache(),
		lhbackupCache:        lhbackups.Cache(),
		lhbackupVolumeCache:  lhbackupVolumes.Cache(),
		volumes:              volumes,
		volumeCache:          volumes.Cache(),
		lhengineCache:        lhengines.Cache(),
		recorder:             management.NewRecorder(restoreControllerName, "", ""),
		restClient:           restClient,
	}

	restores.OnChange(ctx, restoreControllerName, handler.RestoreOnChanged)
	restores.OnRemove(ctx, restoreControllerName, handler.RestoreOnRemove)
	pvcs.OnChange(ctx, restoreControllerName, handler.PersistentVolumeClaimOnChange)
	lhengines.OnChange(ctx, restoreControllerName, handler.LHEngineOnChange)
	vms.OnChange(ctx, restoreControllerName, handler.VMOnChange)
	return nil
}

// RestoreOnChanged handles vmRestore CRD object on change, it will help to create the new PVCs and either replace them
// with existing VM or used for the new VM.
func (h *RestoreHandler) RestoreOnChanged(_ string, restore *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error) {
	if restore == nil || restore.DeletionTimestamp != nil {
		return nil, nil
	}

	if !isVMRestoreProgressing(restore) {
		return nil, nil
	}

	if restore.Status == nil {
		return nil, h.initStatus(restore)
	}

	backup, err := h.getVMBackup(restore)
	if err != nil {
		return nil, h.updateStatusError(restore, err, true)
	}

	if isVMRestoreMissingVolumes(restore) {
		return nil, h.initVolumesStatus(restore, backup)
	}

	vm, isVolumesReady, err := h.reconcileResources(restore, backup)
	if err != nil {
		return nil, h.updateStatusError(restore, err, true)
	}

	// set vmRestore owner reference to the target VM
	if len(restore.OwnerReferences) == 0 {
		return nil, h.updateOwnerRefAndTargetUID(restore, vm)
	}

	return nil, h.updateStatus(restore, backup, vm, isVolumesReady)
}

// RestoreOnRemove delete VolumeSnapshotContent which is created by restore controller
// Since we would like to prevent LH Backups from being removed when users delete the VM,
// we use Retain policy in VolumeSnapshotContent.
// We need to delete VolumeSnapshotContent by restore controller, or there will have remaining VolumeSnapshotContent in the system.
func (h *RestoreHandler) RestoreOnRemove(_ string, restore *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error) {
	if restore == nil || restore.Status == nil {
		return nil, nil
	}

	backup, err := h.getVMBackup(restore)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		return nil, nil
	}

	for _, volumeBackup := range backup.Status.VolumeBackups {
		if volumeSnapshotContent, err := h.snapshotContentCache.Get(h.constructVolumeSnapshotContentName(restore.Namespace, restore.Name, *volumeBackup.Name)); err != nil {
			if err != nil && !apierrors.IsNotFound(err) {
				return nil, err
			}
		} else {
			if err = h.snapshotContents.Delete(volumeSnapshotContent.Name, &metav1.DeleteOptions{}); err != nil {
				return nil, err
			}
		}
	}

	return nil, nil
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

	h.restoreController.Enqueue(pvc.Namespace, restore)
	return nil, nil
}

// PersistentVolumeClaimOnChange watching the PVCs on change and enqueue the vmRestore if it has the restore annotation
func (h *RestoreHandler) PersistentVolumeClaimOnChange(_ string, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	if pvc == nil || pvc.DeletionTimestamp != nil {
		return nil, nil
	}

	restoreName, ok := pvc.Annotations[restoreNameAnnotation]
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

	volumeCopy.Annotations[pvcNameSpaceAnnotation] = pvc.Namespace
	volumeCopy.Annotations[pvcNameAnnotation] = pvc.Name
	volumeCopy.Annotations[restoreNameAnnotation] = restoreName

	if !reflect.DeepEqual(volume, volumeCopy) {
		if _, err := h.volumes.Update(volumeCopy); err != nil {
			return nil, err
		}
	}

	logrus.Debugf("handling PVC updating %s/%s", pvc.Namespace, pvc.Name)
	h.restoreController.EnqueueAfter(pvc.Namespace, restoreName, 5*time.Second)
	return nil, nil
}

func (h *RestoreHandler) LHEngineOnChange(_ string, lhEngine *lhv1beta2.Engine) (*lhv1beta2.Engine, error) {
	if lhEngine == nil || lhEngine.DeletionTimestamp != nil || len(lhEngine.Status.RestoreStatus) == 0 {
		return nil, nil
	}

	volume, err := h.volumeCache.Get(util.LonghornSystemNamespaceName, lhEngine.Spec.VolumeName)
	if err != nil {
		return nil, err
	}

	pvcNamespace, ok := volume.Annotations[pvcNameSpaceAnnotation]
	if !ok {
		return nil, nil
	}

	pvcName, ok := volume.Annotations[pvcNameAnnotation]
	if !ok {
		return nil, nil
	}

	restoreName, ok := volume.Annotations[restoreNameAnnotation]
	if !ok {
		return nil, nil
	}

	//vmRestore could be deleted after restoere completed
	vmRestore, err := h.restoreCache.Get(pvcNamespace, restoreName)
	if err != nil {
		return nil, nil
	}

	vmRestoreCpy := vmRestore.DeepCopy()
	for i, volumeRestore := range vmRestore.Status.VolumeRestores {
		if volumeRestore.PersistentVolumeClaim.ObjectMeta.Namespace != pvcNamespace {
			continue
		}

		if volumeRestore.PersistentVolumeClaim.ObjectMeta.Name != pvcName {
			continue
		}

		vmRestoreCpy.Status.VolumeRestores[i].LonghornEngineName = pointer.String(lhEngine.Name)
		vmRestoreCpy.Status.VolumeRestores[i].VolumeSize = volume.Spec.Size
		break
	}

	if !reflect.DeepEqual(vmRestore.Status, vmRestoreCpy.Status) {
		if _, err := h.restores.Update(vmRestoreCpy); err != nil {
			return nil, err
		}
	}

	//enqueue to trigger progress update
	h.restoreController.Enqueue(pvcNamespace, restoreName)
	return nil, nil
}

// VMOnChange watching the VM on change and enqueue the vmRestore if it has the restore annotation
func (h *RestoreHandler) VMOnChange(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil {
		return nil, nil
	}

	restoreName, ok := vm.Annotations[restoreNameAnnotation]
	if !ok {
		return nil, nil
	}

	logrus.Debugf("handling VM updating %s/%s", vm.Namespace, vm.Name)
	h.restoreController.EnqueueAfter(vm.Namespace, restoreName, 5*time.Second)
	return nil, nil
}

func (h *RestoreHandler) initStatus(restore *harvesterv1.VirtualMachineRestore) error {
	restoreCpy := restore.DeepCopy()

	restoreCpy.Status = &harvesterv1.VirtualMachineRestoreStatus{
		Complete: pointer.BoolPtr(false),
		Conditions: []harvesterv1.Condition{
			newProgressingCondition(corev1.ConditionTrue, "", "Initializing VirtualMachineRestore"),
			newReadyCondition(corev1.ConditionFalse, "", "Initializing VirtualMachineRestore"),
		},
	}

	if _, err := h.restores.Update(restoreCpy); err != nil {
		return err
	}
	return nil
}

func (h *RestoreHandler) initVolumesStatus(vmRestore *harvesterv1.VirtualMachineRestore, backup *harvesterv1.VirtualMachineBackup) error {
	restoreCpy := vmRestore.DeepCopy()

	if restoreCpy.Status.VolumeRestores == nil {
		volumeRestores, err := getVolumeRestores(restoreCpy, backup)
		if err != nil {
			return err
		}
		restoreCpy.Status.VolumeRestores = volumeRestores
	}

	if !IsNewVMOrHasRetainPolicy(vmRestore) && vmRestore.Status.DeletedVolumes == nil {
		var deletedVolumes []string
		for _, vol := range backup.Status.VolumeBackups {
			deletedVolumes = append(deletedVolumes, vol.PersistentVolumeClaim.ObjectMeta.Name)
		}
		restoreCpy.Status.DeletedVolumes = deletedVolumes
	}

	if _, err := h.restores.Update(restoreCpy); err != nil {
		return err
	}
	return nil
}

// getVM returns restore target VM
func (h *RestoreHandler) getVM(vmRestore *harvesterv1.VirtualMachineRestore) (*kubevirtv1.VirtualMachine, error) {
	switch vmRestore.Spec.Target.Kind {
	case kubevirtv1.VirtualMachineGroupVersionKind.Kind:
		vm, err := h.vmCache.Get(vmRestore.Namespace, vmRestore.Spec.Target.Name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}

		return vm, nil
	}

	return nil, fmt.Errorf("unknown target %+v", vmRestore.Spec.Target)
}

// getVolumeRestores helps to create an array of new restored volumes
func getVolumeRestores(vmRestore *harvesterv1.VirtualMachineRestore, backup *harvesterv1.VirtualMachineBackup) ([]harvesterv1.VolumeRestore, error) {
	restores := make([]harvesterv1.VolumeRestore, 0, len(backup.Status.VolumeBackups))
	for _, vb := range backup.Status.VolumeBackups {
		found := false
		for _, vr := range vmRestore.Status.VolumeRestores {
			if vb.VolumeName == vr.VolumeName {
				restores = append(restores, vr)
				found = true
				break
			}
		}

		if !found {
			if vb.Name == nil {
				return nil, fmt.Errorf("VolumeSnapshotName missing %+v", vb)
			}

			vr := harvesterv1.VolumeRestore{
				VolumeName: vb.VolumeName,
				PersistentVolumeClaim: harvesterv1.PersistentVolumeClaimSourceSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name:      getRestorePVCName(vmRestore, vb.VolumeName),
						Namespace: vmRestore.Namespace,
					},
					Spec: vb.PersistentVolumeClaim.Spec,
				},
				VolumeBackupName: *vb.Name,
			}
			restores = append(restores, vr)
		}
	}
	return restores, nil
}

func (h *RestoreHandler) reconcileResources(
	vmRestore *harvesterv1.VirtualMachineRestore,
	backup *harvesterv1.VirtualMachineBackup,
) (*kubevirtv1.VirtualMachine, bool, error) {
	// reconcile restoring volumes and create new PVC from CSI volumeSnapshot if not exist
	isVolumesReady, err := h.reconcileVolumeRestores(vmRestore, backup)
	if err != nil {
		return nil, false, err
	}

	// reconcile VM
	vm, err := h.reconcileVM(vmRestore, backup)
	if err != nil {
		return nil, false, err
	}

	//restore referenced secrets
	if err := h.reconcileSecretBackups(vmRestore, backup, vm); err != nil {
		return nil, false, err
	}

	return vm, isVolumesReady, nil
}

func (h *RestoreHandler) reconcileVolumeRestores(
	vmRestore *harvesterv1.VirtualMachineRestore,
	backup *harvesterv1.VirtualMachineBackup,
) (bool, error) {
	isVolumesReady := true
	for i, volumeRestore := range vmRestore.Status.VolumeRestores {
		pvc, err := h.pvcCache.Get(vmRestore.Namespace, volumeRestore.PersistentVolumeClaim.ObjectMeta.Name)
		if apierrors.IsNotFound(err) {
			volumeBackup := backup.Status.VolumeBackups[i]
			if err = h.createRestoredPVC(vmRestore, volumeBackup, volumeRestore); err != nil {
				return false, err
			}
			isVolumesReady = false
			continue
		}
		if err != nil {
			return false, err
		}

		if pvc.Status.Phase == corev1.ClaimPending {
			isVolumesReady = false
			continue
		}

		if util.GetProvisionedPVCProvisioner(pvc, h.scCache) == types.LonghornDriverName {
			volumeName, err := getVolumeName(pvc)
			if err != nil {
				return false, err
			}

			volume, err := h.volumeCache.Get(util.LonghornSystemNamespaceName, volumeName)
			if err != nil {
				return false, err
			}
			for _, condition := range volume.Status.Conditions {
				if condition.Type == lhv1beta2.VolumeConditionTypeRestore && condition.Reason == lhv1beta2.VolumeConditionReasonRestoreFailure {
					return false, fmt.Errorf("volume %s/%s restore failed: %s", volume.Namespace, volume.Name, condition.Message)
				}
			}
		}

		if pvc.Status.Phase != corev1.ClaimBound {
			return false, fmt.Errorf("PVC %s/%s in status %q", pvc.Namespace, pvc.Name, pvc.Status.Phase)
		}
	}
	return isVolumesReady, nil
}

func (h *RestoreHandler) reconcileVM(
	vmRestore *harvesterv1.VirtualMachineRestore,
	backup *harvesterv1.VirtualMachineBackup,
) (*kubevirtv1.VirtualMachine, error) {
	// create new VM if it's not exist
	vm, err := h.getVM(vmRestore)
	if err != nil {
		return nil, err
	} else if vm == nil && err == nil {
		vm, err = h.createNewVM(vmRestore, backup)
		if err != nil {
			return nil, err
		}
	}

	// make sure target VM has correct annotations
	restoreID := getRestoreID(vmRestore)
	if lastRestoreID, ok := vm.Annotations[lastRestoreAnnotation]; ok && lastRestoreID == restoreID {
		return vm, nil
	}

	// VM doesn't have correct annotations like restore to existing VM.
	// We update its volumes to new reconsile volumes
	newVolumes, err := getNewVolumes(&backup.Status.SourceSpec.Spec, vmRestore)
	if err != nil {
		return nil, err
	}

	vmCpy := vm.DeepCopy()
	vmCpy.Spec = backup.Status.SourceSpec.Spec

	//if the source runStratedy is RerunOnFailure, Kubevirt will not start the new VMI
	//set the VM runStrategy as Halted, VMI will be kicked off in startVM()
	haltedRunStrategy := kubevirtv1.RunStrategyHalted
	vmCpy.Spec.RunStrategy = &haltedRunStrategy

	vmCpy.Spec.Template.Spec.Volumes = newVolumes
	if vmCpy.Annotations == nil {
		vmCpy.Annotations = make(map[string]string)
	}
	vmCpy.Annotations[lastRestoreAnnotation] = restoreID
	vmCpy.Annotations[restoreNameAnnotation] = vmRestore.Name
	delete(vmCpy.Annotations, util.AnnotationVolumeClaimTemplates)

	if vm, err = h.vms.Update(vmCpy); err != nil {
		return nil, err
	}

	return vm, nil
}

func (h *RestoreHandler) reconcileSecretBackups(
	vmRestore *harvesterv1.VirtualMachineRestore,
	backup *harvesterv1.VirtualMachineBackup,
	vm *kubevirtv1.VirtualMachine,
) error {
	ownerRefs := configVMOwner(vm)
	if !vmRestore.Spec.NewVM {
		for _, secretBackup := range backup.Status.SecretBackups {
			if err := h.createOrUpdateSecret(vmRestore.Namespace, secretBackup.Name, secretBackup.Data, ownerRefs); err != nil {
				return err
			}
		}
		return nil
	}

	// Create new secret for new VM
	for _, secretBackup := range backup.Status.SecretBackups {
		newSecretName := getSecretRefName(vmRestore.Spec.Target.Name, secretBackup.Name)
		if err := h.createOrUpdateSecret(vmRestore.Namespace, newSecretName, secretBackup.Data, ownerRefs); err != nil {
			return err
		}
	}
	return nil
}

func (h *RestoreHandler) createOrUpdateSecret(namespace, name string, data map[string][]byte, ownerRefs []metav1.OwnerReference) error {
	secret, err := h.secretCache.Get(namespace, name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		logrus.Infof("create secret %s/%s", namespace, name)
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       namespace,
				OwnerReferences: ownerRefs,
			},
			Data: data,
		}
		if _, err := h.secretClient.Create(secret); err != nil {
			return err
		}
		return nil
	}

	secretCpy := secret.DeepCopy()
	secretCpy.Data = data

	if !reflect.DeepEqual(secret, secretCpy) {
		logrus.Infof("update secret %s/%s", namespace, name)
		if _, err := h.secretClient.Update(secretCpy); err != nil {
			return err
		}
	}
	return nil
}

func (h *RestoreHandler) getVMBackup(vmRestore *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineBackup, error) {
	backup, err := h.backupCache.Get(vmRestore.Spec.VirtualMachineBackupNamespace, vmRestore.Spec.VirtualMachineBackupName)
	if err != nil {
		return nil, err
	}

	if !IsBackupReady(backup) {
		return nil, fmt.Errorf("VMBackup %s is not ready", backup.Name)
	}

	if backup.Status.SourceSpec == nil {
		return nil, fmt.Errorf("empty vm backup source spec of %s", backup.Name)
	}

	return backup, nil
}

// createNewVM helps to create new target VM and set the associated owner reference
func (h *RestoreHandler) createNewVM(restore *harvesterv1.VirtualMachineRestore, backup *harvesterv1.VirtualMachineBackup) (*kubevirtv1.VirtualMachine, error) {
	vmName := restore.Spec.Target.Name
	logrus.Infof("restore target does not exist, creating a new vm %s", vmName)

	restoreID := getRestoreID(restore)
	vmCpy := backup.Status.SourceSpec.DeepCopy()

	newVMAnnotations := map[string]string{
		lastRestoreAnnotation: restoreID,
		restoreNameAnnotation: restore.Name,
	}
	if reservedMem, ok := vmCpy.ObjectMeta.Annotations[util.AnnotationReservedMemory]; ok {
		newVMAnnotations[util.AnnotationReservedMemory] = reservedMem
	}

	newVMSpecAnnotations, err := sanitizeVirtualMachineAnnotationsForRestore(restore, vmCpy.Spec.Template.ObjectMeta.Annotations)
	if err != nil {
		return nil, err
	}

	defaultRunStrategy := kubevirtv1.RunStrategyRerunOnFailure
	if backup.Status.SourceSpec.Spec.RunStrategy != nil {
		defaultRunStrategy = *backup.Status.SourceSpec.Spec.RunStrategy
	}

	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        vmName,
			Namespace:   restore.Namespace,
			Annotations: newVMAnnotations,
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			RunStrategy: &defaultRunStrategy,
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: newVMSpecAnnotations,
					Labels: map[string]string{
						util.LabelVMCreator: "harvester",
						util.LabelVMName:    vmName,
					},
				},
				Spec: sanitizeVirtualMachineForRestore(restore, vmCpy.Spec.Template.Spec),
			},
		},
	}

	newVolumes, err := getNewVolumes(&vm.Spec, restore)
	if err != nil {
		return nil, err
	}
	vm.Spec.Template.Spec.Volumes = newVolumes

	if !restore.Spec.KeepMacAddress {
		for i := range vm.Spec.Template.Spec.Domain.Devices.Interfaces {
			// remove the copied mac address of the new VM
			vm.Spec.Template.Spec.Domain.Devices.Interfaces[i].MacAddress = ""
		}
	}

	newVM, err := h.vms.Create(vm)
	if err != nil {
		return nil, err
	}

	return newVM, nil
}

func (h *RestoreHandler) updateOwnerRefAndTargetUID(vmRestore *harvesterv1.VirtualMachineRestore, vm *kubevirtv1.VirtualMachine) error {
	restoreCpy := vmRestore.DeepCopy()
	if restoreCpy.Status.TargetUID == nil {
		restoreCpy.Status.TargetUID = &vm.UID
	}

	// set vmRestore owner reference to the target VM
	restoreCpy.SetOwnerReferences(configVMOwner(vm))

	if _, err := h.restores.Update(restoreCpy); err != nil {
		return err
	}

	return nil
}

func (h *RestoreHandler) updateVolumeBackup(vmb *harvesterv1.VirtualMachineBackup, vb harvesterv1.VolumeBackup) error {
	for i, volumeBackup := range vmb.Status.VolumeBackups {
		if volumeBackup.VolumeName != vb.VolumeName {
			continue
		}

		*vmb.Status.VolumeBackups[i].Name = *vb.Name + snapRevise
		logrus.WithFields(logrus.Fields{
			"VMBackupNamespace": vmb.Namespace,
			"VMBackupName":      vmb.Name,
			"VolumeBackupName":  *vb.Name,
		}).Info("updating volume backup name")
		return nil
	}

	return fmt.Errorf("vmbackup %s/%s volumebackup %s has no matched entry %v",
		vmb.Namespace, vmb.Name, *vb.Name, vb.VolumeName)
}

// We will update VolumeBackup's name with `snap-revise` suffix, this will trigger
// VMBackup controller to create new VolumeSnapshot/VolumeSnapshotContent
// and link VMBackup to these new resources
func (h *RestoreHandler) reviseVolumeSnapshot(vb harvesterv1.VolumeBackup, vr *harvesterv1.VirtualMachineRestore) error {
	vmBackup, err := h.backupCache.Get(vr.Spec.VirtualMachineBackupNamespace, vr.Spec.VirtualMachineBackupName)
	if err != nil {
		return err
	}

	vmBackupCpy := vmBackup.DeepCopy()
	*vmBackupCpy.Status.ReadyToUse = false

	if err := h.updateVolumeBackup(vmBackupCpy, vb); err != nil {
		return err
	}

	if vmBackupCpy.Annotations == nil {
		vmBackupCpy.Annotations = make(map[string]string)
	}
	vmBackupCpy.Annotations[util.AnnotationSnapshotRevise] = strconv.FormatBool(true)

	if !reflect.DeepEqual(vmBackup.Status, vmBackupCpy.Status) {
		_, err = h.vmBackupClient.Update(vmBackupCpy)
		return err
	}

	// Make VMBackup CR be reconciled and re-created the VolumeSnapshotContent with correct content
	h.vmBackupController.Enqueue(vmBackup.Namespace, vmBackup.Name)
	return nil
}

func (h *RestoreHandler) getDataSourceSameNs(vb harvesterv1.VolumeBackup, vr *harvesterv1.VirtualMachineRestore) (string, error) {
	vs, err := h.snapshotCache.Get(vb.PersistentVolumeClaim.ObjectMeta.Namespace, *vb.Name)
	if err != nil {
		return "", err
	}

	// We don't have to check VolumeSnapshotContent if the VolumeSnapshot is from PVC
	if vs.Spec.Source.PersistentVolumeClaimName != nil {
		return *vb.Name, nil
	}

	vsc, err := h.snapshotContentCache.Get(getVolumeSnapshotContentName(vb))
	if err != nil {
		return "", err
	}

	if vsc.Spec.Source.SnapshotHandle == nil {
		return "", fmt.Errorf("vsc %s missing SnapshotHandle", vsc.Name)
	}

	_, vol, backup := decodeSnapshotID(*vsc.Spec.Source.SnapshotHandle)

	_, err = h.lhbackupCache.Get(util.LonghornSystemNamespaceName, backup)
	if err != nil {
		return "", err
	}

	sets := labels.Set{
		types.LonghornLabelBackupVolume: vol,
	}

	bvs, err := h.lhbackupVolumeCache.List(util.LonghornSystemNamespaceName, sets.AsSelector())
	if err != nil {
		return "", err
	}

	if len(bvs) == 1 {
		logrus.WithFields(logrus.Fields{
			"name":           vsc.Name,
			"snapshotHandle": *vsc.Spec.Source.SnapshotHandle,
		}).Info("VolumeSnapshotContent get correct snapshotHandle")
		return *vb.Name, nil
	}

	// If the volume is not found, it indicates that the VSC is in the wrong format.
	// Therefore, we will create a new VS/VSC and connect it to the current VMBackup.
	if len(bvs) > 1 {
		return "", fmt.Errorf("found more than 1 backupvolume for volume %s", vol)
	}

	if err := h.reviseVolumeSnapshot(vb, vr); err != nil {
		return "", err
	}

	return "", fmt.Errorf("volume backup %s needs to update VolumeSnapshot", *vb.Name)
}

func (h *RestoreHandler) getDataSourceAnotherNs(vb harvesterv1.VolumeBackup, vr *harvesterv1.VirtualMachineRestore) (string, error) {
	// create volumesnapshot if namespace is different
	vs, err := h.getOrCreateVolumeSnapshot(vr, vb)
	if err != nil {
		return "", err
	}
	return vs.Name, nil
}

// createRestoredPVC helps to create new PVC from CSI volumeSnapshot
func (h *RestoreHandler) createRestoredPVC(
	vmRestore *harvesterv1.VirtualMachineRestore,
	volumeBackup harvesterv1.VolumeBackup,
	volumeRestore harvesterv1.VolumeRestore,
) error {
	if volumeBackup.Name == nil {
		return fmt.Errorf("missing VolumeSnapshot name")
	}

	dataSourceFunc := h.getDataSourceSameNs
	if vmRestore.Namespace != volumeBackup.PersistentVolumeClaim.ObjectMeta.Namespace {
		dataSourceFunc = h.getDataSourceAnotherNs
	}

	dataSourceName, err := dataSourceFunc(volumeBackup, vmRestore)
	if err != nil {
		return err
	}

	annotations := map[string]string{}
	for key, value := range volumeBackup.PersistentVolumeClaim.ObjectMeta.Annotations {
		needSkip := false
		for _, prefix := range restoreAnnotationsToDelete {
			if strings.HasPrefix(key, prefix) {
				needSkip = true
				break
			}
		}
		if !needSkip {
			annotations[key] = value
		}
	}
	annotations[restoreNameAnnotation] = vmRestore.Name

	_, err = h.pvcClient.Create(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        volumeRestore.PersistentVolumeClaim.ObjectMeta.Name,
			Namespace:   vmRestore.Namespace,
			Labels:      volumeBackup.PersistentVolumeClaim.ObjectMeta.Labels,
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         harvesterv1.SchemeGroupVersion.String(),
					Kind:               vmRestoreKindName,
					Name:               vmRestore.Name,
					UID:                vmRestore.UID,
					Controller:         pointer.BoolPtr(true),
					BlockOwnerDeletion: pointer.BoolPtr(true),
				},
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: volumeBackup.PersistentVolumeClaim.Spec.AccessModes,
			DataSource: &corev1.TypedLocalObjectReference{
				APIGroup: pointer.StringPtr(snapshotv1.SchemeGroupVersion.Group),
				Kind:     volumeSnapshotKindName,
				Name:     dataSourceName,
			},
			Resources:        volumeBackup.PersistentVolumeClaim.Spec.Resources,
			StorageClassName: volumeBackup.PersistentVolumeClaim.Spec.StorageClassName,
			VolumeMode:       volumeBackup.PersistentVolumeClaim.Spec.VolumeMode,
		},
	})
	return err
}

func (h *RestoreHandler) getOrCreateVolumeSnapshotContent(
	vmRestore *harvesterv1.VirtualMachineRestore,
	volumeBackup harvesterv1.VolumeBackup,
) (*snapshotv1.VolumeSnapshotContent, error) {
	volumeSnapshotContentName := h.constructVolumeSnapshotContentName(vmRestore.Namespace, vmRestore.Name, *volumeBackup.Name)
	if volumeSnapshotContent, err := h.snapshotContentCache.Get(volumeSnapshotContentName); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
	} else {
		return volumeSnapshotContent, nil
	}

	lhBackup, err := h.lhbackupCache.Get(util.LonghornSystemNamespaceName, *volumeBackup.LonghornBackupName)
	if err != nil {
		return nil, err
	}

	if lhBackup.Status.VolumeName == "" {
		return nil, fmt.Errorf("lhbackup %s is not populated for volumenackup %s",
			lhBackup.Name, *volumeBackup.Name)
	}

	// Ref: https://longhorn.io/docs/1.2.3/snapshots-and-backups/csi-snapshot-support/restore-a-backup-via-csi/#restore-a-backup-that-has-no-associated-volumesnapshot
	snapshotHandle := fmt.Sprintf("bak://%s/%s", lhBackup.Status.VolumeName, lhBackup.Name)

	logrus.WithFields(logrus.Fields{
		"name": volumeSnapshotContentName,
	}).Info("creating VolumeSnapshotContent")
	return h.snapshotContents.Create(&snapshotv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      volumeSnapshotContentName,
			Namespace: vmRestore.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: harvesterv1.SchemeGroupVersion.String(),
					Kind:       vmRestoreKindName,
					Name:       vmRestore.Name,
					UID:        vmRestore.UID,
				},
			},
		},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			Driver: "driver.longhorn.io",
			// Use Retain policy to prevent LH Backup from being removed when users delete a VM.
			DeletionPolicy: snapshotv1.VolumeSnapshotContentRetain,
			Source: snapshotv1.VolumeSnapshotContentSource{
				SnapshotHandle: pointer.StringPtr(snapshotHandle),
			},
			VolumeSnapshotClassName: pointer.StringPtr(settings.VolumeSnapshotClass.Get()),
			VolumeSnapshotRef: corev1.ObjectReference{
				Name:      h.constructVolumeSnapshotName(vmRestore.Name, *volumeBackup.Name),
				Namespace: vmRestore.Namespace,
			},
		},
	})
}

func (h *RestoreHandler) getOrCreateVolumeSnapshot(
	vmRestore *harvesterv1.VirtualMachineRestore,
	volumeBackup harvesterv1.VolumeBackup,
) (*snapshotv1.VolumeSnapshot, error) {
	volumeSnapshotName := h.constructVolumeSnapshotName(vmRestore.Name, *volumeBackup.Name)
	if volumeSnapshot, err := h.snapshotCache.Get(vmRestore.Namespace, volumeSnapshotName); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
	} else {
		return volumeSnapshot, nil
	}

	volumeSnapshotContent, err := h.getOrCreateVolumeSnapshotContent(vmRestore, volumeBackup)
	if err != nil {
		return nil, err
	}

	logrus.Debugf("create VolumeSnapshot %s/%s", vmRestore.Namespace, volumeSnapshotName)
	return h.snapshots.Create(&snapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      volumeSnapshotName,
			Namespace: vmRestore.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         harvesterv1.SchemeGroupVersion.String(),
					Kind:               vmRestoreKindName,
					Name:               vmRestore.Name,
					UID:                vmRestore.UID,
					BlockOwnerDeletion: pointer.BoolPtr(true),
				},
			},
		},
		Spec: snapshotv1.VolumeSnapshotSpec{
			Source: snapshotv1.VolumeSnapshotSource{
				VolumeSnapshotContentName: pointer.StringPtr(volumeSnapshotContent.Name),
			},
			VolumeSnapshotClassName: pointer.StringPtr(settings.VolumeSnapshotClass.Get()),
		},
	})
}

func (h *RestoreHandler) deleteOldPVC(vmRestore *harvesterv1.VirtualMachineRestore, vm *kubevirtv1.VirtualMachine) error {
	if IsNewVMOrHasRetainPolicy(vmRestore) {
		logrus.Infof("skip deleting old PVC of vm %s/%s", vm.Name, vm.Namespace)
		return nil
	}

	// clean up existing pvc
	for _, volName := range vmRestore.Status.DeletedVolumes {
		vol, err := h.pvcCache.Get(vmRestore.Namespace, volName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}

		err = h.pvcClient.Delete(vol.Namespace, vol.Name, &metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *RestoreHandler) startVM(vm *kubevirtv1.VirtualMachine) error {
	runStrategy, err := vm.RunStrategy()
	if err != nil {
		return err
	}

	logrus.Infof("starting the vm %s, current state running:%v", vm.Name, runStrategy)
	switch runStrategy {
	case kubevirtv1.RunStrategyAlways:
		return nil
	case kubevirtv1.RunStrategyRerunOnFailure:
		vmi, err := h.vmiCache.Get(vm.Namespace, vm.Name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return h.restClient.Put().Namespace(vm.Namespace).Resource("virtualmachines").SubResource("start").Name(vm.Name).Do(h.context).Error()
			}
			return err
		}

		if vmi.Status.Phase == kubevirtv1.Succeeded {
			logrus.Infof("restart vmi %s in phase %v", vmi.Name, vmi.Status.Phase)
			return h.restClient.Put().Namespace(vm.Namespace).Resource("virtualmachines").SubResource("start").Name(vm.Name).Do(h.context).Error()
		}

		return nil
	case kubevirtv1.RunStrategyManual:
		if vmi, err := h.vmiCache.Get(vm.Namespace, vm.Name); err == nil {
			if vmi != nil && !vmi.IsFinal() && vmi.Status.Phase != kubevirtv1.Unknown && vmi.Status.Phase != kubevirtv1.VmPhaseUnset {
				// vm is already running
				return nil
			}
		}
		return h.restClient.Put().Namespace(vm.Namespace).Resource("virtualmachines").SubResource("start").Name(vm.Name).Do(h.context).Error()
	case kubevirtv1.RunStrategyHalted:
		return h.restClient.Put().Namespace(vm.Namespace).Resource("virtualmachines").SubResource("start").Name(vm.Name).Do(h.context).Error()
	default:
		// skip
	}
	return nil
}

func (h *RestoreHandler) updateRestoreProgress(volumeRestore *harvesterv1.VolumeRestore, isVolumesReady bool) error {
	if volumeRestore.LonghornEngineName == nil {
		return nil
	}

	lhEngine, err := h.lhengineCache.Get(util.LonghornSystemNamespaceName, *volumeRestore.LonghornEngineName)
	if err != nil {
		return err
	}

	var volumeRestoreProgress int

	defer func() {
		if volumeRestoreProgress > volumeRestore.Progress {
			volumeRestore.Progress = volumeRestoreProgress
		}
	}()

	if isVolumesReady {
		volumeRestoreProgress = restoreProgressComplete
		return nil
	}

	var numReplica int
	var replicaRestoreProgressSum int

	for _, rs := range lhEngine.Status.RestoreStatus {
		if rs == nil {
			continue
		}

		// replica's restore status could have IsRestoring == false before the restoring starts,
		// so we increase the numReplica before checking IsRestoring,
		// this makes replica with IsRestoring == false adding zero to current volume progress
		numReplica++

		if !rs.IsRestoring {
			continue
		}

		replicaRestoreProgressSum += rs.Progress
	}

	if numReplica > 0 {
		volumeRestoreProgress = replicaRestoreProgressSum / numReplica
	}

	return nil
}

// VMRestore progress stays in 90% before VM start
func (h *RestoreHandler) recifyProgressBeforeVMStart(vmRestore *harvesterv1.VirtualMachineRestore) {
	if vmRestore.Status.Progress <= restoreProgressBeforeVMStart {
		return
	}

	vmRestore.Status.Progress = restoreProgressBeforeVMStart
}

func (h *RestoreHandler) updateStatus(
	vmRestore *harvesterv1.VirtualMachineRestore,
	backup *harvesterv1.VirtualMachineBackup,
	vm *kubevirtv1.VirtualMachine,
	isVolumesReady bool,
) error {
	restoreCpy := vmRestore.DeepCopy()

	var volumeSizeSum int64
	var progressWeightSum int64

	if backup.Spec.Type == harvesterv1.Backup {
		for i := range restoreCpy.Status.VolumeRestores {
			vr := &restoreCpy.Status.VolumeRestores[i]
			if err := h.updateRestoreProgress(vr, isVolumesReady); err != nil {
				return err
			}

			volumeSizeSum += vr.VolumeSize
			progressWeightSum += int64(vr.Progress) * vr.VolumeSize
		}
	}

	if volumeSizeSum != 0 {
		restoreCpy.Status.Progress = int(progressWeightSum / volumeSizeSum)
	}

	if !isVolumesReady {
		h.recifyProgressBeforeVMStart(restoreCpy)
		updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionTrue, "", "Creating new PVCs"))
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, "", "Waiting for new PVCs"))
		if !reflect.DeepEqual(vmRestore, restoreCpy) {
			if _, err := h.restores.Update(restoreCpy); err != nil {
				return err
			}
		}
		return nil
	}

	// start VM before checking status
	if err := h.startVM(vm); err != nil {
		return h.updateStatusError(vmRestore, fmt.Errorf("failed to start vm, err:%s", err.Error()), false)
	}

	if !vm.Status.Ready {
		h.recifyProgressBeforeVMStart(restoreCpy)
		message := "Waiting for target vm to be ready"
		updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionFalse, "", message))
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, "", message))
		if !reflect.DeepEqual(vmRestore, restoreCpy) {
			if _, err := h.restores.Update(restoreCpy); err != nil {
				return err
			}
		}
		return nil
	}

	if err := h.deleteOldPVC(restoreCpy, vm); err != nil {
		return h.updateStatusError(vmRestore, fmt.Errorf("error cleaning up, err:%s", err.Error()), false)
	}

	h.recorder.Eventf(
		restoreCpy,
		corev1.EventTypeNormal,
		restoreCompleteEvent,
		"Successfully completed VirtualMachineRestore %s",
		restoreCpy.Name,
	)

	restoreCpy.Status.RestoreTime = currentTime()
	restoreCpy.Status.Complete = pointer.BoolPtr(true)
	updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionFalse, "", "Operation complete"))
	updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionTrue, "", "Operation complete"))
	if _, err := h.restores.Update(restoreCpy); err != nil {
		return err
	}
	return nil
}

func (h *RestoreHandler) updateStatusError(restore *harvesterv1.VirtualMachineRestore, err error, createEvent bool) error {
	restoreCpy := restore.DeepCopy()
	updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionFalse, "Error", err.Error()))
	updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, "Error", err.Error()))

	if !reflect.DeepEqual(restore, restoreCpy) {
		if createEvent {
			h.recorder.Eventf(
				restoreCpy,
				corev1.EventTypeWarning,
				restoreErrorEvent,
				"VirtualMachineRestore encountered error %s",
				err.Error(),
			)
		}

		if _, err2 := h.restores.Update(restoreCpy); err2 != nil {
			return err2
		}
	}

	return err
}

func (h *RestoreHandler) constructVolumeSnapshotName(restoreName, volumeBackupName string) string {
	return name.SafeConcatName("restore", restoreName, volumeBackupName)
}

func (h *RestoreHandler) constructVolumeSnapshotContentName(restoreNamespace, restoreName, volumeBackupName string) string {
	// VolumeSnapshotContent is cluster-scoped resource,
	// so adding restoreNamespace to its name to prevent conflict in different namespace with same restore name and backup
	return name.SafeConcatName("restore", restoreNamespace, restoreName, volumeBackupName)
}
