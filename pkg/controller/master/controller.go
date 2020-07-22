package master

import (
	"context"

	"github.com/rancher/steve/pkg/server"
	pkgcontext "github.com/rancher/vm/pkg/context"
	"github.com/rancher/vm/pkg/controller/master/image"
)

type registerFunc func(context.Context, *pkgcontext.Management) error

var registerFuncs = []registerFunc{
	image.Register,
}

func register(ctx context.Context, server *server.Server, management *pkgcontext.Management) error {
	for _, f := range registerFuncs {
		if err := f(ctx, management); err != nil {
			return err
		}
	}
	return nil
}
