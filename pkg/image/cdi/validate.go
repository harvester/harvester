package cdi

import (
	"fmt"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

type Validator struct {
	vmiv     common.VMIValidator
	podCache ctlcorev1.PodCache
}

func GetValidator(vmiv common.VMIValidator, podCache ctlcorev1.PodCache) backend.Validator {
	podCache.AddIndexer(util.IndexPodByPVC, util.IndexPodByPVCFunc)
	return &Validator{vmiv: vmiv, podCache: podCache}
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

	if err := cv.checkPVCInUse(vmImg); err != nil {
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

func (cv *Validator) Delete(_ *harvesterv1.VirtualMachineImage) error {
	return nil
}

func (cv *Validator) checkPVCInUse(vmImg *harvesterv1.VirtualMachineImage) error {
	index := fmt.Sprintf("%s-%s", vmImg.Spec.PVCNamespace, vmImg.Spec.PVCName)
	pods, err := cv.podCache.GetByIndex(util.IndexPodByPVC, index)
	if err == nil && len(pods) > 0 {
		podList := make([]string, 0, len(pods))
		for _, pod := range pods {
			podList = append(podList, fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
		}
		return fmt.Errorf("PVC %s is used by Pods %v, cannot export volume when it's running", vmImg.Spec.PVCName, podList)
	}
	return nil
}
