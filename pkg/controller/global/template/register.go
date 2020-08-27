package template

import (
	"context"

	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/steve/pkg/server"
)

func Register(ctx context.Context, scaled *config.Scaled, server *server.Server) error {
	templates := scaled.HarvesterFactory.Harvester().V1alpha1().VirtualMachineTemplate()
	templateVersions := scaled.HarvesterFactory.Harvester().V1alpha1().VirtualMachineTemplateVersion()

	return initData(templates, templateVersions)
}
