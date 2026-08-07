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
	authorizationv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	cdicore "kubevirt.io/containerized-data-importer/pkg/common"
	cdicommon "kubevirt.io/containerized-data-importer/pkg/controller/common"

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
	scCache ctlstoragev1.StorageClassCache,
	settingCache ctlharvesterv1.SettingCache,
	backingImageCache ctllonghornv1.BackingImageCache,
	sar authorizationv1client.SubjectAccessReviewInterface) types.Validator {
	return &pvcValidator{
		pvcCache:          pvcCache,
		vmCache:           vmCache,
		kubevirtCache:     kubevirtCache,
		imageCache:        imageCache,
		scCache:           scCache,
		settingCache:      settingCache,
		backingImageCache: backingImageCache,
		sar:               sar,
	}
}

type pvcValidator struct {
	types.DefaultValidator
	pvcCache          v1.PersistentVolumeClaimCache
	vmCache           ctlkv1.VirtualMachineCache
	imageCache        ctlharvesterv1.VirtualMachineImageCache
	kubevirtCache     ctlkv1.KubeVirtCache
	scCache           ctlstoragev1.StorageClassCache
	settingCache      ctlharvesterv1.SettingCache
	backingImageCache ctllonghornv1.BackingImageCache
	sar               authorizationv1client.SubjectAccessReviewInterface
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
			admissionregv1.Create,
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
			return werror.NewConflict(message)
		}
	}

	if len(pvc.OwnerReferences) == 0 {
		return nil
	}

	return v.validateOwnerReferences(pvc)
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

	return webhookutil.CheckExpand(newPVC, v.vmCache, v.kubevirtCache, v.scCache, v.settingCache)
}

func (v *pvcValidator) Create(request *types.Request, newObj runtime.Object) error {
	newPVC := newObj.(*corev1.PersistentVolumeClaim)
	return v.validateInternalUsage(request, newPVC)
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

func (v *pvcValidator) validateInternalUsage(request *types.Request, pvc *corev1.PersistentVolumeClaim) error {
	if pvc.Spec.StorageClassName == nil {
		return nil
	}

	scName := *pvc.Spec.StorageClassName
	switch scName {
	case util.StorageClassLonghornStatic:
		isBelongToUpgradeImage, err := v.isBelongToUpgradeImage(pvc)
		if err != nil {
			return werror.NewInternalError(fmt.Sprintf("failed to check if pvc %s/%s is for internal usage: %v", pvc.Namespace, pvc.Name, err))
		}
		if !isBelongToUpgradeImage {
			message := fmt.Sprintf("can not create volume with the reserved storage class %s", scName)
			return werror.NewInvalidError(message, "spec.storageClassName")
		}
		return nil
	case util.StorageClassVmstatePersistence:
		if !isManagedByKubeVirt(pvc) {
			message := fmt.Sprintf("can not create volume with the reserved storage class %s", scName)
			return werror.NewInvalidError(message, "spec.storageClassName")
		}
		return nil
	default:
		return v.validateBackingImageAccess(request, scName)
	}
}

func (v *pvcValidator) validateBackingImageAccess(request *types.Request, scName string) error {
	sc, err := v.scCache.Get(scName)
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to get storage class %s: %v", scName, err))
	}

	if sc.Provisioner != util.CSIProvisionerLonghorn {
		return nil
	}

	biName, ok := sc.Parameters[util.LonghornOptionBackingImageName]
	if !ok || biName == "" {
		return nil
	}

	bi, err := v.backingImageCache.Get(util.LonghornSystemNamespaceName, biName)
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to get backing image %s: %v", biName, err))
	}

	vmImageID := bi.Annotations[util.AnnotationImageID]

	if vmImageID == "" {
		return nil
	}

	parts := strings.SplitN(vmImageID, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	imageNS, imageName := parts[0], parts[1]

	allowed, err := util.CheckObjectAccess(request.Context, util.ResourceAccessCheck{
		SAR:       v.sar,
		Username:  request.UserInfo.Username,
		Groups:    request.UserInfo.Groups,
		Verb:      util.VerbGet,
		GVR:       util.VirtualMachineImageGVR,
		Namespace: imageNS,
		Name:      imageName,
	})
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to check access to image %s/%s: %v", imageNS, imageName, err))
	}
	if !allowed {
		return werror.NewInvalidError(fmt.Sprintf("user %q is not allowed to access image %s/%s", request.UserInfo.Username, imageNS, imageName), "spec.storageClassName")
	}

	return nil
}

func (v *pvcValidator) isBelongToUpgradeImage(pvc *corev1.PersistentVolumeClaim) (bool, error) {
	// only longhorn-static storage class pvc could be for upgrade image
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != util.StorageClassLonghornStatic {
		return false, nil
	}

	isUpgradeImagePVC, err := v.isUpgradeImagePVC(pvc)
	if err != nil || isUpgradeImagePVC {
		return isUpgradeImagePVC, err
	}

	return v.isUpgradeImageScratchPVC(pvc)
}

func (v *pvcValidator) isUpgradeImagePVC(pvc *corev1.PersistentVolumeClaim) (bool, error) {
	// upgrade vm image is created in harvester-system namespace and so does all related resources, so only check the owner in harvester-system namespace
	if pvc.Namespace != util.HarvesterSystemNamespaceName {
		return false, nil
	}

	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != util.StorageClassLonghornStatic {
		return false, nil
	}

	for _, owner := range pvc.OwnerReferences {
		// upgrade vm image will create 3 pvcs
		// 1. image-xxxxx pvc (owner kind: DataVolume) with longhorn-static storage class, the owner here is the underlying data volume of upgrade vm image
		// 2. prime-{uuid} pvc (owner kind: PersistentVolumeClaim) with longhorn-static storage class, the owner here is the image-xxxxx pvc
		// 3. prime-{uuid}-scratch pvc (owner kind: Pod), which can inherit longhorn-static from the data pvc when CDI scratchSpaceStorageClass is not set
		// note that vm image and data volume and image-xxxxx pvc share the same name
		if owner.Kind != util.DVObjectName && owner.Kind != util.PVCObjectName {
			continue
		}

		upgradeImage, err := v.imageCache.Get(pvc.Namespace, owner.Name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return false, err
		}

		if val, ok := upgradeImage.Annotations[util.AnnotationUpgradeImage]; ok && val == "True" {
			return true, nil
		}
	}

	return false, nil
}

func (v *pvcValidator) isUpgradeImageScratchPVC(pvc *corev1.PersistentVolumeClaim) (bool, error) {
	scratchPVCSuffix := "-" + cdicore.ScratchNameSuffix

	// CDI creates scratch PVCs with the importer pod as the controller owner.
	if v.pvcCache == nil || !strings.HasSuffix(pvc.Name, scratchPVCSuffix) || !isControlledByPod(pvc) {
		return false, nil
	}

	dataPVCName := strings.TrimSuffix(pvc.Name, scratchPVCSuffix)
	if dataPVCName == "" || dataPVCName == pvc.Name {
		return false, nil
	}

	dataPVC, err := v.pvcCache.Get(pvc.Namespace, dataPVCName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return v.isUpgradeImagePVC(dataPVC)
}

func isControlledByPod(pvc *corev1.PersistentVolumeClaim) bool {
	for _, owner := range pvc.OwnerReferences {
		if owner.APIVersion == corev1.SchemeGroupVersion.String() &&
			owner.Kind == "Pod" &&
			owner.UID != "" &&
			owner.Controller != nil &&
			*owner.Controller {
			return true
		}
	}
	return false
}

// isManagedByKubeVirt checks if a PVC creation request is legitimately from KubeVirt.
// It validates multiple criteria to prevent users from bypassing the restriction:
// 1. Must have the persistent-state-for label with a non-empty value
// 2. Must have owner reference to a VM or VMI
// 3. The label value must match the owner's name (VM/VMI name)
func isManagedByKubeVirt(pvc *corev1.PersistentVolumeClaim) bool {
	if pvc.Labels == nil {
		return false
	}

	// Check if the label exists and has a non-empty value
	vmName, exists := pvc.Labels[util.LabelKubeVirtPersistentState]
	if !exists || vmName == "" {
		return false
	}

	// Verify that the PVC has an owner reference to a VM or VMI
	hasValidOwner := false
	for _, owner := range pvc.OwnerReferences {
		if owner.Kind == kubevirtv1.VirtualMachineGroupVersionKind.Kind ||
			owner.Kind == kubevirtv1.VirtualMachineInstanceGroupVersionKind.Kind {
			// The owner name should match the label value
			if owner.Name == vmName {
				hasValidOwner = true
				break
			}
		}
	}

	return hasValidOwner
}

func (v *pvcValidator) validateOwnerReferences(pvc *corev1.PersistentVolumeClaim) error {
	var ownerList []string
	for _, owner := range pvc.OwnerReferences {
		if owner.Kind == kubevirtv1.VirtualMachineGroupVersionKind.Kind {
			// check if VM exists and is not in deleting status, if so, block the deletion of PVC
			vmObj, err := v.vmCache.Get(pvc.Namespace, owner.Name)
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return werror.NewInternalError(fmt.Sprintf("failed to get VM %s/%s: %v", pvc.Namespace, owner.Name, err))
			}
			if vmObj.DeletionTimestamp == nil {
				ownerList = append(ownerList, owner.Name)
			}

		}
	}

	if len(ownerList) > 0 {
		message := fmt.Sprintf("can not delete the volume %s which is currently owned by these VMs: %s", pvc.Name, strings.Join(ownerList, ","))
		return werror.NewInvalidError(message, "")
	}
	return nil
}
