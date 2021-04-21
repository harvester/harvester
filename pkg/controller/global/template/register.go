package template

import (
	"context"

	"github.com/rancher/steve/pkg/server"

	"github.com/harvester/harvester/pkg/config"
)

func Register(ctx context.Context, scaled *config.Scaled, server *server.Server, options config.Options) error {
	templates := scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplate()
	templateVersions := scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplateVersion()

	return initData(templates, templateVersions, options.Namespace)
}
