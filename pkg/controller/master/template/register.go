package template

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	templateControllerAgentName        = "template-controller"
	templateVersionControllerAgentName = "template-version-controller"
	vmImageControllerAgentName         = "vm-image-in-template-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	templates := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplate()
	templateVersions := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplateVersion()
	vmImages := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage()

	templateController := &templateHandler{
		templates:            templates,
		templateVersions:     templateVersions,
		templateVersionCache: templateVersions.Cache(),
		templateController:   templates,
	}

	templateVersionController := &templateVersionHandler{
		templateCache:      templates.Cache(),
		templateVersions:   templateVersions,
		templateController: templates,
		vmImageCache:       vmImages.Cache(),
	}

	vmImageController := &vmImageHandler{
		templateVersionCache:      templateVersions.Cache(),
		templateVersionController: templateVersions,
	}

	templates.OnChange(ctx, templateControllerAgentName, templateController.OnChanged)
	templateVersions.OnChange(ctx, templateVersionControllerAgentName, templateVersionController.OnChanged)
	vmImages.OnChange(ctx, vmImageControllerAgentName, vmImageController.OnChanged)
	return nil
}
