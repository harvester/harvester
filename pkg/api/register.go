package api

import (
	"context"

	"github.com/rancher/steve/pkg/server"

	"github.com/rancher/harvester/pkg/api/image"
	"github.com/rancher/harvester/pkg/api/keypair"
	"github.com/rancher/harvester/pkg/api/setting"
	"github.com/rancher/harvester/pkg/api/vm"
	"github.com/rancher/harvester/pkg/api/vmtemplate"
	"github.com/rancher/harvester/pkg/config"
)

type registerSchema func(scaled *config.Scaled, server *server.Server) error

func registerSchemas(scaled *config.Scaled, server *server.Server, registers ...registerSchema) error {
	for _, register := range registers {
		if err := register(scaled, server); err != nil {
			return err
		}
	}
	return nil
}

func Setup(ctx context.Context, server *server.Server) error {
	scaled := config.ScaledWithContext(ctx)
	return registerSchemas(scaled, server,
		image.RegisterSchema,
		keypair.RegisterSchema,
		vmtemplate.RegisterSchema,
		vm.RegisterSchema,
		setting.RegisterSchema)
}
