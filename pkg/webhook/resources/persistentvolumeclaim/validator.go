package persistentvolumeclaim

import (
	"fmt"
	"strings"

	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	cdicommon "kubevirt.io/containerized-data-importer/pkg/controller/common"
	kvirtfeatures "kubevirt.io/kubevirt/pkg/virt-config/featuregate"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
	indexeresutil "github.com/harvester/harvester/pkg/util/indexeres"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/indexeres"
	"github.com/harvester/harvester/pkg/webhook/types"
	webhookutil "github.com/harvester/harvester/pkg/webhook/util"
)

func NewValidator(pvcCache v1.PersistentVolumeClaimCache,
	vmCache ctlkv1.VirtualMachineCache,
	kubevirtCache ctlkv1.KubeVirtCache,
	imageCache ctlharvesterv1.VirtualMachineImageCache,
	engineCache ctllonghornv1.EngineCache,
	scCache ctlstoragev1.StorageClassCache,
	settingCache ctlharvesterv1.SettingCache) types.Validator {
	return &pvcValidator{
		pvcCache:      pvcCache,
		vmCache:       vmCache,
		kubevirtCache: kubevirtCache,
		imageCache:    imageCache,
		engineCache:   engineCache,
		scCache:       scCache,
		settingCache:  settingCache,
	}
}

type pvcValidator struct {
	types.DefaultValidator
	pvcCache      v1.PersistentVolumeClaimCache
	vmCache       ctlkv1.VirtualMachineCache
	imageCache    ctlharvesterv1.VirtualMachineImageCache
	kubevirtCache ctlkv1.KubeVirtCache
	engineCache   ctllonghornv1.EngineCache
	scCache       ctlstoragev1.StorageClassCache
	settingCache  ctlharvesterv1.SettingCache
}

func (v *pvcValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"persistentvolumeclaims"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   corev1.SchemeGroupVersion.Group,
		APIVersion: corev1.SchemeGroupVersion.Version,
		ObjectType: &corev1.PersistentVolumeClaim{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Delete,
			admissionregv1.Update,
		},
	}
}

func (v *pvcValidator) Delete(request *types.Request, oldObj runtime.Object) error {
	if request.IsGarbageCollection() {
		return nil
	}

	oldPVC := oldObj.(*corev1.PersistentVolumeClaim)

	if err := v.checkGoldenImageAnno(oldPVC); err != nil {
		return werror.NewInvalidError(err.Error(), "")
	}

	pvc, err := v.pvcCache.Get(oldPVC.Namespace, oldPVC.Name)
	if err != nil {
		return werror.NewInvalidError(err.Error(), "metadata.name")
	}

	images, err := v.imageCache.GetByIndex(indexeres.ImageByExportSourcePVCIndex,
		fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name))
	if err != nil {
		return werror.NewInvalidError(err.Error(), "value")
	}

	for _, image := range images {
		if !harvesterv1beta1.ImageImported.IsTrue(image) {
			message := fmt.Sprintf("can not delete volume %s which is exporting for image %s/%s",
				pvc.Name, image.Namespace, image.Name)
			return werror.NewInvalidError(message, "value")
		}
	}

	vms, err := v.vmCache.GetByIndex(indexeresutil.VMByPVCIndex, ref.Construct(oldPVC.Namespace, oldPVC.Name))
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to get VMs by index: %s, PVC: %s/%s, err: %s", indexeresutil.VMByPVCIndex, oldPVC.Namespace, oldPVC.Name, err))
	}

	for _, vm := range vms {
		if vm.DeletionTimestamp == nil {
			message := fmt.Sprintf("can not delete the volume %s which is currently attached to VM: %s/%s", oldPVC.Name, vm.Namespace, vm.Name)
			return werror.NewInvalidError(message, "")
		}
	}

	if len(pvc.OwnerReferences) == 0 {
		return nil
	}

	var ownerList []string
	for _, owner := range pvc.OwnerReferences {
		if owner.Kind == kubevirtv1.VirtualMachineGroupVersionKind.Kind {
			ownerList = append(ownerList, owner.Name)
		}
	}
	if len(ownerList) > 0 {
		message := fmt.Sprintf("can not delete the volume %s which is currently owned by these VMs: %s", oldPVC.Name, strings.Join(ownerList, ","))
		return werror.NewInvalidError(message, "")
	}

	return nil
}

func (v *pvcValidator) isOnlineExpandNeeded(pvc *corev1.PersistentVolumeClaim) (bool, error) {
	vms, err := v.vmCache.GetByIndex(indexeresutil.VMByPVCIndex, ref.Construct(pvc.Namespace, pvc.Name))
	if err != nil {
		return false, werror.NewInternalError(fmt.Sprintf("failed to get VMs by index: %s, PVC: %s/%s, err: %s", indexeresutil.VMByPVCIndex, pvc.Namespace, pvc.Name, err))
	}
	for _, vm := range vms {
		if vm.Status.PrintableStatus != kubevirtv1.VirtualMachineStatusStopped {
			return true, nil
		}
	}
	return false, nil
}

func (v *pvcValidator) isHotpluggedFilesystemPVC(pvc *corev1.PersistentVolumeClaim) (bool, error) {
	// Check if the PVC is in Filesystem mode
	if pvc.Spec.VolumeMode == nil || *pvc.Spec.VolumeMode != corev1.PersistentVolumeFilesystem {
		return false, nil
	}

	// Check if the PVC is hotplugged to any VM
	vms, err := v.vmCache.GetByIndex(indexeresutil.VMByHotplugPVCIndex, ref.Construct(pvc.Namespace, pvc.Name))
	if err != nil {
		return false, werror.NewInternalError(err.Error())
	}

	return len(vms) > 0, nil
}

func isKubevirtExpandEnabled(kubevirt *kubevirtv1.KubeVirt) bool {
	featureGates := kubevirt.Spec.Configuration.DeveloperConfiguration.FeatureGates
	for _, f := range featureGates {
		if f == kvirtfeatures.ExpandDisksGate {
			return true
		}
	}

	return false
}

func (v *pvcValidator) checkExpand(pvc *corev1.PersistentVolumeClaim) error {
	// we will have an early return if there is no related VMs or all related VMs are stopped
	if onlineExpand, err := v.isOnlineExpandNeeded(pvc); err != nil || !onlineExpand {
		return err
	}

	kubevirt, err := v.kubevirtCache.Get(util.HarvesterSystemNamespaceName, util.KubeVirtObjectName)
	if err != nil {
		return err
	}
	if !isKubevirtExpandEnabled(kubevirt) {
		return werror.NewInvalidError(util.PVCExpandErrorPrefix+": kubevirt ExpandDisks not included in featureGate", "")
	}

	hotpluggedFSPVC, err := v.isHotpluggedFilesystemPVC(pvc)
	if err != nil {
		return err
	}
	if hotpluggedFSPVC {
		return werror.NewInvalidError(
			fmt.Sprintf(
				util.PVCExpandErrorPrefix+": Expansion of hotplugged PVC '%s/%s' in filesystem mode is not supported",
				pvc.Namespace,
				pvc.Name,
			),
			"",
		)
	}

	expandable, err := webhookutil.CheckOnlineExpand(pvc, v.engineCache, v.scCache, v.settingCache)
	if err != nil {
		return err
	}
	if !expandable {
		return werror.NewInvalidError(
			fmt.Sprintf(
				util.PVCExpandErrorPrefix+": pvc %s/%s is not online expandable with its provider",
				pvc.Namespace,
				pvc.Name,
			),
			"",
		)
	}

	return nil
}

func (v *pvcValidator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	oldPVC := oldObj.(*corev1.PersistentVolumeClaim)
	newPVC := newObj.(*corev1.PersistentVolumeClaim)

	oldAnno := oldPVC.GetAnnotations()
	if _, find := oldAnno[util.AnnotationGoldenImage]; find {
		newAnno := newPVC.GetAnnotations()
		if _, find := newAnno[util.AnnotationGoldenImage]; !find {
			msg := fmt.Sprintf("can not remove golden image annotation from PVC %s/%s", newPVC.Namespace, newPVC.Name)
			return werror.NewInvalidError(msg, "")
		}
		if oldAnno[util.AnnotationGoldenImage] != newAnno[util.AnnotationGoldenImage] {
			msg := fmt.Sprintf("can not change golden image annotation from PVC %s/%s", newPVC.Namespace, newPVC.Name)
			return werror.NewInvalidError(msg, "")
		}
	}

	newQuantity := newPVC.Spec.Resources.Requests.Storage()
	oldQuantity := oldPVC.Spec.Resources.Requests.Storage()
	if oldQuantity.Cmp(*newQuantity) == 0 {
		return nil
	}

	return v.checkExpand(newPVC)
}

func (v *pvcValidator) checkGoldenImageAnno(pvc *corev1.PersistentVolumeClaim) error {
	if _, find := pvc.Annotations[util.AnnotationGoldenImage]; find {
		if pvc.Annotations[util.AnnotationGoldenImage] == "true" {
			// ensure the corresponding vm image exists
			imageName := pvc.Name
			imageNS := pvc.Namespace
			if v, find := pvc.Annotations[cdicommon.AnnPopulatorKind]; find && v == cdiv1.VolumeCloneSourceRef {
				if image, find := pvc.Annotations[cdicommon.AnnEventSource]; find {
					imageRaw := strings.Split(image, "/")
					if len(imageRaw) == 2 {
						imageNS = imageRaw[0]
						imageName = imageRaw[1]
					}
				}
			}
			if _, err := v.imageCache.Get(imageNS, imageName); err != nil {
				if apierrors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("get image %s/%s failed: %v", imageNS, imageName, err)
			}
			// ignore the golden image PVC if it is in Lost/Terminating status
			if pvc.Status.Phase == corev1.ClaimLost || pvc.Status.Phase == "Terminating" {
				return nil
			}
			return fmt.Errorf("can not delete golden image PVC")
		}
	}
	return nil
}
