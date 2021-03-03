package backup

// NOTE: Harvester VM backup & restore is referenced from the Kubevirt's VM snapshot & restore,
// currently, we have decided to use custom VM backup and restore controllers because of the following issues:
// 1. live VM snapshot/backup should be supported, but it is prohibited on the Kubevirt side.
// 2. restore a VM backup to a new VM should be supported.
import (
	"context"
	"fmt"
	"reflect"
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	kubevirtv1 "kubevirt.io/client-go/api/v1"
	cdiv1 "kubevirt.io/containerized-data-importer/pkg/apis/core/v1alpha1"

	harvesterapiv1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	ctlcdiv1 "github.com/rancher/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	ctlharvesterv1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	ctlkubevirtv1 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

var (
	snapshotAPIGroup = "snapshot.storage.k8s.io"
	vmResource       = "virtualmachines"
)

type vmRestoreTarget struct {
	handler   *RestoreHandler
	vmRestore *harvesterapiv1.VirtualMachineRestore
	vm        *kubevirtv1.VirtualMachine
}

const (
	restoreControllerName = "vm-restore-controller"

	restoreNameAnnotation = "restore.harvester.io/name"
	lastRestoreAnnotation = "restore.harvester.io/lastRestoreUID"

	vmCreatorLabel = "harvester.cattle.io/creator"
	vmNameLabel    = "harvester.cattle.io/vmName"

	restoreErrorEvent    = "VirtualMachineRestoreError"
	restoreCompleteEvent = "VirtualMachineRestoreComplete"
)

func RegisterRestore(ctx context.Context, management *config.Management, opts config.Options) error {
	restores := management.HarvesterFactory.Harvester().V1alpha1().VirtualMachineRestore()
	backups := management.HarvesterFactory.Harvester().V1alpha1().VirtualMachineBackup()
	contents := management.HarvesterFactory.Harvester().V1alpha1().VirtualMachineBackupContent()
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
	dataVolumeCache   ctlcdiv1.DataVolumeCache
	dataVolumes       ctlcdiv1.DataVolumeClient

	recorder   record.EventRecorder
	restClient *rest.RESTClient
}

func restorePVCName(vmRestore *harvesterapiv1.VirtualMachineRestore, name string) string {
	s := fmt.Sprintf("restore-%s-%s-%s", vmRestore.Spec.VirtualMachineBackupName, vmRestore.UID, name)
	return s
}

func vmRestoreProgressing(vmRestore *harvesterapiv1.VirtualMachineRestore) bool {
	return vmRestore.Status == nil || vmRestore.Status.Complete == nil || !*vmRestore.Status.Complete
}

// RestoreOnChanged handles vmresotre CRD object on change state
func (h *RestoreHandler) RestoreOnChanged(key string, restore *harvesterapiv1.VirtualMachineRestore) (*harvesterapiv1.VirtualMachineRestore, error) {
	if restore == nil || restore.DeletionTimestamp != nil {
		return nil, nil
	}

	if !vmRestoreProgressing(restore) {
		return nil, nil
	}

	restoreCpy := restore.DeepCopy()

	if restoreCpy.Status == nil {
		restoreCpy.Status = &harvesterapiv1.VirtualMachineRestoreStatus{}
	}

	restoreCpy.Status.Complete = pointer.BoolPtr(false)
	restoreCpy.Status.RestoreTime = nil

	targetVM, err := h.getTarget(restoreCpy)
	if err != nil {
		return nil, h.doUpdateError(restore, restoreCpy, err)
	}

	if targetVM == nil {
		logrus.Errorf("failed to find restore target %s", restoreCpy.Spec.Target.Name)
		return nil, nil
	}

	content, err := h.getBackupContent(restore, targetVM.UID(), restore.Spec.NewVM)
	if err != nil {
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, err.Error()))
		return nil, h.doUpdate(restore, restoreCpy)
	}

	if content == nil {
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, fmt.Sprintf("failed to find vm backup resource %s", restoreCpy.Spec.VirtualMachineBackupName)))
		return nil, h.doUpdate(restore, restoreCpy)
	}

	if len(restoreCpy.OwnerReferences) == 0 {
		targetVM.Own(restoreCpy)
		updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionTrue, "Initializing VirtualMachineRestore"))
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, "Initializing VirtualMachineRestore"))
		return nil, h.doUpdate(restore, restoreCpy)
	}

	var updated bool
	updated, err = h.reconcileVolumeRestores(restoreCpy, content)
	if err != nil {
		logrus.Error("error reconciling VolumeRestores")
		return nil, h.doUpdateError(restore, restoreCpy, err)
	}

	if !updated {
		var ready bool
		ready, err = targetVM.Ready()
		if err != nil {
			logrus.Errorf("error checking target ready, err:%s", err.Error())
			return nil, h.doUpdateError(restore, restoreCpy, err)
		}

		// reconcile the vm if the restore is not ready
		if ready {
			updated, err = targetVM.Reconcile()
			if err != nil {
				logrus.Errorf("error reconciling target, err:%s", err.Error())
				return nil, h.doUpdateError(restore, restoreCpy, err)
			}

			if !updated {
				if err = targetVM.Cleanup(); err != nil {
					logrus.Errorf("error cleaning up, err:%s", err.Error())
					return nil, h.doUpdateError(restore, restoreCpy, err)
				}

				if err = targetVM.RestartVM(); err != nil {
					logrus.Errorf("failed to restart vm, err:%s", err.Error())
					return nil, err
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
		updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionTrue, "Creating new PVCs"))
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, "Waiting for new PVCs"))
	}

	return nil, h.doUpdate(restore, restoreCpy)
}

func (h *RestoreHandler) getTarget(vmRestore *harvesterapiv1.VirtualMachineRestore) (*vmRestoreTarget, error) {
	switch vmRestore.Spec.Target.Kind {
	case vmKindName:
		vm, err := h.vmCache.Get(vmRestore.Namespace, vmRestore.Spec.Target.Name)
		if err != nil {
			if vmRestore.Spec.NewVM && apierrors.IsNotFound(err) {
				return h.createNewVM(vmRestore)
			} else if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}

		return &vmRestoreTarget{
			handler:   h,
			vmRestore: vmRestore,
			vm:        vm,
		}, nil
	}

	return nil, fmt.Errorf("unknown source %+v", vmRestore.Spec.Target)
}

func (h *RestoreHandler) doUpdateError(original, updated *harvesterapiv1.VirtualMachineRestore, err error) error {
	h.recorder.Eventf(
		updated,
		corev1.EventTypeWarning,
		restoreErrorEvent,
		"VirtualMachineRestore encountered error %s",
		err.Error(),
	)

	updateRestoreCondition(updated, newProgressingCondition(corev1.ConditionFalse, err.Error()))
	updateRestoreCondition(updated, newReadyCondition(corev1.ConditionFalse, err.Error()))
	if err2 := h.doUpdate(original, updated); err2 != nil {
		return err2
	}

	return err
}

func updateRestoreCondition(r *harvesterapiv1.VirtualMachineRestore, c harvesterapiv1.Condition) {
	r.Status.Conditions = updateCondition(r.Status.Conditions, c, true)
}

func (h *RestoreHandler) doUpdate(original, updated *harvesterapiv1.VirtualMachineRestore) error {
	if !reflect.DeepEqual(original, updated) {
		if _, err := h.restores.Update(updated); err != nil {
			return err
		}
	}

	return nil
}

func (h *RestoreHandler) enqueueAfter(original, updated *harvesterapiv1.VirtualMachineRestore, t time.Duration) {
	if !reflect.DeepEqual(original, updated) {
		h.restoreController.EnqueueAfter(updated.Namespace, updated.Name, t)
	}
}

func (h *RestoreHandler) reconcileVolumeRestores(vmRestore *harvesterapiv1.VirtualMachineRestore, content *harvesterapiv1.VirtualMachineBackupContent) (bool, error) {
	restores := make([]harvesterapiv1.VolumeRestore, 0, len(content.Spec.VolumeBackups))
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

			vr := harvesterapiv1.VolumeRestore{
				VolumeName:                vb.VolumeName,
				PersistentVolumeClaimName: restorePVCName(vmRestore, vb.VolumeName),
				VolumeBackupName:          *vb.Name,
			}
			restores = append(restores, vr)
		}
	}

	if !reflect.DeepEqual(vmRestore.Status.VolumeRestores, restores) {
		if len(vmRestore.Status.VolumeRestores) > 0 {
			logrus.Warningf("VMRestore in strange state, obj:%v", vmRestore)
		}

		vmRestore.Status.VolumeRestores = restores
		return true, nil
	}

	return false, nil
}

func (h *RestoreHandler) getBackupContent(vmRestore *harvesterapiv1.VirtualMachineRestore, targetUID types.UID, newVM bool) (*harvesterapiv1.VirtualMachineBackupContent, error) {
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

	if (backup.Status.SourceUID == nil || *backup.Status.SourceUID != targetUID) && !newVM &&
		(vmRestore.Status == nil || vmRestore.Status.TargetUID == nil || *vmRestore.Status.TargetUID != targetUID) {
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

func (t *vmRestoreTarget) Own(obj metav1.Object) {
	obj.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion:         snapshotv1.SchemeGroupVersion.String(),
			Kind:               vmKindName,
			Name:               t.vm.Name,
			UID:                t.vm.UID,
			Controller:         pointer.BoolPtr(true),
			BlockOwnerDeletion: pointer.BoolPtr(true),
		},
	})
}

func (t *vmRestoreTarget) Cleanup() error {
	if t.vmRestore.Spec.NewVM || t.vmRestore.Spec.DeletionPolicy == harvesterapiv1.VirtualMachineRestoreRetain {
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
	logrus.Infof("restarting the vm %s, current state running: %v", t.vm.Name, *t.vm.Spec.Running)
	var running = true
	if t.vm.Spec.Running == nil || t.vm.Spec.Running == &running {
		return t.handler.restClient.Put().Namespace(t.vm.Namespace).Resource(vmResource).SubResource("restart").Name(t.vm.Name).Do(t.handler.context).Error()
	}
	return nil
}

func ReconcileVMTemplate(vm *kubevirtv1.VirtualMachineSpec, vmRestore *harvesterapiv1.VirtualMachineRestore) ([]kubevirtv1.DataVolumeTemplateSpec, []kubevirtv1.Volume, bool) {
	var newTemplates = make([]kubevirtv1.DataVolumeTemplateSpec, len(vm.DataVolumeTemplates))
	var newVolumes = make([]kubevirtv1.Volume, len(vm.Template.Spec.Volumes))
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
					templateIndex := -1
					for i, dvt := range vm.DataVolumeTemplates {
						if vol.DataVolume.Name == dvt.Name {
							templateIndex = i
							break
						}
					}

					if templateIndex >= 0 {
						dvtCopy := vm.DataVolumeTemplates[templateIndex].DeepCopy()

						// only need to update the dataVolumeTemplate if the restore name is different
						if dvtCopy.Name != vr.PersistentVolumeClaimName {
							//TODO, dealing with filesystem type volume since the CDI will rewrite some volume data
							// to cause the system failed to boot
							dvtCopy.Name = vr.PersistentVolumeClaimName
							dvtCopy.Spec.Source = cdiv1.DataVolumeSource{
								Blank: &cdiv1.DataVolumeBlankImage{},
							}
							dvtCopy.Spec.PVC.DataSource = &corev1.TypedLocalObjectReference{
								Name:     vr.VolumeBackupName,
								Kind:     "VolumeSnapshot",
								APIGroup: &snapshotAPIGroup,
							}
							if dvtCopy.Annotations == nil {
								dvtCopy.Annotations = make(map[string]string, 1)
							}
							dvtCopy.Annotations[restoreNameAnnotation] = vmRestore.Name
							newTemplates[templateIndex] = *dvtCopy

							nv := vol.DeepCopy()
							nv.DataVolume.Name = vr.PersistentVolumeClaimName
							newVolumes[j] = *nv
							updatedStatus = true
						}
					} else {
						// convert dv to PersistentVolumeClaim volume
						nv := kubevirtv1.Volume{
							Name: vol.Name,
							VolumeSource: kubevirtv1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: vr.PersistentVolumeClaimName,
								},
							},
						}
						newVolumes[j] = nv
					}
				} else {
					// 2. vm volume is pvc or others
					nv := vol.DeepCopy()
					nv.PersistentVolumeClaim.ClaimName = vr.PersistentVolumeClaimName
					newVolumes[j] = *nv
				}
			}
		}
	}
	return newTemplates, newVolumes, updatedStatus
}

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
	if reflect.DeepEqual(source.VirtualMachineSpec, kubevirtv1.VirtualMachineSpec{}) {
		return false, fmt.Errorf("unexpected snapshot source")
	}

	newTemplates, newVolumes, updatedStatus := ReconcileVMTemplate(source.VirtualMachineSpec, t.vmRestore)

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

	newVM := t.vm.DeepCopy()
	newVM.Spec = *source.VirtualMachineSpec
	newVM.Spec.DataVolumeTemplates = newTemplates
	newVM.Spec.Template.Spec.Volumes = newVolumes
	if newVM.Annotations == nil {
		newVM.Annotations = make(map[string]string)
	}
	newVM.Annotations[lastRestoreAnnotation] = restoreID
	newVM.Annotations[restoreNameAnnotation] = t.vmRestore.Name

	_, err = t.handler.vms.Update(newVM)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (h *RestoreHandler) createNewVM(vmRestore *harvesterapiv1.VirtualMachineRestore) (*vmRestoreTarget, error) {
	vmName := vmRestore.Spec.Target.Name
	logrus.Infof("restore target does not exist, creating a new vm %s", vmName)
	content, err := h.getBackupContent(vmRestore, vmRestore.UID, vmRestore.Spec.NewVM)
	if err != nil {
		return nil, err
	}

	restoreCpy := vmRestore.DeepCopy()
	if content == nil {
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, fmt.Sprintf("failed to find vm backup resource %s", restoreCpy.Spec.VirtualMachineBackupName)))
		return nil, h.doUpdate(vmRestore, restoreCpy)
	}

	updated, err := h.reconcileVolumeRestores(restoreCpy, content)
	if err != nil {
		logrus.Error("error reconciling VolumeRestores")
		return nil, h.doUpdateError(vmRestore, restoreCpy, err)
	}

	if updated {
		updateRestoreCondition(restoreCpy, newProgressingCondition(corev1.ConditionTrue, "Creating new PVCs"))
		updateRestoreCondition(restoreCpy, newReadyCondition(corev1.ConditionFalse, "Waiting for new PVCs"))
		if vmRestore, err = h.restores.Update(restoreCpy); err != nil {
			return nil, err
		}
	}

	var running = true
	restoreID := getRestoreID(vmRestore)
	vmCpy := content.Spec.Source.DeepCopy()
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmName,
			Namespace: vmRestore.Namespace,
			Annotations: map[string]string{
				lastRestoreAnnotation: restoreID,
				restoreNameAnnotation: vmRestore.Name,
			},
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Running: &running,
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
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

	newTemplates, newVolumes, _ := ReconcileVMTemplate(&vm.Spec, vmRestore)
	vm.Spec.DataVolumeTemplates = newTemplates
	vm.Spec.Template.Spec.Volumes = newVolumes

	for _, net := range vm.Spec.Template.Spec.Domain.Devices.Interfaces {
		// remove the copied mac address of the default management network
		if net.Name == "default" {
			net.MacAddress = ""
		}
	}

	newVM, err := h.vms.Create(vm)
	if err != nil {
		return nil, err
	}

	if vmRestore.Status.TargetUID == nil {
		vmRestore.Status.TargetUID = &newVM.UID
	}
	if _, err = h.restores.Update(vmRestore); err != nil {
		return nil, err
	}

	return &vmRestoreTarget{
		handler:   h,
		vmRestore: vmRestore,
		vm:        newVM,
	}, nil
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

func (h *RestoreHandler) VMOnChange(key string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
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

func getRestoreID(vmRestore *harvesterapiv1.VirtualMachineRestore) string {
	return fmt.Sprintf("%s-%s", vmRestore.Name, vmRestore.UID)
}
