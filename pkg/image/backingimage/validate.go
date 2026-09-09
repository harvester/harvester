package backingimage

import (
	"fmt"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
	lhutil "github.com/longhorn/longhorn-manager/util"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

type Validator struct {
	vmiv    common.VMIValidator
	scCache ctlstoragev1.StorageClassCache
}

func GetValidator(vmiv common.VMIValidator, scCache ctlstoragev1.StorageClassCache) backend.Validator {
	return &Validator{vmiv: vmiv, scCache: scCache}
}

func (biv *Validator) Create(request *types.Request, vmi *harvesterv1.VirtualMachineImage) error {
	if err := biv.vmiv.CheckDisplayName(vmi); err != nil {
		return err
	}

	if err := biv.ensureBackingImageSCNotInUse(vmi); err != nil {
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

	if err := biv.ensureBackingImageAnnotationIsImmutable(oldVMI, newVMI); err != nil {
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

	if err := biv.vmiv.VMTemplateVersionOccupation(vmi); err != nil {
		return err
	}

	if err := biv.vmiv.PVCOccupation(vmi); err != nil {
		return err
	}

	if err := biv.vmiv.VMBackupOccupation(vmi); err != nil {
		return err
	}

	return nil
}

func (biv *Validator) ensureBackingImageSCNotInUse(vmi *harvesterv1.VirtualMachineImage) error {
	// Implement the logic to ensure the backing image is not already in use if one is provided.
	// This is a placeholder for the actual implementation.

	if vmi.Annotations == nil {
		return nil
	}

	if vmi.Annotations != nil {
		scOverrideName, ok := vmi.Annotations[util.AnnotationHarvesterVMImageStorageClassNameOverride]
		if !ok {
			return nil
		}

		if scOverrideName != "" {
			// Perform any necessary validation or processing for the override name here.
			if !lhutil.ValidateName(scOverrideName) {
				return werror.NewInvalidError(fmt.Sprintf("storage class name override is not valid: %s", scOverrideName), util.AnnotationHarvesterVMImageStorageClassNameOverride)
			}
		}
		// verify scOverrideName is not already in use
		_, err := biv.scCache.Get(scOverrideName)
		if err != nil {
			if !errors.IsNotFound(err) {
				return werror.NewInvalidError(fmt.Sprintf("failed to check storage class name override: %s", err.Error()), util.AnnotationHarvesterVMImageStorageClassNameOverride)
			}
		} else {
			return werror.NewInvalidError(fmt.Sprintf("storage class name override is already in use: %s", scOverrideName), util.AnnotationHarvesterVMImageStorageClassNameOverride)
		}
	}

	return nil
}

// ensureBackingImageAnnotationIsImmutable checks that the storage class name override annotation is not changed between the old and new VirtualMachineImage objects.
func (biv *Validator) ensureBackingImageAnnotationIsImmutable(oldVMI, vmi *harvesterv1.VirtualMachineImage) error {
	if oldVMI.Annotations == nil {
		oldVMI.Annotations = make(map[string]string)
	}

	if vmi.Annotations == nil {
		vmi.Annotations = make(map[string]string)
	}

	oldSCValue, okSCOld := oldVMI.Annotations[util.AnnotationHarvesterVMImageStorageClassNameOverride]
	newSCValue, okSCNew := vmi.Annotations[util.AnnotationHarvesterVMImageStorageClassNameOverride]

	if (okSCOld && !okSCNew) || (!okSCOld && okSCNew) {
		return werror.NewInvalidError(fmt.Sprintf("storage class name override annotation is immutable: %s", util.AnnotationHarvesterVMImageStorageClassNameOverride), util.AnnotationHarvesterVMImageStorageClassNameOverride)
	}

	if okSCOld && okSCNew && oldSCValue != newSCValue {
		return werror.NewInvalidError(fmt.Sprintf("storage class name override annotation is immutable: %s", util.AnnotationHarvesterVMImageStorageClassNameOverride), util.AnnotationHarvesterVMImageStorageClassNameOverride)
	}

	return nil
}
