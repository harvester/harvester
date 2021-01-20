package template

import (
	"context"

	"github.com/rancher/steve/pkg/server"

	"github.com/rancher/harvester/pkg/config"
)

func Register(ctx context.Context, scaled *config.Scaled, server *server.Server, options config.Options) error {
	templates := scaled.HarvesterFactory.Harvester().V1alpha1().VirtualMachineTemplate()
	templateVersions := scaled.HarvesterFactory.Harvester().V1alpha1().VirtualMachineTemplateVersion()

	return initData(templates, templateVersions, options.Namespace)
}
