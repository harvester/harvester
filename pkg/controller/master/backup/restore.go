package backup

// NOTE: Harvester VM backup & restore is referenced from the Kubevirt's VM snapshot & restore,
// currently, we have decided to use custom VM backup and restore controllers because of the following issues:
// 1. live VM snapshot/backup should be supported, but it is prohibited on the Kubevirt side.
// 2. restore a VM backup to a new VM should be supported.
import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	kv1 "kubevirt.io/client-go/api/v1"
	cdiv1 "kubevirt.io/containerized-data-importer/pkg/apis/core/v1alpha1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer/pkg/apis/core/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlcdiv1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

var (
	restoreAnnotationsToDelete = []string{
		"pv.kubernetes.io",
		"volume.beta.kubernetes.io",
		"cdi.kubevirt.io",
	}
)

type vmRestoreTarget struct {
	handler   *RestoreHandler
	vmRestore *harvesterv1.VirtualMachineRestore
	vm        *kv1.VirtualMachine
	newVM     bool
}

const (
	restoreControllerName = "harvester-vm-restore-controller"

	volumeSnapshotKindName = "VolumeSnapshot"
	vmRestoreKindName      = "VirtualMachineRestore"

	restoreNameAnnotation = "restore.harvesterhci.io/name"
	lastRestoreAnnotation = "restore.harvesterhci.io/lastRestoreUID"

	populatedForPVCAnnotation = "cdi.kubevirt.io/storage.populatedFor"

	vmCreatorLabel = "harvesterhci.io/creator"
	vmNameLabel    = "harvesterhci.io/vmName"

	restoreErrorEvent    = "VirtualMachineRestoreError"
	restoreCompleteEvent = "VirtualMachineRestoreComplete"
)

func RegisterRestore(ctx context.Context, management *config.Management, opts config.Options) error {
	restores := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineRestore()
	backups := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
	contents := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackupContent()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	pvcs := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	dataVolumes := management.CDIFactory.Cdi().V1beta1().DataVolume()

	handler := &RestoreHandler{
		context:           ctx,
		restores:          restores,
		restoreController: restores,
		backupCache:       backups.Cache(),
		contents:          contents,
		vms:               vms,
		vmCache:           vms.Cache(),
		pvcClient:         pvcs,
		pvcCache:          pvcs.Cache(),
		dataVolumes:       dataVolumes,
		dataVolumeCache:   dataVolumes.Cache(),
		recorder:          management.NewRecorder(restoreControllerName, "", ""),
	}

	restores.OnChange(ctx, restoreControllerName, handler.RestoreOnChanged)
	pvcs.OnChange(ctx, restoreControllerName, handler.PersistentVolumeClaimOnChange)
	vms.OnChange(ctx, restoreControllerName, handler.VMOnChange)
	return nil
}

type RestoreHandler struct {
	context context.Context

	restores          ctlharvesterv1.VirtualMachineRestoreClient
	restoreController ctlharvesterv1.VirtualMachineRestoreController
	backupCache       ctlharvesterv1.VirtualMachineBackupCache
	contents          ctlharvesterv1.VirtualMachineBackupContentController
	vms               ctlkubevirtv1.VirtualMachineClient
	vmCache           ctlkubevirtv1.VirtualMachineCache
	pvcClient         ctlcorev1.PersistentVolumeClaimClient
	pvcCache          ctlcorev1.PersistentVolumeClaimCache
	dataVolumeCache   ctlcdiv1.DataVolumeCache
	dataVolumes       ctlcdiv1.DataVolumeClient

	recorder   record.EventRecorder
	restClient *rest.RESTClient
}

func restorePVCName(vmRestore *harvesterv1.VirtualMachineRestore, name string) string {
	s := fmt.Sprintf("restore-%s-%s-%s", vmRestore.Spec.VirtualMachineBackupName, vmRestore.UID, name)
	return s
}

func vmRestoreProgressing(vmRestore *harvesterv1.VirtualMachineRestore) bool {
	return vmRestore.Status == nil || vmRestore.Status.Complete == nil || !*vmRestore.Status.Complete
}

// RestoreOnChanged handles vmresotre CRD object on change state
//
// This method does follwings step by step:
//
// 1. First, update status of vmrestore to record which volume need to restore.
// 2. Next, restore PVCs for those listed in vmrestore.Status.VolumeRestores.
// 3. Then create a new VM or replace the old one.
// 4. Reconcile the spec and status of the restored VM, specifically for volume related fields.
func (h *RestoreHandler) RestoreOnChanged(key string, restore *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error) {
	if restore == nil || restore.DeletionTimestamp != nil {
		return nil, nil
	}

	if !vmRestoreProgressing(restore) {
		return nil, nil
	}

	restoreCpy := restore.DeepCopy()

	if restoreCpy.Status == nil {
		restoreCpy.Status = &harvesterv1.VirtualMachineRestoreStatus{
			Complete: pointer.BoolPtr(false),
		}
	}

	target, err := h.getTarget(restoreCpy)
	if err != nil {
		return nil, h.doUpdateError(restore, restoreCpy, err, true)
	}

	if target == nil {
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, fmt.Sprintf("failed to find restore target %s", restoreCpy.Spec.Target.Name)))
		return nil, h.doUpdate(restore, restoreCpy)
	}

	content, err := h.getBackupContent(restoreCpy, target.UID(), restoreCpy.Spec.NewVM)
	if err != nil {
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, err.Error()))
		return nil, h.doUpdate(restore, restoreCpy)
	}

	if content == nil {
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, fmt.Sprintf("failed to find vm backup resource %s", restoreCpy.Spec.VirtualMachineBackupName)))
		return nil, h.doUpdate(restore, restoreCpy)
	}

	if len(restoreCpy.OwnerReferences) == 0 && !target.newVM {
		restoreCpy.SetOwnerReferences(configVMOwner(target.vm))
		updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionTrue, "Initializing VirtualMachineRestore"))
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, "Initializing VirtualMachineRestore"))
		return nil, h.doUpdate(restore, restoreCpy)
	}

	var updated bool
	updated, err = h.reconcileVolumeRestores(restoreCpy, content)
	if err != nil {
		return nil, h.doUpdateError(restore, restoreCpy, fmt.Errorf("error reconciling VolumeRestores"), true)
	}

	// create new vm target after reconcile the restore volumes
	if target.vm == nil && restoreCpy.Spec.NewVM {
		target.vm, err = h.createNewVM(restoreCpy, content)
		if err != nil {
			return nil, h.doUpdateError(restore, restoreCpy, err, true)
		}
	}

	if !updated {
		var ready bool
		ready, err = target.Ready()
		if err != nil {
			return nil, h.doUpdateError(restore, restoreCpy, fmt.Errorf("error checking target ready, err:%s", err.Error()), false)
		}

		// reconcile the vm if the restore is not ready
		if ready {
			updated, err = target.Reconcile()
			if err != nil {
				return nil, h.doUpdateError(restore, restoreCpy, fmt.Errorf("error reconciling target, err:%s", err.Error()), false)
			}

			if !updated {
				if err = target.Cleanup(); err != nil {
					return nil, h.doUpdateError(restore, restoreCpy, fmt.Errorf("error cleaning up, err:%s", err.Error()), false)
				}

				if err = target.RestartVM(); err != nil {
					return nil, fmt.Errorf("failed to restart vm, err:%s", err.Error())
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
				updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionFalse, "Operation complete"))
				updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionTrue, "Operation complete"))
			} else {
				updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionTrue, "Updating target spec"))
				updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, "Waiting for target update"))
			}
		} else {
			reason := "Waiting for target vm to be ready"
			updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionFalse, reason))
			updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, reason))
			// try again in 5 secs
			h.enqueueAfter(restore, restoreCpy, 5*time.Second)
			return nil, nil
		}
	} else {
		// The PVCs and VirtualMachineRestore are still under update/bound process.
		vmCpy := target.vm
		if vmCpy.Annotations == nil {
			vmCpy.Annotations = make(map[string]string)
		}

		if vmCpy.Annotations[restoreNameAnnotation] != restoreCpy.Name {
			vmCpy.Annotations[restoreNameAnnotation] = restoreCpy.Name
			if _, err = h.vms.Update(vmCpy); err != nil {
				return nil, err
			}
		}
		updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionTrue, "Creating new PVCs"))
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, "Waiting for new PVCs"))
	}
	return nil, h.doUpdate(restore, restoreCpy)
}

func (h *RestoreHandler) getTarget(vmRestore *harvesterv1.VirtualMachineRestore) (*vmRestoreTarget, error) {
	isNewVM := false
	switch vmRestore.Spec.Target.Kind {
	case kv1.VirtualMachineGroupVersionKind.Kind:
		vm, err := h.vmCache.Get(vmRestore.Namespace, vmRestore.Spec.Target.Name)
		if err != nil {
			if !apierrors.IsNotFound(err) && !vmRestore.Spec.NewVM {
				return nil, err
			}
			isNewVM = true
		}

		return &vmRestoreTarget{
			handler:   h,
			vmRestore: vmRestore,
			vm:        vm,
			newVM:     isNewVM,
		}, nil
	}

	return nil, fmt.Errorf("unknown source %+v", vmRestore.Spec.Target)
}

func (h *RestoreHandler) doUpdateError(original, updated *harvesterv1.VirtualMachineRestore, err error, createEvent bool) error {
	if createEvent {
		h.recorder.Eventf(
			updated,
			corev1.EventTypeWarning,
			restoreErrorEvent,
			"VirtualMachineRestore encountered error %s",
			err.Error(),
		)
	}
	updateRestoreCondition(updated, newProgressingCondition(corev1.ConditionFalse, err.Error()))
	updateRestoreCondition(updated, newReadyCondition(corev1.ConditionFalse, err.Error()))
	if err2 := h.doUpdate(original, updated); err2 != nil {
		return err2
	}

	return err
}

func updateRestoreCondition(r *harvesterv1.VirtualMachineRestore, c harvesterv1.Condition) {
	r.Status.Conditions = updateCondition(r.Status.Conditions, c, true)
}

func (h *RestoreHandler) doUpdate(original, updated *harvesterv1.VirtualMachineRestore) error {
	if !reflect.DeepEqual(original, updated) {
		if _, err := h.restores.Update(updated); err != nil {
			return err
		}
	}

	return nil
}

func (h *RestoreHandler) enqueueAfter(original, updated *harvesterv1.VirtualMachineRestore, t time.Duration) {
	if !reflect.DeepEqual(original, updated) {
		h.restoreController.EnqueueAfter(updated.Namespace, updated.Name, t)
	}
}

// reconcileVolumeRestores returns a flag indicating whether the VirtualMachineRestore
// and its related PersistentVolumeClaims are still under the update process.
//
// This method does two things and may be called multiple times for a single VM restore process.
//
// - Reconciling the status of VirtualMachineRestore, and
// - Creating PersistentVolumeClaim for each VolumeRestore if not exists.
// - Creating DataVolume for each VolumeRestore if it is not an embedded one inside the VM.
func (h *RestoreHandler) reconcileVolumeRestores(vmRestore *harvesterv1.VirtualMachineRestore,
	content *harvesterv1.VirtualMachineBackupContent) (bool, error) {
	restores := make([]harvesterv1.VolumeRestore, 0, len(content.Spec.VolumeBackups))

	// 1. collect all VolumeRestores that contain PVCs to restore
	for _, vb := range content.Spec.VolumeBackups {
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
				return false, fmt.Errorf("VolumeSnapshotName missing %+v", vb)
			}

			vr := harvesterv1.VolumeRestore{
				VolumeName: vb.VolumeName,
				PersistentVolumeClaim: harvesterv1.PersistentVolumeClaimSpec{
					Name:      restorePVCName(vmRestore, vb.VolumeName),
					Namespace: vb.PersistentVolumeClaim.Namespace,
					Spec:      vb.PersistentVolumeClaim.Spec,
				},
				VolumeBackupName: *vb.Name,
			}
			restores = append(restores, vr)
		}
	}

	if !reflect.DeepEqual(vmRestore.Status.VolumeRestores, restores) {
		// We update the volume restore in status at the very first call of this method.
		// Just return it now and wait for the next call to restore missing PVCs.
		if len(vmRestore.Status.VolumeRestores) > 0 {
			logrus.Warningf("VMRestore in strange state, obj:%v", vmRestore)
		}

		vmRestore.Status.VolumeRestores = restores
		return true, nil
	}

	volumeNamesInDVT := createSetOfVolumesNameInDataVolumeTemplates(content)

	// 2. Restore PVCs. Create a new one if not exists.
	createdPVC := false
	waitingPVC := false
	for i, restore := range restores {
		pvc, err := h.pvcCache.Get(restore.PersistentVolumeClaim.Namespace, restore.PersistentVolumeClaim.Name)
		if apierrors.IsNotFound(err) {
			backup := content.Spec.VolumeBackups[i]
			var err error
			if volumeNamesInDVT.Has(restore.VolumeName) {
				logrus.Infof("restore pvc from embedded dataVolume `%s/%s`", vmRestore.Namespace, restore.VolumeBackupName)
				err = h.createRestorePVC(vmRestore, backup, restore)
			} else {
				logrus.Infof("restore pvc from manually attached dataVolume `%s/%s`", vmRestore.Namespace, restore.VolumeBackupName)
				err = h.createRestoreDataVolume(vmRestore, backup, restore)
			}
			if err != nil {
				return false, err
			}
			createdPVC = true
			continue
		}
		if err != nil {
			return false, err
		}

		if pvc.Status.Phase == corev1.ClaimPending {
			waitingPVC = true
		} else if pvc.Status.Phase != corev1.ClaimBound {
			return false, fmt.Errorf("PVC %s/%s in status %q", pvc.Namespace, pvc.Name, pvc.Status.Phase)
		}
	}

	return createdPVC || waitingPVC, nil
}

func (h *RestoreHandler) getBackupContent(vmRestore *harvesterv1.VirtualMachineRestore, targetUID types.UID, newVM bool) (*harvesterv1.VirtualMachineBackupContent, error) {
	backup, err := h.backupCache.Get(vmRestore.Namespace, vmRestore.Spec.VirtualMachineBackupName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	if !vmBackupReady(backup) {
		return nil, fmt.Errorf("VMBackup %s is not ready", backup.Name)
	}

	if !newVM && (vmRestore.Status.TargetUID == nil || *vmRestore.Status.TargetUID != targetUID) &&
		(backup.Status.SourceUID == nil || *backup.Status.SourceUID != targetUID) {
		return nil, fmt.Errorf("neither a new VM or VMBackup source and restore target differ")
	}

	if backup.Status.VirtualMachineBackupContentName == nil {
		return nil, fmt.Errorf("no snapshot content name in %s", backup.Name)
	}

	content, err := h.contents.Get(backup.Namespace, getVMBackupContentName(backup), metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if !vmBackupContentReady(content) {
		return nil, fmt.Errorf("VMBackupContent %s not ready", content.Name)
	}

	return content, nil
}

func configVMOwner(vm *kv1.VirtualMachine) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         kv1.SchemeGroupVersion.String(),
			Kind:               kv1.VirtualMachineGroupVersionKind.Kind,
			Name:               vm.Name,
			UID:                vm.UID,
			Controller:         pointer.BoolPtr(true),
			BlockOwnerDeletion: pointer.BoolPtr(true),
		},
	}
}

func (t *vmRestoreTarget) Cleanup() error {
	if t.vmRestore.Spec.NewVM || t.vmRestore.Spec.DeletionPolicy == harvesterv1.VirtualMachineRestoreRetain {
		logrus.Infof("skip clean up carryover resources of vm %s/%s", t.vm.Name, t.vm.Namespace)
		return nil
	}

	for _, dvName := range t.vmRestore.Status.DeletedDataVolumes {
		dv, err := t.handler.dataVolumeCache.Get(t.vmRestore.Namespace, dvName)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}

		if dv != nil {
			err = t.handler.dataVolumes.Delete(dv.Namespace, dv.Name, &metav1.DeleteOptions{})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (t *vmRestoreTarget) UID() types.UID {
	if t.newVM {
		return ""
	}
	return t.vm.UID
}

func (t *vmRestoreTarget) Ready() (bool, error) {

	_, err := t.vm.RunStrategy()
	if err != nil {
		return false, err
	}

	_, err = t.handler.vmCache.Get(t.vm.Namespace, t.vm.Name)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (t *vmRestoreTarget) RestartVM() error {
	logrus.Infof("restarting the vm %s, current state running:%v", t.vm.Name, *t.vm.Spec.Running)
	var running = true
	if t.vm.Spec.Running == nil || t.vm.Spec.Running == &running {
		return t.handler.restClient.Put().Namespace(t.vm.Namespace).Resource("virtualmachines").SubResource("restart").Name(t.vm.Name).Do(t.handler.context).Error()
	}
	return nil
}

// ReconcileVMTemplate reconciles DataVolumeTemplates and Template.Spec.Volumes of the restored VM.
//
// This method does many tasks:
//
// - Links newly created PVCs to DataVolumeTemplates , as well as sets some ownership annotations back to PVC.
// - Links to newly updated DataVolumes to Template.Spec.Volumes for those volumes with DataVolume kind.
// - Links to newly created pvc to Template.Spec.Volumes for those volumes not DataVolume kind.
func (h *RestoreHandler) ReconcileVMTemplate(vm *kv1.VirtualMachineSpec,
	vmRestore *harvesterv1.VirtualMachineRestore) ([]kv1.DataVolumeTemplateSpec, []kv1.Volume, bool, error) {
	var newTemplates = make([]kv1.DataVolumeTemplateSpec, len(vm.DataVolumeTemplates))
	var newVolumes = make([]kv1.Volume, len(vm.Template.Spec.Volumes))
	updatedStatus := false

	copy(newTemplates, vm.DataVolumeTemplates)
	copy(newVolumes, vm.Template.Spec.Volumes)

	for j, vol := range vm.Template.Spec.Volumes {

		if vol.DataVolume != nil || vol.PersistentVolumeClaim != nil {

			for _, vr := range vmRestore.Status.VolumeRestores {
				if vr.VolumeName != vol.Name {
					continue
				}

				// 1. vm volume type is dataVolume
				if vol.DataVolume != nil {
					pvc, err := h.pvcCache.Get(vr.PersistentVolumeClaim.Namespace, vr.PersistentVolumeClaim.Name)
					if err != nil {
						return nil, nil, false, err
					}

					templateIndex := -1
					for i, dvt := range vm.DataVolumeTemplates {
						if vol.DataVolume.Name == dvt.Name {
							templateIndex = i
							break
						}
					}

					if templateIndex >= 0 {
						// Found datavolume inside VM's DataVolumeTemplates, this
						// datavolume is embedded in the VM exclusively.
						dvtCopy := vm.DataVolumeTemplates[templateIndex].DeepCopy()

						// only need to update the dataVolumeTemplate if the restore name is different
						if dvtCopy.Name != vr.PersistentVolumeClaim.Name {

							dvtCopy.Name = vr.PersistentVolumeClaim.Name
							dvtCopy.Spec.Source = cdiv1.DataVolumeSource{
								Blank: &cdiv1.DataVolumeBlankImage{},
							}

							if _, err = h.takeOwnershipOfPVCIfNeeded(pvc, dvtCopy.Name); err != nil {
								return newTemplates, newVolumes, updatedStatus, err
							}

							if dvtCopy.Annotations == nil {
								dvtCopy.Annotations = make(map[string]string, 1)
							}
							dvtCopy.Annotations[restoreNameAnnotation] = vmRestore.Name
							newTemplates[templateIndex] = *dvtCopy

							nv := vol.DeepCopy()
							nv.DataVolume.Name = vr.PersistentVolumeClaim.Name
							newVolumes[j] = *nv
							updatedStatus = true
						}
					} else {
						// This data volume is not in the VM's DataVolumeTemplates,
						// indicating that it was created outside the VM, then
						// manually attached to the VM.  The dadtavolume should
						// have already got prepared in `reconcileVolumeRestores`
						// so here we only need to retrieve it from cache.
						dv, err := h.dataVolumeCache.Get(vr.PersistentVolumeClaim.Namespace, vr.PersistentVolumeClaim.Name)
						if err != nil {
							return newTemplates, newVolumes, updatedStatus, err
						}

						nv := kv1.Volume{
							Name: vol.Name,
							VolumeSource: kv1.VolumeSource{
								DataVolume: &kv1.DataVolumeSource{
									Name: dv.Name,
								},
							},
						}
						newVolumes[j] = nv
					}
				} else {
					// 2. vm volume is pvc or others
					nv := vol.DeepCopy()
					nv.PersistentVolumeClaim.ClaimName = vr.PersistentVolumeClaim.Name
					newVolumes[j] = *nv
				}
			}
		}
	}
	return newTemplates, newVolumes, updatedStatus, nil
}

// Reconcile reconiles the restore target (a.k.a. the restored VM).
// Returns a flag indicating whether the VM is still under the update process.
func (t *vmRestoreTarget) Reconcile() (bool, error) {
	logrus.Debugf("VM ready, reconciling target VM %s", t.vmRestore.Name)

	restoreID := getRestoreID(t.vmRestore)
	if lastRestoreID, ok := t.vm.Annotations[lastRestoreAnnotation]; ok && lastRestoreID == restoreID {
		return false, nil
	}

	content, err := t.handler.getBackupContent(t.vmRestore, t.UID(), false)
	if err != nil {
		return false, err
	}

	source := content.Spec.Source
	if reflect.DeepEqual(source.VirtualMachineSpec, kv1.VirtualMachineSpec{}) {
		return false, fmt.Errorf("unexpected snapshot source")
	}

	newTemplates, newVolumes, updatedStatus, err := t.handler.ReconcileVMTemplate(source.VirtualMachineSpec, t.vmRestore)
	if err != nil {
		return false, err
	}

	var deletedDataVolumes []string
	if updatedStatus {
		// find DataVolumes that will no longer exist
		for _, cdv := range t.vm.Spec.DataVolumeTemplates {
			found := false
			for _, ndv := range newTemplates {
				if cdv.Name == ndv.Name {
					found = true
					break
				}
			}
			if !found {
				deletedDataVolumes = append(deletedDataVolumes, cdv.Name)
			}
		}
		t.vmRestore.Status.DeletedDataVolumes = deletedDataVolumes
	}

	vmCpy := t.vm.DeepCopy()
	vmCpy.Spec = *source.VirtualMachineSpec
	vmCpy.Spec.DataVolumeTemplates = newTemplates
	vmCpy.Spec.Template.Spec.Volumes = newVolumes
	if vmCpy.Annotations == nil {
		vmCpy.Annotations = make(map[string]string)
	}
	vmCpy.Annotations[lastRestoreAnnotation] = restoreID
	vmCpy.Annotations[restoreNameAnnotation] = t.vmRestore.Name

	if _, err = t.handler.vms.Update(vmCpy); err != nil {
		return false, err
	}

	return true, nil
}

func (h *RestoreHandler) createNewVM(restore *harvesterv1.VirtualMachineRestore, content *harvesterv1.VirtualMachineBackupContent) (*kv1.VirtualMachine, error) {
	vmName := restore.Spec.Target.Name
	logrus.Infof("restore target does not exist, creating a new vm %s", vmName)

	restoreID := getRestoreID(restore)
	vmCpy := content.Spec.Source.DeepCopy()
	vm := &kv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmName,
			Namespace: restore.Namespace,
			Annotations: map[string]string{
				lastRestoreAnnotation: restoreID,
				restoreNameAnnotation: restore.Name,
			},
		},
		Spec: kv1.VirtualMachineSpec{
			Running: pointer.BoolPtr(true),
			Template: &kv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: vmCpy.VirtualMachineSpec.Template.ObjectMeta.Annotations,
					Labels: map[string]string{
						vmCreatorLabel: "harvester",
						vmNameLabel:    vmName,
					},
				},
				Spec: vmCpy.VirtualMachineSpec.Template.Spec,
			},
			DataVolumeTemplates: vmCpy.VirtualMachineSpec.DataVolumeTemplates,
		},
	}

	newTemplates, newVolumes, _, err := h.ReconcileVMTemplate(&vm.Spec, restore)
	if err != nil {
		return nil, err
	}
	vm.Spec.DataVolumeTemplates = newTemplates
	vm.Spec.Template.Spec.Volumes = newVolumes

	for i := range vm.Spec.Template.Spec.Domain.Devices.Interfaces {
		// remove the copied mac address of the new VM
		vm.Spec.Template.Spec.Domain.Devices.Interfaces[i].MacAddress = ""
	}

	newVM, err := h.vms.Create(vm)
	if err != nil {
		return nil, err
	}

	if restore.Status.TargetUID == nil {
		restore.Status.TargetUID = &newVM.UID
	}
	vm.SetOwnerReferences(configVMOwner(vm))
	if _, err = h.restores.Update(restore); err != nil {
		return nil, err
	}

	return vm, nil
}

func (h *RestoreHandler) PersistentVolumeClaimOnChange(key string, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	if pvc == nil || pvc.DeletionTimestamp != nil {
		return nil, nil
	}

	restoreName, ok := pvc.Annotations[restoreNameAnnotation]
	if !ok {
		return nil, nil
	}

	logrus.Debugf("handling PVC updating %s/%s", pvc.Namespace, pvc.Name)
	h.restoreController.EnqueueAfter(pvc.Namespace, restoreName, 5*time.Second)
	return nil, nil
}

func (h *RestoreHandler) VMOnChange(key string, vm *kv1.VirtualMachine) (*kv1.VirtualMachine, error) {
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

func getRestoreID(vmRestore *harvesterv1.VirtualMachineRestore) string {
	return fmt.Sprintf("%s-%s", vmRestore.Name, vmRestore.UID)
}

func (h *RestoreHandler) createRestoreDataVolume(vmRestore *harvesterv1.VirtualMachineRestore,
	volumeBackup harvesterv1.VolumeBackup, volumeRestore harvesterv1.VolumeRestore) error {
	_, err := h.dataVolumeCache.Get(vmRestore.Namespace, volumeRestore.PersistentVolumeClaim.Name)
	if apierrors.IsNotFound(err) {
		annotations := make(map[string]string, 1)
		annotations[restoreNameAnnotation] = vmRestore.Name
		sourcePvcSpecCopy := volumeBackup.PersistentVolumeClaim.Spec.DeepCopy()
		// Cleanup the original volume name to prevent binding to old pv
		sourcePvcSpecCopy.VolumeName = ""
		_, err = h.dataVolumes.Create(&cdiv1beta1.DataVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name:        volumeRestore.PersistentVolumeClaim.Name,
				Namespace:   vmRestore.Namespace,
				Annotations: annotations,
			},
			Spec: cdiv1beta1.DataVolumeSpec{
				Source: cdiv1beta1.DataVolumeSource{
					Blank: &cdiv1beta1.DataVolumeBlankImage{},
				},
				PVC: sourcePvcSpecCopy,
			},
		})
	}
	return err
}

func (h *RestoreHandler) createRestorePVC(vmRestore *harvesterv1.VirtualMachineRestore,
	volumeBackup harvesterv1.VolumeBackup, volumeRestore harvesterv1.VolumeRestore) error {
	sourcePVC := volumeBackup.PersistentVolumeClaim
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        volumeRestore.PersistentVolumeClaim.Name,
			Namespace:   vmRestore.Namespace,
			Labels:      sourcePVC.Labels,
			Annotations: sourcePVC.Annotations,
		},
		Spec: *sourcePVC.Spec.DeepCopy(),
	}

	pvc.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion:         harvesterv1.SchemeGroupVersion.String(),
			Kind:               vmRestoreKindName,
			Name:               vmRestore.Name,
			UID:                vmRestore.UID,
			Controller:         pointer.BoolPtr(true),
			BlockOwnerDeletion: pointer.BoolPtr(true),
		},
	})
	if volumeBackup.Name == nil {
		return fmt.Errorf("missing VolumeSnapshot name")
	}

	if pvc.Annotations == nil {
		pvc.Annotations = make(map[string]string)
	}

	for _, prefix := range restoreAnnotationsToDelete {
		for anno := range pvc.Annotations {
			if strings.HasPrefix(anno, prefix) {
				delete(pvc.Annotations, anno)
			}
		}
	}
	pvc.Annotations[restoreNameAnnotation] = vmRestore.Name
	pvc.Spec.DataSource = &corev1.TypedLocalObjectReference{
		APIGroup: pointer.StringPtr(snapshotv1.SchemeGroupVersion.Group),
		Kind:     volumeSnapshotKindName,
		Name:     *volumeBackup.Name,
	}
	pvc.Spec.VolumeName = ""

	_, err := h.pvcClient.Create(pvc)
	return err
}

// takeOwnershipOfPVCIfNeeded let dataVolume takes the ownership of given pvc
// by addition `cdi.kubevirt.io/storage.populatedFor` annotation on it.
func (h *RestoreHandler) takeOwnershipOfPVCIfNeeded(pvc *corev1.PersistentVolumeClaim, dataVolumeName string) (*corev1.PersistentVolumeClaim, error) {
	updatePVC := pvc.DeepCopy()
	if updatePVC.Annotations[populatedForPVCAnnotation] != dataVolumeName {
		if updatePVC.Annotations == nil {
			updatePVC.Annotations = make(map[string]string)
		}
		updatePVC.Annotations[populatedForPVCAnnotation] = dataVolumeName
		// dataVolume will take ownership
		updatePVC.OwnerReferences = nil
		return h.pvcClient.Update(updatePVC)
	}
	return pvc, nil
}

func createSetOfVolumesNameInDataVolumeTemplates(content *harvesterv1.VirtualMachineBackupContent) sets.String {
	volumes := content.Spec.Source.VirtualMachineSpec.Template.Spec.Volumes
	dvts := content.Spec.Source.VirtualMachineSpec.DataVolumeTemplates

	volNameMap := make(map[string]string, len(volumes))
	for _, vol := range volumes {
		if vol.DataVolume != nil {
			volNameMap[vol.DataVolume.Name] = vol.Name
		}
	}
	volumeNamesInDVT := sets.String{}
	for _, dvt := range dvts {
		volName, ok := volNameMap[dvt.Name]
		if ok {
			volumeNamesInDVT.Insert(volName)
		}
	}
	return volumeNamesInDVT
}
