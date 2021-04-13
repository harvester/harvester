package template

import (
	"context"

	"github.com/rancher/harvester/pkg/config"
)

const (
	templateControllerAgentName        = "template-controller"
	templateVersionControllerAgentName = "template-version-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	templates := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplate()
	templateVersions := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplateVersion()

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
	}

	templates.OnChange(ctx, templateControllerAgentName, templateController.OnChanged)
	templateVersions.OnChange(ctx, templateVersionControllerAgentName, templateVersionController.OnChanged)
	return nil
}
