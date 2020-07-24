package master

import (
	"context"

	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/vm/pkg/config"
	"github.com/rancher/wrangler/pkg/leader"
)

func Setup(ctx context.Context, server *server.Server) error {
	scaled := config.ScaledWithContext(ctx)
	go leader.RunOrDie(ctx, "", "vm-controllers", server.K8s, func(ctx context.Context) {
		if err := register(ctx, server, scaled.Management); err != nil {
			panic(err)
		}
		if err := scaled.Management.Start(); err != nil {
			panic(err)
		}
		<-ctx.Done()
	})

	return nil
}
