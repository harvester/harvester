package api

import (
	"context"

	"github.com/rancher/harvester/pkg/api/datavolume"
	"github.com/rancher/harvester/pkg/api/image"
	"github.com/rancher/harvester/pkg/api/keypair"
	"github.com/rancher/harvester/pkg/api/network"
	"github.com/rancher/harvester/pkg/api/node"
	"github.com/rancher/harvester/pkg/api/restore"
	"github.com/rancher/harvester/pkg/api/setting"
	"github.com/rancher/harvester/pkg/api/user"
	"github.com/rancher/harvester/pkg/api/vm"
	"github.com/rancher/harvester/pkg/api/vmtemplate"
	"github.com/rancher/harvester/pkg/config"

	"github.com/rancher/steve/pkg/server"
)

type registerSchema func(scaled *config.Scaled, server *server.Server, options config.Options) error

func registerSchemas(scaled *config.Scaled, server *server.Server, options config.Options, registers ...registerSchema) error {
	for _, register := range registers {
		if err := register(scaled, server, options); err != nil {
			return err
		}
	}
	return nil
}

func Setup(ctx context.Context, server *server.Server, controllers *server.Controllers, options config.Options) error {
	scaled := config.ScaledWithContext(ctx)
	return registerSchemas(scaled, server, options,
		image.RegisterSchema,
		keypair.RegisterSchema,
		vmtemplate.RegisterSchema,
		vm.RegisterSchema,
		setting.RegisterSchema,
		user.RegisterSchema,
		network.RegisterSchema,
		node.RegisterSchema,
		datavolume.RegisterSchema,
		restore.RegisterSchema)
}
