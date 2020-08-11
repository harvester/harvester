package template

import (
	"context"

	"github.com/rancher/harvester/pkg/config"
)

const (
	templateControllerAgentName        = "template-controller"
	templateVersionControllerAgentName = "template-version-controller"
)

func Register(ctx context.Context, management *config.Management) error {
	templates := management.VMFactory.Vm().V1alpha1().Template()
	templateVersions := management.VMFactory.Vm().V1alpha1().TemplateVersion()

	templateController := &templateHandler{
		templates:            templates,
		templateVersionCache: templateVersions.Cache(),
	}

	templateVersionController := &templateVersionHandler{
		templateCache:    templates.Cache(),
		templateVersions: templateVersions,
	}

	templates.OnChange(ctx, templateControllerAgentName, templateController.OnChanged)
	templateVersions.OnChange(ctx, templateVersionControllerAgentName, templateVersionController.OnChanged)
	return nil
}
