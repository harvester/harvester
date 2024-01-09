package template

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/indexeres"
)

// vmImageHandler watch vm image and enqueue related vm template versions.
type vmImageHandler struct {
	templateVersionCache      ctlharvesterv1.VirtualMachineTemplateVersionCache
	templateVersionController ctlharvesterv1.VirtualMachineTemplateVersionController
}

func (h *vmImageHandler) OnChanged(_ string, vmImage *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if vmImage == nil || vmImage.DeletionTimestamp != nil {
		return nil, nil
	}

	vmTemplateVersions, err := h.templateVersionCache.GetByIndex(indexeres.VMTemplateVersionByImageIDIndex, fmt.Sprintf("%s/%s", vmImage.Namespace, vmImage.Name))
	if err != nil {
		if apierrors.IsNotFound(err) {
			return vmImage, nil
		}
		return vmImage, err
	}

	for _, vmTemplateVersion := range vmTemplateVersions {
		if harvesterv1.TemplateVersionReady.IsTrue(vmTemplateVersion) {
			continue
		}
		h.templateVersionController.Enqueue(vmTemplateVersion.Namespace, vmTemplateVersion.Name)
	}
	return vmImage, nil
}
