package backingimage

import (
	"fmt"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

type Validator struct {
	vmiv     common.VMIValidator
	pvcCache ctlcorev1.PersistentVolumeClaimCache
}

func GetValidator(vmiv common.VMIValidator,
	pvcCache ctlcorev1.PersistentVolumeClaimCache) backend.Validator {
	return &Validator{vmiv: vmiv, pvcCache: pvcCache}
}

func (biv *Validator) Create(request *types.Request, vmi *harvesterv1.VirtualMachineImage) error {
	if err := biv.vmiv.CheckDisplayName(vmi); err != nil {
		return err
	}

	// ensure backingImage is not already in use if one is provided
	if err := biv.vmiv.CheckSCExists(vmi); err != nil {
		return err
	}

	if err := biv.vmiv.SCConsistency(nil, vmi); err != nil {
		return err
	}

	if err := biv.vmiv.CheckURL(vmi); err != nil {
		return err
	}

	if err := biv.vmiv.CheckSecurityParameters(request, vmi); err != nil {
		return err
	}

	if err := biv.vmiv.CheckImagePVC(request, vmi); err != nil {
		return err
	}

	return nil
}

func (biv *Validator) Update(oldVMI, newVMI *harvesterv1.VirtualMachineImage) error {
	if err := biv.vmiv.SCParametersConsistency(oldVMI, newVMI); err != nil {
		return err
	}

	if err := biv.vmiv.SCConsistency(oldVMI, newVMI); err != nil {
		return err
	}

	if err := biv.vmiv.SourceTypeConsistency(oldVMI, newVMI); err != nil {
		return err
	}

	if biv.vmiv.IsExportVolume(newVMI) {
		if err := biv.vmiv.PVCConsistency(oldVMI, newVMI); err != nil {
			return err
		}
	}

	if err := biv.vmiv.URLConsistency(oldVMI, newVMI); err != nil {
		return err
	}

	if err := biv.vmiv.SecurityParameterConsistency(oldVMI, newVMI); err != nil {
		return err
	}

	if err := biv.vmiv.CheckUpdateDisplayName(oldVMI, newVMI); err != nil {
		return err
	}

	if err := biv.vmiv.CheckURL(newVMI); err != nil {
		return err
	}

	return nil
}

func (biv *Validator) Delete(vmi *harvesterv1.VirtualMachineImage) error {
	if biv.vmiv.GetStatusSC(vmi) == "" {
		return nil
	}

	if err := biv.checkSCUsage(vmi); err != nil {
		return err
	}

	if err := biv.vmiv.VMTemplateVersionOccupation(vmi); err != nil {
		return err
	}

	if err := biv.vmiv.VMBackupOccupation(vmi); err != nil {
		return err
	}

	return nil
}

func (biv *Validator) checkSCUsage(vmi *harvesterv1.VirtualMachineImage) error {
	scName := vmi.Status.StorageClassName
	pvcs, err := biv.pvcCache.List(corev1.NamespaceAll, labels.Everything())
	if err != nil {
		return err
	}

	for _, pvc := range pvcs {
		if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != scName {
			continue
		}
		message := fmt.Sprintf("Cannot delete image %s/%s: being used by volume %s/%s",
			vmi.Namespace, vmi.Spec.DisplayName, pvc.Namespace, pvc.Name)
		return werror.NewInvalidError(message, "")
	}
	return nil
}
