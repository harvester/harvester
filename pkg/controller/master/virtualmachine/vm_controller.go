package virtualmachine

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
	kubevirt "kubevirt.io/api/core"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlcdiv1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/image/cdi"
	"github.com/harvester/harvester/pkg/indexeres"
	"github.com/harvester/harvester/pkg/util"
	rqutils "github.com/harvester/harvester/pkg/util/resourcequota"
)

// A list of labels that are always automatically synchronized with the
// VMI in addition to the instance labels.
var syncLabelsToVmi = []string{util.LabelMaintainModeStrategy}

type VMController struct {
	dataVolumeClient ctlcdiv1.DataVolumeClient
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
	settingCache     ctlharvesterv1.SettingCache
	recorder         record.EventRecorder

	vmrCalculator *rqutils.Calculator
}

// createPVCsFromAnnotation creates PVCs defined in the volumeClaimTemplates annotation if they don't exist.
func (h *VMController) createPVCsFromAnnotation(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil {
		return nil, nil
	}
	volumeClaimTemplates, ok := vm.Annotations[util.AnnotationVolumeClaimTemplates]
	if !ok || volumeClaimTemplates == "" {
		return nil, nil
	}
	var pvcs []*corev1.PersistentVolumeClaim
	if err := json.Unmarshal([]byte(volumeClaimTemplates), &pvcs); err != nil {
		return nil, err
	}

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
		if err != nil {
			if apierrors.IsNotFound(err) {
				if !createPVCWithDataVolume {
					pvcAnno.Namespace = vm.Namespace
					// trigger to create the blank filesystem volume
					if *pvcAnno.Spec.VolumeMode == corev1.PersistentVolumeFilesystem {
						pvcAnnos := pvcAnno.GetAnnotations()
						if pvcAnnos == nil {
							pvcAnnos = make(map[string]string)
						}
						pvcAnnos[util.AnnotationVolForVM] = "true"
						pvcAnno.SetAnnotations(pvcAnnos)
					}
					if _, err = h.pvcClient.Create(pvcAnno); err != nil {
						return nil, err
					}
				}
				continue
			}
			return nil, err
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
			logrus.Warnf("PVC expand error: %v", err)
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
		if vmBackup.Spec.Type == harvesterv1.Backup || vmBackup.Status == nil {
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
					APIVersion: pvc.APIVersion,
					Kind:       pvc.Kind,
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
			h.vmController.EnqueueAfter(vm.Namespace, vm.Name, 5*time.Second)
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
	}
	sc, err := h.scCache.Get(*pvc.Spec.StorageClassName)
	if err != nil {
		return nil, fmt.Errorf("failed to get StorageClass %s: %w", *pvc.Spec.StorageClassName, err)
	}
	return sc, nil
}
