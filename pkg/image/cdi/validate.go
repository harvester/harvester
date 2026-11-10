package cdi

import (
	"fmt"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlcdiv1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	ctlkv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/indexeres"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

type Validator struct {
	vmiv     common.VMIValidator
	pvcCache ctlcorev1.PersistentVolumeClaimCache
	vmCache  ctlkv1.VirtualMachineCache
	dvCache  ctlcdiv1.DataVolumeCache
}

func GetValidator(vmiv common.VMIValidator,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	vmCache ctlkv1.VirtualMachineCache,
	dvCache ctlcdiv1.DataVolumeCache) backend.Validator {
	return &Validator{vmiv: vmiv, pvcCache: pvcCache, vmCache: vmCache, dvCache: dvCache}
}

func (cv *Validator) Create(req *types.Request, vmImg *harvesterv1.VirtualMachineImage) error {
	if err := cv.vmiv.CheckDisplayName(vmImg); err != nil {
		return err
	}

	if err := cv.vmiv.SCConsistency(nil, vmImg); err != nil {
		return err
	}

	if err := cv.vmiv.CheckURL(vmImg); err != nil {
		return err
	}

	if err := cv.vmiv.CheckPVCInUse(vmImg); err != nil {
		return err
	}

	if err := cv.vmiv.CheckImagePVC(req, vmImg); err != nil {
		return err
	}
	return nil
}

func (cv *Validator) Update(oldVMImg, newVMImg *harvesterv1.VirtualMachineImage) error {
	if err := cv.vmiv.SourceTypeConsistency(oldVMImg, newVMImg); err != nil {
		return err
	}

	if err := cv.vmiv.SCConsistency(oldVMImg, newVMImg); err != nil {
		return err
	}

	if cv.vmiv.IsExportVolume(newVMImg) {
		if err := cv.vmiv.PVCConsistency(oldVMImg, newVMImg); err != nil {
			return err
		}
	}

	if err := cv.vmiv.URLConsistency(oldVMImg, newVMImg); err != nil {
		return err
	}

	if err := cv.vmiv.CheckUpdateDisplayName(oldVMImg, newVMImg); err != nil {
		return err
	}

	return nil
}

func (cv *Validator) Delete(vmImg *harvesterv1.VirtualMachineImage) error {
	// The VM importer uses the VMImage's golden PVC directly.
	if err := cv.checkPVCUsage(vmImg); err != nil {
		return err
	}
	return cv.checkPVCClones(vmImg)
}

func (cv *Validator) checkPVCUsage(vmImg *harvesterv1.VirtualMachineImage) error {
	// CDI creates the image's DataVolume and PVC with the image's namespace and name.
	pvcKey := util.NamespacedName(vmImg.Namespace, vmImg.Name)
	vms, err := cv.vmCache.GetByIndex(indexeres.VMByPVCIndex, pvcKey)
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to get VMs using image PVC %s: %v", pvcKey, err))
	}

	for _, vm := range vms {
		if vm.DeletionTimestamp != nil {
			continue
		}
		return werror.NewConflict(fmt.Sprintf("cannot delete image %s: its PVC is used by VM %s/%s",
			pvcKey, vm.Namespace, vm.Name))
	}
	return nil
}

func (cv *Validator) checkPVCClones(vmImg *harvesterv1.VirtualMachineImage) error {
	dvs, err := cv.dvCache.List(corev1.NamespaceAll, labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to get image PVC clones: %v", err))
	}

	for _, dv := range dvs {
		if !usesImagePVC(dv, vmImg) {
			continue
		}
		if err := cv.checkClonePVC(vmImg, dv); err != nil {
			return err
		}
	}
	return nil
}

func usesImagePVC(dv *cdiv1.DataVolume, vmImg *harvesterv1.VirtualMachineImage) bool {
	if dv.Spec.Source == nil || dv.Spec.Source.PVC == nil {
		return false
	}
	source := dv.Spec.Source.PVC
	namespace := source.Namespace
	if namespace == "" {
		namespace = dv.Namespace
	}
	return namespace == vmImg.Namespace && source.Name == vmImg.Name
}

func (cv *Validator) checkClonePVC(vmImg *harvesterv1.VirtualMachineImage, dv *cdiv1.DataVolume) error {
	claimName := dv.Status.ClaimName
	if claimName == "" {
		claimName = dv.Name
	}
	_, err := cv.pvcCache.Get(dv.Namespace, claimName)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to get clone PVC %s/%s: %v",
			dv.Namespace, claimName, err))
	}
	// Keep the image while its clone PVC exists, even after cloning succeeds.
	message := fmt.Sprintf("cannot delete image %s/%s: being used by clone PVC %s/%s",
		vmImg.Namespace, vmImg.Name, dv.Namespace, claimName)
	return werror.NewConflict(message)
}
