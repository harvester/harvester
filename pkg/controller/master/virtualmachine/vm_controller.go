package virtualmachine

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	wranglername "github.com/rancher/wrangler/v3/pkg/name"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	kubevirt "kubevirt.io/api/core"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	backendstorage "kubevirt.io/kubevirt/pkg/storage/backend-storage"
	kubevirtutil "kubevirt.io/kubevirt/pkg/util"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlcdiv1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/image/cdi"
	"github.com/harvester/harvester/pkg/indexeres"
	"github.com/harvester/harvester/pkg/util"
	utilHelm "github.com/harvester/harvester/pkg/util/helm"
	rqutils "github.com/harvester/harvester/pkg/util/resourcequota"
)

const (
	backendStorageJobTTL            = 300
	backendStorageJobBackoffLimit   = 3
	backendStorageJobLabelKey       = "harvesterhci.io/efi-rename-vm"
	maxBackendStorageCloneRetries   = 5
	backendStorageCloneTimeout      = 30 * time.Minute
	backendStorageCloneRequeueDelay = 5 * time.Second
)

var fetchImageFromHelmValues = utilHelm.FetchImageFromHelmValues

// A list of labels that are always automatically synchronized with the
// VMI in addition to the instance labels.
var syncLabelsToVmi = []string{util.LabelMaintainModeStrategy}

type VMController struct {
	dataVolumeClient ctlcdiv1.DataVolumeClient
	jobClient        ctlbatchv1.JobClient
	jobCache         ctlbatchv1.JobCache
	podClient        v1.PodClient
	pvcClient        v1.PersistentVolumeClaimClient
	pvcCache         v1.PersistentVolumeClaimCache
	vmClient         ctlkubevirtv1.VirtualMachineClient
	vmController     ctlkubevirtv1.VirtualMachineController
	vmiCache         ctlkubevirtv1.VirtualMachineInstanceCache
	vmiClient        ctlkubevirtv1.VirtualMachineInstanceClient
	vmImgClient      ctlharvesterv1.VirtualMachineImageClient
	vmImgCache       ctlharvesterv1.VirtualMachineImageCache
	vmBackupClient   ctlharvesterv1.VirtualMachineBackupClient
	vmBackupCache    ctlharvesterv1.VirtualMachineBackupCache
	scClient         ctlstoragev1.StorageClassClient
	scCache          ctlstoragev1.StorageClassCache
	snapshotClient   ctlsnapshotv1.VolumeSnapshotClient
	snapshotCache    ctlsnapshotv1.VolumeSnapshotCache
	clientset        kubernetes.Interface
	settingCache     ctlharvesterv1.SettingCache
	recorder         record.EventRecorder

	vmrCalculator *rqutils.Calculator
}

// createPVCsFromAnnotation creates PVCs defined in the volumeClaimTemplates annotation if they don't exist.
func (h *VMController) createPVCsFromAnnotation(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil {
		return nil, nil
	}
	volumeClaimTemplatesStr, ok := vm.Annotations[util.AnnotationVolumeClaimTemplates]
	if !ok || volumeClaimTemplatesStr == "" {
		return nil, nil
	}
	entries, err := util.UnmarshalVolumeClaimTemplates(volumeClaimTemplatesStr)
	if err != nil {
		return nil, err
	}
	pvcs := util.GetPVCsFromVolumeClaimTemplates(entries)

	for _, pvcAnno := range pvcs {
		imageName := ""
		imageNS := ""
		createPVCWithDataVolume := false
		// check VM Image Backend
		if imageIDRaw, ok := pvcAnno.Annotations[util.AnnotationImageID]; ok {
			imageID := strings.Split(imageIDRaw, "/")
			if len(imageID) == 2 {
				imageName = imageID[1]
				imageNS = imageID[0]
				vmImage, err := h.vmImgCache.Get(imageNS, imageName)
				if err != nil {
					logrus.Warnf("The corresponding VMImage is not created yet, error: %v. Skip this one", err)
					continue
				}
				if vmImage.Spec.Backend == harvesterv1.VMIBackendCDI {
					createPVCWithDataVolume = true
				}
			}
		}

		// create DataVolume for PVC (only for CDI backend)
		if createPVCWithDataVolume {
			if _, err := h.dataVolumeClient.Get(vm.Namespace, pvcAnno.Name, metav1.GetOptions{}); apierrors.IsNotFound(err) {
				targetSC, err := h.getPVCStorageClass(pvcAnno)
				if err != nil {
					return nil, fmt.Errorf("failed to get StorageClass %s: %w", *pvcAnno.Spec.StorageClassName, err)
				}
				if err := h.createDataVolume(pvcAnno.Name, vm.Namespace, imageName, imageNS, targetSC, pvcAnno.Spec.Resources); err != nil {
					return nil, err
				}
				continue
			} else if err != nil {
				return nil, err
			}
		}

		// check pvc
		pvc, err := h.pvcCache.Get(vm.Namespace, pvcAnno.Name)
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		}
		if apierrors.IsNotFound(err) {
			if !createPVCWithDataVolume {
				pvcAnno.Namespace = vm.Namespace
				if _, err = h.pvcClient.Create(pvcAnno); err != nil {
					return nil, err
				}
			}
			continue
		}

		// Users also can resize volumes through Volumes page. In that case, we can't track the update in VM annotation.
		// If storage request in the VM annotation is smaller than or equal to the real PVC, we skip it.
		if pvcAnno.Spec.Resources.Requests.Storage().Cmp(*pvc.Spec.Resources.Requests.Storage()) <= 0 {
			continue
		}

		toUpdate := pvc.DeepCopy()
		toUpdate.Spec.Resources.Requests = pvcAnno.Spec.Resources.Requests
		if reflect.DeepEqual(toUpdate, pvc) {
			continue
		}

		_, err = h.pvcClient.Update(toUpdate)
		if err == nil {
			continue
		}

		if strings.Contains(err.Error(), util.PVCExpandErrorPrefix) {
			logrus.Warnf("in VM controller, PVC expand error: %v", err)
			continue
		}

		return nil, err
	}

	return nil, nil
}

// SyncLabelsToVmi synchronizes the labels in the VM spec to the existing VMI without re-deployment
func (h *VMController) SyncLabelsToVmi(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil || vm.Spec.Template == nil {
		return vm, nil
	}

	vmi, err := h.vmiCache.Get(vm.Namespace, vm.Name)
	if err != nil && apierrors.IsNotFound(err) {
		return vm, nil
	} else if err != nil {
		return nil, fmt.Errorf("get vmi %s/%s failed, error: %w", vm.Namespace, vm.Name, err)
	}

	if err := h.syncLabels(vm, vmi); err != nil {
		return nil, err
	}

	return vm, nil
}

func (h *VMController) syncLabels(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) error {
	vmiCopy := vmi.DeepCopy()

	// Modification of the following reserved kubevirt.io/ labels on a VMI object is prohibited by the admission webhook
	// "virtualmachineinstances-update-validator.kubevirt.io", so ignore those labels during synchronization.
	// Add or update the labels of VMI
	for k, v := range vm.Spec.Template.ObjectMeta.Labels {
		if !strings.HasPrefix(k, kubevirt.GroupName) && vmi.Labels[k] != v {
			if len(vmiCopy.Labels) == 0 {
				vmiCopy.Labels = make(map[string]string)
			}
			vmiCopy.Labels[k] = v
		}
	}
	// delete the labels exist in the `vmi.Labels` but not in the `vm.spec.template.objectMeta.Labels`
	for k := range vmi.Labels {
		if _, ok := vm.Spec.Template.ObjectMeta.Labels[k]; !strings.HasPrefix(k, kubevirt.GroupName) && !ok {
			delete(vmiCopy.Labels, k)
		}
	}

	// Finally synchronize several extra labels.
	for _, k := range syncLabelsToVmi {
		value, ok := vm.Labels[k]
		if !ok {
			continue
		}
		if len(vmiCopy.Labels) == 0 {
			vmiCopy.Labels = make(map[string]string)
		}
		vmiCopy.Labels[k] = value
	}

	if !reflect.DeepEqual(vmi, vmiCopy) {
		if _, err := h.vmiClient.Update(vmiCopy); err != nil {
			return fmt.Errorf("sync labels of vm %s/%s to vmi failed, error: %w", vm.Namespace, vm.Name, err)
		}
	}

	return nil
}

// StoreRunStrategy stores the last running strategy into the annotation before the VM is stopped.
// As a workaround for the issue https://github.com/kubevirt/kubevirt/issues/7295
func (h *VMController) StoreRunStrategy(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil || vm.Spec.Template == nil {
		return vm, nil
	}

	runStrategy, err := vm.RunStrategy()
	if err != nil {
		logrus.Warnf("Skip store run strategy to the annotation, error: %s", err)
		return vm, nil
	}

	if runStrategy != kubevirtv1.RunStrategyHalted && vm.Annotations[util.AnnotationRunStrategy] != string(runStrategy) {
		toUpdate := vm.DeepCopy()
		if toUpdate.Annotations == nil {
			toUpdate.Annotations = make(map[string]string)
		}
		toUpdate.Annotations[util.AnnotationRunStrategy] = string(runStrategy)
		if _, err := h.vmClient.Update(toUpdate); err != nil {
			return vm, err
		}
	}

	return vm, nil
}

// cleanupPVCAndSnapshot handle related PVC and delete related VMBackup snapshot
func (h *VMController) cleanupPVCAndSnapshot(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil {
		return vm, nil
	}

	if err := h.removeVMBackupSnapshot(vm); err != nil {
		return vm, err
	}

	if err := h.removePVCInRemovedPVCsAnnotation(vm); err != nil {
		return vm, err
	}

	return vm, nil
}

// removePVCInRemovedPVCsAnnotation cleanup PVCs which users want to remove with the VM.
func (h *VMController) removePVCInRemovedPVCsAnnotation(vm *kubevirtv1.VirtualMachine) error {
	for _, pvcName := range getRemovedPVCs(vm) {
		if err := h.pvcClient.Delete(vm.Namespace, pvcName, &metav1.DeleteOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("can't delete PVC %s/%s, err: %w", vm.Namespace, pvcName, err)
		}
	}
	return nil
}

// removeVMBackupSnapshot only works on VMBackup snapshot.
// This function will set PVC as VolumeSnapshot's owner reference and remove related VMBackup snapshot.
func (h *VMController) removeVMBackupSnapshot(vm *kubevirtv1.VirtualMachine) error {
	vmBackups, err := h.vmBackupCache.GetByIndex(indexeres.VMBackupBySourceVMNameIndex, vm.Name)
	if err != nil {
		return fmt.Errorf("can't get VMBackups from index %s, err: %w", indexeres.VMBackupBySourceVMNameIndex, err)
	}

	for _, vmBackup := range vmBackups {
		// Only the in-cluster Snapshot type benefits from re-parenting the
		// VolumeSnapshot to its PVC; remote-target types (currently Backup)
		// manage their own external state and create only transient
		// VolumeSnapshots that are cleaned up by the engine.
		if vmBackup.Spec.Type.UsesRemoteBackupTarget() {
			continue
		}

		for _, vb := range vmBackup.Status.VolumeBackups {
			volumeSnapshot, err := h.snapshotCache.Get(vmBackup.Namespace, *vb.Name)
			if err != nil {
				return fmt.Errorf("can't get VolumeSnapshot %s/%s, err: %w", vmBackup.Namespace, *vb.Name, err)
			}
			pvc, err := h.pvcCache.Get(vb.PersistentVolumeClaim.ObjectMeta.Namespace, vb.PersistentVolumeClaim.ObjectMeta.Name)
			if err != nil {
				return fmt.Errorf("can't get PVC %s/%s, err: %w", vb.PersistentVolumeClaim.ObjectMeta.Namespace, vb.PersistentVolumeClaim.ObjectMeta.Name, err)
			}

			volumeSnapshotCpy := volumeSnapshot.DeepCopy()
			volumeSnapshotCpy.OwnerReferences = []metav1.OwnerReference{
				{
					APIVersion: util.PersistentVolumeClaimsKind.GroupVersion().String(),
					Kind:       util.PersistentVolumeClaimsKind.Kind,
					Name:       pvc.Name,
					UID:        pvc.UID,
				},
			}
			if !reflect.DeepEqual(volumeSnapshot, volumeSnapshotCpy) {
				if _, err := h.snapshotClient.Update(volumeSnapshotCpy); err != nil {
					return fmt.Errorf("can't update VolumeSnapshot %+v, err: %w", volumeSnapshotCpy, err)
				}
			}
		}

		if err := h.vmBackupClient.Delete(vmBackup.Namespace, vmBackup.Name, metav1.NewDeleteOptions(0)); err != nil {
			return fmt.Errorf("can't delete VMBackup %s/%s, err: %w", vmBackup.Namespace, vmBackup.Name, err)
		}
	}
	return nil
}

// getRemovedPVCs returns removed PVCs.
func getRemovedPVCs(vm *kubevirtv1.VirtualMachine) []string {
	results := []string{}
	for _, pvcName := range strings.Split(vm.Annotations[util.RemovedPVCsAnnotationKey], ",") {
		pvcName = strings.TrimSpace(pvcName)
		if pvcName == "" {
			continue
		}
		results = append(results, pvcName)
	}
	return results
}

func (h *VMController) SetHaltIfInsufficientResourceQuota(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil || vm.Spec.Template == nil {
		return vm, nil
	}

	err := h.vmrCalculator.CheckIfVMCanStartByResourceQuota(vm)
	if err == nil {
		if vm.Status.PrintableStatus == kubevirtv1.VirtualMachineStatusStarting {
			logrus.Debugf("VM %s in namespace %s is starting", vm.Name, vm.Namespace)

			// Since VMI would wait for Pod to run status,
			// cause the VM is still in Starting,
			// that recalculates the VM resource quota each time until the VM is non-Starting.
			h.requeueVM(vm)
			return vm, nil
		}
		return vm, h.cleanUpInsufficientResourceAnnotation(vm)
	} else if !rqutils.IsInsufficientResourceError(err) {
		return nil, err
	}

	// Refer: https://github.com/harvester/harvester/issues/7585
	// When VM is detected as insufficient resource, there is one race case:
	//  the VM's POD is created and the ResourceQuota is updated, but vmrCalculator's local cache does not have this POD yet
	// Re-check POD on ApiServer when insufficient resource happens
	exist, err1 := h.isVMPodExistingOnAPIServer(vm)
	if err1 != nil {
		errNew := fmt.Errorf("SetHaltIfInsufficientResourceQuota: VM %s/%s reports error %w, but failed to recheck POD, error %w", vm.Namespace, vm.Name, err, err1)
		logrus.Debugf("%s", err.Error())
		return nil, errNew
	}
	if exist {
		logrus.Infof("SetHaltIfInsufficientResourceQuota: VM %s/%s reports error %s, but the POD is existing, enqueue to re-check", vm.Namespace, vm.Name, err.Error())
		// next time, the CheckIfVMCanStartByResourceQuota will get the already existing POD
		h.vmController.EnqueueAfter(vm.Namespace, vm.Name, 1*time.Second)
		return vm, nil
	}

	return vm, h.stopVM(vm, err.Error())
}

func (h *VMController) stopVM(vm *kubevirtv1.VirtualMachine, errMsg string) error {
	return stopVM(h.vmClient, h.recorder, vm, errMsg)
}

func (h *VMController) cleanUpInsufficientResourceAnnotation(vm *kubevirtv1.VirtualMachine) error {
	if vm.Annotations == nil {
		return nil
	}
	if _, ok := vm.Annotations[util.AnnotationInsufficientResourceQuota]; !ok {
		return nil
	}

	runStrategy, err := vm.RunStrategy()
	if err != nil {
		return err
	}
	if runStrategy == kubevirtv1.RunStrategyHalted {
		logrus.Debugf("skip cleaning up insufficient resource annotation, VM %s in namespace %s is halted.", vm.Name, vm.Namespace)
		return nil
	}

	vmi, err := h.vmiCache.Get(vm.Namespace, vm.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Debugf("skip cleaning up insufficient resource annotation, VMI %s in namespace %s did not exist.", vm.Name, vm.Namespace)
			return nil
		}
		return err
	}
	if !vmi.IsRunning() {
		logrus.Debugf("skip cleaning up insufficient resource annotation, VM %s in namespace %s is not running.", vm.Name, vm.Namespace)
		return nil
	}

	vmCpy := vm.DeepCopy()
	delete(vmCpy.Annotations, util.AnnotationInsufficientResourceQuota)
	_, err = h.vmClient.Update(vmCpy)

	return err
}

func stopVM(vms ctlkubevirtv1.VirtualMachineClient, recorder record.EventRecorder, vm *kubevirtv1.VirtualMachine, errMsg string) error {
	logrus.Debugf("stop the VM %s in namespace %s due to insufficient resource quota: %s", vm.Name, vm.Namespace, errMsg)

	vmCpy := vm.DeepCopy()

	if vmCpy.Annotations == nil {
		vmCpy.Annotations = make(map[string]string)
	}
	vmCpy.ObjectMeta.Annotations[util.AnnotationInsufficientResourceQuota] = errMsg

	runStrategy := kubevirtv1.RunStrategyHalted
	vmCpy.Spec.RunStrategy = &runStrategy
	_, err := vms.Update(vmCpy)
	if err != nil {
		return fmt.Errorf("error updating run strategy for vm %s in namespace %s: %v", vmCpy.Name, vmCpy.Namespace, err)
	}

	recorder.Eventf(vm, corev1.EventTypeWarning, "InsufficientResourceQuota", "Set runStrategy to %s: %s", kubevirtv1.RunStrategyHalted, errMsg)
	return nil
}

// removeDeprecatedFinalizer remove deprecated finalizer, so removing vm will not be blocked
func (h *VMController) removeDeprecatedFinalizer(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil {
		return vm, nil
	}
	vmObj := vm.DeepCopy()
	util.RemoveFinalizer(vmObj, deprecatedHarvesterUnsetOwnerOfPVCsFinalizer)
	util.RemoveFinalizer(vmObj, util.GetWranglerFinalizerName(deprecatedVMUnsetOwnerOfPVCsFinalizer))
	if !reflect.DeepEqual(vm, vmObj) {
		return h.vmClient.Update(vmObj)
	}
	return vm, nil
}

// List the VM related POD on APIServer instead of local cache
// Note: this is slower than query from cache, call it when really necessary
func (h *VMController) isVMPodExistingOnAPIServer(vm *kubevirtv1.VirtualMachine) (bool, error) {
	pods, err := h.podClient.List(vm.Namespace, metav1.ListOptions{
		LabelSelector: labels.Set{
			util.LabelVMName: vm.Name,
		}.String(),
	})
	if err != nil {
		return false, err
	} else if len(pods.Items) == 0 {
		return false, nil
	}
	return true, nil
}

func (h *VMController) createDataVolume(pvcName, pvcNS, imageID, imageNS string, targetSC *storagev1.StorageClass, reqResources corev1.VolumeResourceRequirements) error {
	// generate dataVolumeTempate
	dataVolumeTemplate := &cdiv1.DataVolume{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: cdi.GenerateDVAnnotations(targetSC),
			Name:        pvcName,
			Namespace:   pvcNS,
		},
		Spec: cdiv1.DataVolumeSpec{
			Source: &cdiv1.DataVolumeSource{
				PVC: &cdiv1.DataVolumeSourcePVC{
					Namespace: imageNS,
					Name:      imageID,
				},
			},
			Storage: &cdiv1.StorageSpec{
				StorageClassName: &targetSC.Name,
				Resources:        reqResources,
			},
		},
	}

	if _, err := h.dataVolumeClient.Create(dataVolumeTemplate); err != nil {
		return fmt.Errorf("failed to create DataVolume %s/%s: %w", pvcNS, pvcName, err)
	}

	return nil
}

// SetTargetVolumeStrategy detects targetVolume entries in volumeClaimTemplates
// and updates the VM spec to trigger KubeVirt volume migration.
func (h *VMController) SetTargetVolumeStrategy(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil || vm.Spec.Template == nil {
		return vm, nil
	}

	volumeClaimTemplatesStr, ok := vm.Annotations[util.AnnotationVolumeClaimTemplates]
	if !ok || volumeClaimTemplatesStr == "" {
		return vm, nil
	}

	entries, err := util.UnmarshalVolumeClaimTemplates(volumeClaimTemplatesStr)
	if err != nil {
		return nil, err
	}

	// Collect target volumes: old PVC name -> new name
	targetVolumes := map[string]string{}
	for _, entry := range entries {
		if entry.TargetVolume != "" {
			targetVolumes[entry.Name] = entry.TargetVolume
		}
	}
	if len(targetVolumes) == 0 {
		return vm, nil
	}

	vmCopy := vm.DeepCopy()
	for i, vol := range vmCopy.Spec.Template.Spec.Volumes {
		oldName, hotpluggable := getVolumeBackingRef(vol)

		// oldName is empty means the volume is not a PVC, skip it
		if oldName == "" {
			continue
		}

		newName, ok := targetVolumes[oldName]
		if !ok {
			continue
		}

		vmCopy.Spec.Template.Spec.Volumes[i].VolumeSource = buildPVCVolumeSource(newName, hotpluggable)
	}

	strategy := kubevirtv1.UpdateVolumesStrategyMigration
	vmCopy.Spec.UpdateVolumesStrategy = &strategy

	if reflect.DeepEqual(vm, vmCopy) {
		return h.clearMigrationWait(vm)
	}

	// First round: set spec + waiting annotation
	vmCopy.Annotations[util.AnnotationWaitingStorageMigration] = "true"
	return h.vmClient.Update(vmCopy)
}

// getVolumeBackingRef returns the PVC name and hotpluggable flag if the volume is a PVC, otherwise returns ok=false.
func getVolumeBackingRef(vol kubevirtv1.Volume) (name string, hotpluggable bool) {
	if vol.PersistentVolumeClaim != nil {
		return vol.PersistentVolumeClaim.ClaimName, vol.PersistentVolumeClaim.Hotpluggable
	}
	return "", false
}

func buildPVCVolumeSource(name string, hotpluggable bool) kubevirtv1.VolumeSource {
	return kubevirtv1.VolumeSource{
		PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
			PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: name,
			},
			Hotpluggable: hotpluggable,
		},
	}
}

func (h *VMController) clearMigrationWait(vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm.Status.PrintableStatus == kubevirtv1.VirtualMachineStatusMigrating {
		if _, ok := vm.Annotations[util.AnnotationWaitingStorageMigration]; ok {
			vmCopy := vm.DeepCopy()
			delete(vmCopy.Annotations, util.AnnotationWaitingStorageMigration)
			return h.vmClient.Update(vmCopy)
		}
		return vm, nil
	}
	// Not yet migrating, keep waiting
	h.requeueVM(vm)
	return vm, nil
}

// CleanupTargetVolumeAnnotation updates the volumeClaimTemplates annotation after volume migration completes.
// Migration is considered complete when:
// 1. The annotation entry has a non-empty targetVolume field
// 2. The VM spec volume already points to the targetVolume name
// 3. The VM does NOT have a VolumesChange condition (migration finished)
// When all conditions are met, the entry's PVC fields are updated from the actual new PVC in the cluster.
func (h *VMController) CleanupTargetVolumeAnnotation(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil || vm.Spec.Template == nil {
		return vm, nil
	}

	volumeClaimTemplatesStr, ok := vm.Annotations[util.AnnotationVolumeClaimTemplates]
	if !ok || volumeClaimTemplatesStr == "" {
		return vm, nil
	}

	// Skip cleanup if storage migration is waiting to start
	if _, ok := vm.Annotations[util.AnnotationWaitingStorageMigration]; ok {
		return vm, nil
	}

	// Wait until VM is back to Running and VolumesChange condition is gone
	if vm.Status.PrintableStatus != kubevirtv1.VirtualMachineStatusRunning {
		return vm, nil
	}
	if hasVolumesChangeCondition(vm) {
		return vm, nil
	}

	entries, err := util.UnmarshalVolumeClaimTemplates(volumeClaimTemplatesStr)
	if err != nil {
		return nil, err
	}

	if !hasAnyTargetVolume(entries) {
		return vm, nil
	}

	currentVolumes := getVMSpecVolumeNames(vm)
	entries, updated, err := h.finalizeCompletedMigrations(vm.Namespace, entries, currentVolumes)
	if err != nil {
		return nil, err
	}

	if !updated {
		return vm, nil
	}

	data, err := util.MarshalVolumeClaimTemplates(entries)
	if err != nil {
		return nil, err
	}

	vmCopy := vm.DeepCopy()
	vmCopy.Annotations[util.AnnotationVolumeClaimTemplates] = data
	return h.vmClient.Update(vmCopy)
}

func hasAnyTargetVolume(entries []util.VolumeClaimTemplateEntry) bool {
	for _, entry := range entries {
		if entry.TargetVolume != "" {
			return true
		}
	}
	return false
}

func getVMSpecVolumeNames(vm *kubevirtv1.VirtualMachine) map[string]bool {
	names := map[string]bool{}
	for _, vol := range vm.Spec.Template.Spec.Volumes {
		pvcName, _ := getVolumeBackingRef(vol)
		if pvcName != "" {
			names[pvcName] = true
		}
	}
	return names
}

// finalizeCompletedMigrations updates annotation entries whose targetVolume is already
// reflected in the VM spec volumes, replacing the PVC fields with the actual new PVC from the cluster.
func (h *VMController) finalizeCompletedMigrations(namespace string, entries []util.VolumeClaimTemplateEntry, currentVolumes map[string]bool) ([]util.VolumeClaimTemplateEntry, bool, error) {
	updated := false
	for i, entry := range entries {
		if entry.TargetVolume == "" || !currentVolumes[entry.TargetVolume] {
			continue
		}

		newPVC, err := h.pvcCache.Get(namespace, entry.TargetVolume)
		if err != nil {
			return nil, false, fmt.Errorf("failed to get target volume PVC %s/%s: %w", namespace, entry.TargetVolume, err)
		}

		entries[i].PersistentVolumeClaim = corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:        newPVC.Name,
				Annotations: newPVC.Annotations,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      newPVC.Spec.AccessModes,
				Resources:        newPVC.Spec.Resources,
				VolumeMode:       newPVC.Spec.VolumeMode,
				StorageClassName: newPVC.Spec.StorageClassName,
			},
		}
		entries[i].TargetVolume = ""
		updated = true
	}
	return entries, updated, nil
}

func hasVolumesChangeCondition(vm *kubevirtv1.VirtualMachine) bool {
	for _, cond := range vm.Status.Conditions {
		if string(cond.Type) == string(kubevirtv1.VirtualMachineInstanceVolumesChange) &&
			cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (h *VMController) getPVCStorageClass(pvc *corev1.PersistentVolumeClaim) (*storagev1.StorageClass, error) {
	if pvc.Spec.StorageClassName == nil {
		// find default StorageClass
		allSC, err := h.scClient.List(metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to list StorageClass: %w", err)
		}
		for _, sc := range allSC.Items {
			scAnnos := sc.GetAnnotations()
			if scAnnos == nil {
				continue
			}
			if v, ok := scAnnos[util.AnnotationIsDefaultStorageClassName]; ok && v == "true" {
				return &sc, nil
			}
		}
		return nil, fmt.Errorf("failed to find default StorageClass for PVC %s/%s", pvc.Namespace, pvc.Name)
	}
	sc, err := h.scCache.Get(*pvc.Spec.StorageClassName)
	if err != nil {
		return nil, fmt.Errorf("failed to get StorageClass %s: %w", *pvc.Spec.StorageClassName, err)
	}
	return sc, nil
}

// isWaitForFirstConsumerPVC checks if the PVC uses WaitForFirstConsumer binding mode
func (h *VMController) isWaitForFirstConsumerPVC(pvc *corev1.PersistentVolumeClaim) (bool, error) {
	sc, err := h.getPVCStorageClass(pvc)
	if err != nil {
		return false, err
	}
	if sc.VolumeBindingMode == nil {
		// Default is Immediate if not specified
		return false, nil
	}
	return *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer, nil
}

// CleanUpBackendStorageClone actively deletes backend storage jobs when a VM is being removed.
func (h *VMController) CleanUpBackendStorageClone(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil {
		return vm, nil
	}

	// Only clean up if the VM has a clone in progress
	if vm.Annotations[util.AnnotationBSCloneStatus] != util.CloneInProgress {
		return vm, nil
	}

	if err := h.deleteBackendStorageCloneJobs(vm); err != nil {
		logrus.Warn(err.Error())
		return vm, err
	}

	logrus.Infof("Deleted backend storage jobs for VM %s/%s as VM is being removed", vm.Namespace, vm.Name)
	return vm, nil
}

// ReconcileBackendStorageClone drives the backend storage (EFI/TPM) clone as an
// idempotent state machine. Each reconcile advances one step:
//
//  1. Find or create the cloned PVC (via DataSource from the source VM's backend storage PVC, clone EFI/TPM content.)
//  2. Wait for the cloned PVC to become Bound
//  3. Create the EFI rename Job (rename NVRAM file from source VM name to target VM name)
//  4. Wait for the Job to succeed
//  5. Mark clone complete: set annotation to "cloned" and restore the desired RunStrategy
//
// Clone operations built-in protections:
//   - Timeout: 30 minutes from start
//   - Retry limit: 5 job failures before giving up
//   - Cleanup: VM deletion automatically cleans up PVCs, jobs, and secrets via OwnerReferences
//   - OnRemove: CleanUpBackendStorageClone actively deletes jobs for faster cleanup
func (h *VMController) ReconcileBackendStorageClone(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if !needsBackendStorageCloneReconcile(vm) {
		return vm, nil
	}

	if vm.Annotations[util.AnnotationBSCloneStage] == util.CloneStagePreCompleted {
		return h.finalizeBackendStorageClone(vm)
	}

	expired, err := isBackendStorageCloneExpired(vm)
	if err != nil {
		return nil, err
	}
	if expired {
		return h.failBackendStorageClone(vm, fmt.Sprintf("Clone operation exceeded timeout of %v", backendStorageCloneTimeout))
	}

	// Check the clone action annotation to determine what post-clone processing is needed
	cloneAction := vm.Annotations[util.AnnotationBSCloneActions]
	if cloneAction == "" {
		return h.completeBackendStorageClone(vm)
	}

	sourceVMName := vm.Annotations[util.AnnotationBSCloneSourceVM]
	clonedPVC, ready, err := h.awaitClonedBackendStoragePVC(vm, sourceVMName)
	if err != nil || !ready {
		return vm, err
	}

	// clonePVC == nil in this condition represent skip clone.
	if clonedPVC == nil {
		return h.failBackendStorageClone(vm, fmt.Sprintf("Source VM %s has no backend storage PVC", sourceVMName))
	}

	job, waitForDeletion, err := h.reconcileBackendStorageJob(vm, sourceVMName, clonedPVC, cloneAction)
	if err != nil {
		return nil, fmt.Errorf("reconcile backend storage job for VM %s/%s: %w", vm.Namespace, vm.Name, err)
	}
	if waitForDeletion {
		h.requeueVM(vm)
		return vm, nil
	}

	finish, failed := getBackendStorageJobStatus(job)
	if !finish {
		h.requeueVM(vm)
		return vm, nil
	}
	if failed {
		return h.handleBackendStorageCloneJobFailure(vm, job)
	}
	return h.handleBackendStorageCloneJobSuccess(vm, job)
}

func needsBackendStorageCloneReconcile(vm *kubevirtv1.VirtualMachine) bool {
	if vm == nil || vm.DeletionTimestamp != nil || vm.Spec.Template == nil {
		return false
	}
	if vm.Annotations[util.AnnotationBSCloneStatus] != util.CloneInProgress {
		return false
	}
	if vm.Annotations[util.AnnotationBSCloneSourceVM] == "" {
		return false
	}
	if vm.Annotations[util.AnnotationBSCloneStartTime] == "" {
		return false
	}
	return true
}

func isBackendStorageCloneExpired(vm *kubevirtv1.VirtualMachine) (bool, error) {
	startTime, err := time.Parse(time.RFC3339, vm.Annotations[util.AnnotationBSCloneStartTime])
	if err != nil {
		return false, fmt.Errorf("failed to parse clone start time for VM %s/%s: %w", vm.Namespace, vm.Name, err)
	}
	return time.Since(startTime) > backendStorageCloneTimeout, nil
}

func (h *VMController) awaitClonedBackendStoragePVC(vm *kubevirtv1.VirtualMachine, sourceVMName string) (*corev1.PersistentVolumeClaim, bool, error) {
	clonedPVC, skipClone, err := h.reconcileClonedBackendStoragePVC(vm, sourceVMName)
	if err != nil {
		return nil, false, fmt.Errorf("reconcile cloned backend storage PVC for VM %s/%s: %w", vm.Namespace, vm.Name, err)
	}
	if skipClone {
		return nil, true, nil
	}
	if clonedPVC == nil {
		h.recorder.Eventf(vm, corev1.EventTypeWarning, "ClonePVCNotFound",
			"Source VM %s has no backend storage PVC, will retry", sourceVMName)
		h.requeueVM(vm)
		return nil, false, nil
	}

	isWaitForFirstConsumer, err := h.isWaitForFirstConsumerPVC(clonedPVC)
	if err != nil {
		return nil, false, fmt.Errorf("check storage class binding mode for PVC %s/%s: %w", clonedPVC.Namespace, clonedPVC.Name, err)
	}

	if clonedPVC.Status.Phase != corev1.ClaimBound {
		if clonedPVC.Status.Phase == corev1.ClaimPending {
			if isWaitForFirstConsumer {
				logrus.Infof("PVC %s/%s uses WaitForFirstConsumer binding mode, proceeding to create Job", clonedPVC.Namespace, clonedPVC.Name)
				return clonedPVC, true, nil
			}
			// For Immediate binding mode, wait for PVC to be bound
			h.recorder.Eventf(vm, corev1.EventTypeWarning, "ClonePVCPending",
				"Cloned backend storage PVC %s is pending, waiting for it to be bound", clonedPVC.Name)
		}
		h.requeueVM(vm)
		return nil, false, nil
	}
	return clonedPVC, true, nil
}

func (h *VMController) handleBackendStorageCloneJobFailure(vm *kubevirtv1.VirtualMachine, job *batchv1.Job) (*kubevirtv1.VirtualMachine, error) {
	retries := 0
	if retriesStr, ok := vm.Annotations[util.AnnotationBSCloneRetries]; ok {
		if r, err := strconv.Atoi(retriesStr); err == nil {
			retries = r
		}
	}
	if retries >= maxBackendStorageCloneRetries {
		return h.failBackendStorageClone(vm, fmt.Sprintf("Clone job failed after %d retries", retries))
	}

	logrus.Warnf("Backend storage job %s/%s failed, deleting to retry (attempt %d/%d)", job.Namespace, job.Name, retries+1, maxBackendStorageCloneRetries)
	if err := h.jobClient.Delete(job.Namespace, job.Name, foregroundDeleteOptions()); err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("delete failed backend storage job %s/%s: %w", job.Namespace, job.Name, err)
	}

	vmCopy := vm.DeepCopy()
	vmCopy.Annotations[util.AnnotationBSCloneRetries] = strconv.Itoa(retries + 1)
	if _, err := h.vmClient.Update(vmCopy); err != nil {
		return nil, fmt.Errorf("update retry count for VM %s/%s: %w", vm.Namespace, vm.Name, err)
	}

	h.requeueVM(vm)
	return vm, nil
}

func (h *VMController) handleBackendStorageCloneJobSuccess(vm *kubevirtv1.VirtualMachine, job *batchv1.Job) (*kubevirtv1.VirtualMachine, error) {
	vm, err := h.markBackendStorageClonePreCompleted(vm)
	if err != nil {
		return nil, fmt.Errorf("mark backend storage clone pre-completed for VM %s/%s: %w", vm.Namespace, vm.Name, err)
	}

	logrus.Infof("Backend storage job %s/%s succeeded, deleting job", job.Namespace, job.Name)
	if err := h.jobClient.Delete(job.Namespace, job.Name, foregroundDeleteOptions()); err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("delete succeeded backend storage job %s/%s: %w", job.Namespace, job.Name, err)
	}

	h.requeueVM(vm)
	return vm, nil
}

func (h *VMController) requeueVM(vm *kubevirtv1.VirtualMachine) {
	h.vmController.EnqueueAfter(vm.Namespace, vm.Name, backendStorageCloneRequeueDelay)
}

func (h *VMController) reconcileClonedBackendStoragePVC(vm *kubevirtv1.VirtualMachine, sourceVMName string) (*corev1.PersistentVolumeClaim, bool, error) {
	pvc, err := h.getCurrentClonedBackendStoragePVC(vm)
	if err != nil {
		return nil, false, err
	}
	if pvc != nil {
		return pvc, false, nil
	}

	sourcePVC, skipClone, err := h.findSourceBackendStoragePVC(vm.Namespace, sourceVMName)
	if err != nil || skipClone {
		return nil, skipClone, err
	}

	clonedPVC := buildClonedBackendStoragePVC(vm, sourcePVC)

	existingPVC, err := h.pvcClient.Create(clonedPVC)
	if err == nil {
		return existingPVC, false, nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return nil, false, err
	}
	// Defensive fallback in case a concurrent reconcile created the PVC first.
	existingPVC, err = h.getCurrentClonedBackendStoragePVC(vm)
	return existingPVC, false, err
}

func (h *VMController) reconcileBackendStorageJob(vm *kubevirtv1.VirtualMachine, sourceVMName string, clonedPVC *corev1.PersistentVolumeClaim, cloneAction string) (*batchv1.Job, bool, error) {
	job, waitForDeletion, err := h.getCurrentBackendStorageJob(vm)
	if err != nil {
		return nil, false, err
	}
	if waitForDeletion {
		return nil, true, nil
	}
	if job != nil {
		return job, false, nil
	}

	job, err = h.buildBackendStorageJob(vm, sourceVMName, clonedPVC, cloneAction)
	if err != nil {
		return nil, false, err
	}
	created, err := h.jobClient.Create(job)
	if err == nil {
		return created, false, nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return nil, false, err
	}
	job, waitForDeletion, err = h.getCurrentBackendStorageJob(vm)
	if err != nil {
		return nil, false, err
	}
	if waitForDeletion {
		return nil, true, nil
	}
	return job, false, nil
}

func (h *VMController) getCurrentClonedBackendStoragePVC(vm *kubevirtv1.VirtualMachine) (*corev1.PersistentVolumeClaim, error) {
	pvcs, err := h.pvcClient.List(vm.Namespace, metav1.ListOptions{
		LabelSelector: labels.Set{
			backendstorage.PVCPrefix: vm.Name,
			util.LabelGeneratedBy:    util.ValueGeneratedByHarvester,
		}.String(),
	})
	if err != nil {
		return nil, err
	}

	var matched []*corev1.PersistentVolumeClaim
	for i := range pvcs.Items {
		pvc := &pvcs.Items[i]
		if isOwnedByVM(vm, pvc.OwnerReferences) {
			matched = append(matched, pvc)
		}
	}

	switch len(matched) {
	case 0:
		return nil, nil
	case 1:
		return matched[0], nil
	default:
		return nil, fmt.Errorf("expect at most 1 cloned backend storage PVC for VM %s/%s, got %d", vm.Namespace, vm.Name, len(matched))
	}
}

func (h *VMController) findSourceBackendStoragePVC(namespace, sourceVMName string) (*corev1.PersistentVolumeClaim, bool, error) {
	sourcePVCs, err := h.pvcClient.List(namespace, metav1.ListOptions{
		LabelSelector: labels.Set{
			backendstorage.PVCPrefix: sourceVMName,
		}.String()})
	if err != nil {
		return nil, false, err
	}
	if len(sourcePVCs.Items) == 0 {
		logrus.Warnf("no backend storage PVC found for source VM %s/%s, skipping clone", namespace, sourceVMName)
		return nil, true, nil
	}
	if len(sourcePVCs.Items) != 1 {
		return nil, false, fmt.Errorf("expect 1 backend storage PVC for source VM %s/%s, got %d", namespace, sourceVMName, len(sourcePVCs.Items))
	}
	return &sourcePVCs.Items[0], false, nil
}

func buildClonedBackendStoragePVC(vm *kubevirtv1.VirtualMachine, sourcePVC *corev1.PersistentVolumeClaim) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: backendStorageClonePVCGenerateName(vm.Name),
			Namespace:    vm.Namespace,
			Labels: map[string]string{
				backendstorage.PVCPrefix: vm.Name,
				util.LabelGeneratedBy:    util.ValueGeneratedByHarvester,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(vm, kubevirtv1.VirtualMachineGroupVersionKind),
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: sourcePVC.Spec.AccessModes,
			DataSource: &corev1.TypedLocalObjectReference{
				Kind: "PersistentVolumeClaim",
				Name: sourcePVC.Name,
			},
			Resources:        sourcePVC.Spec.Resources,
			StorageClassName: sourcePVC.Spec.StorageClassName,
			VolumeMode:       sourcePVC.Spec.VolumeMode,
		},
	}
}

func (h *VMController) buildBackendStorageJob(vm *kubevirtv1.VirtualMachine, sourceVMName string, clonedPVC *corev1.PersistentVolumeClaim, cloneAction string) (*batchv1.Job, error) {
	script := h.buildBackendStorageScript(sourceVMName, vm.Name, cloneAction)
	image, err := fetchImageFromHelmValues(
		h.clientset,
		util.FleetLocalNamespaceName,
		util.HarvesterChartReleaseName,
		[]string{"generalJob", "image"},
	)
	if err != nil {
		return nil, fmt.Errorf("get generalJob image for backend storage job in VM %s/%s: %w", vm.Namespace, vm.Name, err)
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: backendStorageCloneJobGenerateName(vm.Name),
			Namespace:    vm.Namespace,
			Annotations: map[string]string{
				util.AnnotationBSCloneStartTime: vm.Annotations[util.AnnotationBSCloneStartTime],
			},
			Labels: map[string]string{
				backendStorageJobLabelKey: vm.Name,
				util.LabelGeneratedBy:     util.ValueGeneratedByHarvester,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(vm, kubevirtv1.VirtualMachineGroupVersionKind),
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To(int32(backendStorageJobBackoffLimit)),
			TTLSecondsAfterFinished: ptr.To(int32(backendStorageJobTTL)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:    ptr.To(int64(kubevirtutil.NonRootUID)),
						RunAsGroup:   ptr.To(int64(kubevirtutil.NonRootUID)),
						RunAsNonRoot: ptr.To(true),
						FSGroup:      ptr.To(int64(kubevirtutil.NonRootUID)),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{{
						Name:            "backend-storage",
						Image:           image.ImageName(),
						ImagePullPolicy: image.GetImagePullPolicy(),
						Command:         []string{"sh", "-c", script},
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot:             ptr.To(true),
							AllowPrivilegeEscalation: ptr.To(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "pvc",
							MountPath: "/pvc",
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "pvc",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: clonedPVC.Name,
							},
						},
					}},
				},
			},
		},
	}, nil
}

func backendStorageClonePVCGenerateName(vmName string) string {
	return wranglername.SafeConcatName(backendstorage.PVCPrefix, vmName) + "-"
}

func backendStorageCloneJobGenerateName(vmName string) string {
	return wranglername.SafeConcatName(util.BackendStorageJobPrefix, vmName) + "-"
}

// buildBackendStorageScript generates the shell script for post-clone processing.
func (h *VMController) buildBackendStorageScript(sourceVMName, targetVMName, cloneAction string) string {
	switch cloneAction {
	case util.CloneActionDeleteEFI:
		// Delete EFI NVRAM files (target VM doesn't need EFI)
		srcFile := fmt.Sprintf("%s_VARS.fd", sourceVMName)
		return strings.NewReplacer(
			"{{.Src}}", srcFile,
		).Replace(`set -e
src="/pvc/nvram/{{.Src}}"
[ -f "$src" ] && rm -f "$src"
echo "Deleted EFI NVRAM file: $src"`)

	case util.CloneActionDeleteTPMRenameEFI:
		// Delete TPM state directory and rename EFI NVRAM file
		srcFile := fmt.Sprintf("%s_VARS.fd", sourceVMName)
		dstFile := fmt.Sprintf("%s_VARS.fd", targetVMName)
		return strings.NewReplacer(
			"{{.Src}}", srcFile,
			"{{.Dst}}", dstFile,
		).Replace(`set -e
# Delete TPM state directories
tpm_dir="/pvc/swtpm"
tpm_localca_dir="/pvc/swtpm-localca"
if [ -d "$tpm_dir" ]; then
  rm -rf "$tpm_dir"
  echo "Deleted TPM state directory: $tpm_dir"
fi
if [ -d "$tpm_localca_dir" ]; then
  rm -rf "$tpm_localca_dir"
  echo "Deleted TPM local CA directory: $tpm_localca_dir"
fi

# Rename EFI NVRAM file from source VM name to target VM name
src="/pvc/nvram/{{.Src}}"
dst="/pvc/nvram/{{.Dst}}"
if [ -f "$src" ]; then
  mv "$src" "$dst"
  chmod 600 "$dst"
  echo "Renamed EFI NVRAM file: $src -> $dst"
fi`)

	case util.CloneActionRenameEFI:
		// Rename EFI NVRAM file from source VM name to target VM name
		srcFile := fmt.Sprintf("%s_VARS.fd", sourceVMName)
		dstFile := fmt.Sprintf("%s_VARS.fd", targetVMName)
		return strings.NewReplacer(
			"{{.Src}}", srcFile,
			"{{.Dst}}", dstFile,
		).Replace(`set -e
src="/pvc/nvram/{{.Src}}"
dst="/pvc/nvram/{{.Dst}}"
[ -f "$src" ] && mv "$src" "$dst"
[ -f "$dst" ] && chmod 600 "$dst"`)

	default:
		return "exit 0"
	}
}

func (h *VMController) completeBackendStorageClone(vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	vmCopy := vm.DeepCopy()

	if rs, ok := vmCopy.Annotations[util.AnnotationBSCloneRunStrategy]; ok {
		runStrategy := kubevirtv1.VirtualMachineRunStrategy(rs)
		vmCopy.Spec.RunStrategy = &runStrategy
	}

	cleanUpBSCloneAnnotations(vmCopy)
	vmCopy.Annotations[util.AnnotationBSCloneStatus] = util.CloneComplete

	return h.vmClient.Update(vmCopy)
}

func (h *VMController) failBackendStorageClone(vm *kubevirtv1.VirtualMachine, reason string) (*kubevirtv1.VirtualMachine, error) {
	vmCopy := vm.DeepCopy()

	if err := h.deleteBackendStorageCloneJobs(vm); err != nil {
		logrus.WithError(err).Warnf("Failed to delete backend storage jobs for VM %s/%s during clone failure", vm.Namespace, vm.Name)
		return nil, fmt.Errorf("delete backend storage jobs for VM %s/%s during clone failure: %w", vm.Namespace, vm.Name, err)
	}

	cleanUpBSCloneAnnotations(vmCopy)
	vmCopy.Annotations[util.AnnotationBSCloneStatus] = util.CloneFailed

	h.recorder.Eventf(vm, corev1.EventTypeWarning, "CloneFailed", "Clone operation failed: %s", reason)
	logrus.Warnf("Clone failed for VM %s/%s: %s", vm.Namespace, vm.Name, reason)

	return h.vmClient.Update(vmCopy)
}

func cleanUpBSCloneAnnotations(vm *kubevirtv1.VirtualMachine) {
	for _, key := range []string{
		util.AnnotationBSCloneStatus,
		util.AnnotationBSCloneRunStrategy,
		util.AnnotationBSCloneSourceVM,
		util.AnnotationBSCloneActions,
		util.AnnotationBSCloneRetries,
		util.AnnotationBSCloneStartTime,
		util.AnnotationBSCloneStage,
	} {
		delete(vm.Annotations, key)
	}
}

func (h *VMController) markBackendStorageClonePreCompleted(vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	vmCopy := vm.DeepCopy()
	if vmCopy.Annotations == nil {
		vmCopy.Annotations = map[string]string{}
	}
	vmCopy.Annotations[util.AnnotationBSCloneStage] = util.CloneStagePreCompleted
	return h.vmClient.Update(vmCopy)
}

func (h *VMController) finalizeBackendStorageClone(vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	// getCurrentBackendStorageJob returns one of three states:
	//   (job!=nil, wait=false) - a live job exists
	//   (job==nil, wait=true)  - no live job, but stale/deleting jobs still present
	//   (job==nil, wait=false) - no jobs at all, safe to complete
	job, waitForDeletion, err := h.getCurrentBackendStorageJob(vm)
	if err != nil {
		return nil, fmt.Errorf("get backend storage jobs for VM %s/%s while finalizing clone: %w", vm.Namespace, vm.Name, err)
	}
	if job == nil && !waitForDeletion {
		return h.completeBackendStorageClone(vm)
	}
	if job != nil && job.DeletionTimestamp == nil {
		if err := h.jobClient.Delete(job.Namespace, job.Name, foregroundDeleteOptions()); err != nil && !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("delete backend storage job %s/%s while finalizing clone: %w", job.Namespace, job.Name, err)
		}
	}

	h.requeueVM(vm)
	return vm, nil
}

// getCurrentBackendStorageJob returns the current backend storage job if it exists and is valid for the current clone operation.
// Use AnnotationBSCloneStartTime as the unique identifier for the clone operation to determine whether an existing job is stale.
func (h *VMController) getCurrentBackendStorageJob(vm *kubevirtv1.VirtualMachine) (*batchv1.Job, bool, error) {
	jobs, err := h.jobClient.List(vm.Namespace, metav1.ListOptions{
		LabelSelector: labels.Set{
			backendStorageJobLabelKey: vm.Name,
			util.LabelGeneratedBy:     util.ValueGeneratedByHarvester,
		}.String(),
	})
	if err != nil {
		return nil, false, err
	}

	var currentJob *batchv1.Job
	waitForDeletion := false
	for i := range jobs.Items {
		job := &jobs.Items[i]
		if !isOwnedByVM(vm, job.OwnerReferences) {
			continue
		}
		if job.Annotations[util.AnnotationBSCloneStartTime] != vm.Annotations[util.AnnotationBSCloneStartTime] {
			if job.DeletionTimestamp == nil {
				if err := h.jobClient.Delete(job.Namespace, job.Name, foregroundDeleteOptions()); err != nil && !apierrors.IsNotFound(err) {
					return nil, false, fmt.Errorf("delete stale backend storage job %s/%s: %w", job.Namespace, job.Name, err)
				}
			}
			waitForDeletion = true
			continue
		}
		if job.DeletionTimestamp != nil {
			waitForDeletion = true
			continue
		}
		if currentJob != nil {
			return nil, false, fmt.Errorf("expect at most 1 current backend storage job for VM %s/%s, got multiple", vm.Namespace, vm.Name)
		}
		currentJob = job
	}

	// Wait for all terminating jobs to be fully deleted before returning any current job
	if waitForDeletion {
		return nil, true, nil
	}

	if currentJob != nil {
		return currentJob, false, nil
	}

	return nil, false, nil
}

func (h *VMController) deleteBackendStorageCloneJobs(vm *kubevirtv1.VirtualMachine) error {
	jobs, err := h.jobClient.List(vm.Namespace, metav1.ListOptions{
		LabelSelector: labels.Set{
			backendStorageJobLabelKey: vm.Name,
			util.LabelGeneratedBy:     util.ValueGeneratedByHarvester,
		}.String(),
	})
	if err != nil {
		return fmt.Errorf("list backend storage jobs for VM %s/%s: %w", vm.Namespace, vm.Name, err)
	}

	for i := range jobs.Items {
		job := &jobs.Items[i]
		if !isOwnedByVM(vm, job.OwnerReferences) {
			continue
		}
		if err := h.jobClient.Delete(job.Namespace, job.Name, foregroundDeleteOptions()); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete backend storage job %s/%s: %w", job.Namespace, job.Name, err)
		}
	}
	return nil
}

func isOwnedByVM(vm *kubevirtv1.VirtualMachine, ownerRefs []metav1.OwnerReference) bool {
	for _, ownerRef := range ownerRefs {
		if ownerRef.UID == vm.UID {
			return true
		}
	}
	return false
}

func foregroundDeleteOptions() *metav1.DeleteOptions {
	propagationPolicy := metav1.DeletePropagationForeground
	return &metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}
}

// getBackendStorageJobStatus returns whether the Job has reached a terminal state
// and whether it failed terminally (backoffLimit exhausted).
func getBackendStorageJobStatus(job *batchv1.Job) (finish bool, failed bool) {
	for _, c := range job.Status.Conditions {
		switch c.Type {
		case batchv1.JobComplete:
			if c.Status == corev1.ConditionTrue {
				return true, false
			}
		case batchv1.JobFailed:
			if c.Status == corev1.ConditionTrue {
				return true, true
			}
		}
	}
	return false, false
}
