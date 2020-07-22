package global

import (
	"context"

	"github.com/rancher/steve/pkg/server"
	pkgcontext "github.com/rancher/vm/pkg/context"
	"github.com/rancher/vm/pkg/controller/global/image"
	"github.com/rancher/vm/pkg/controller/global/settings"
)

type registerFunc func(context.Context, *pkgcontext.Scaled, *server.Server) error

var registerFuncs = []registerFunc{
	createCRDs,
	settings.Register,
	image.Register,
}

func Setup(ctx context.Context, server *server.Server) error {
	scaled := pkgcontext.ScaledWithContext(ctx)
	for _, f := range registerFuncs {
		if err := f(ctx, scaled, server); err != nil {
			return err
		}
	}
	return nil
}
