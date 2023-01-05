package master

import (
	"context"

	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/pkg/leader"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/controller/master/addon"
	"github.com/harvester/harvester/pkg/controller/master/backup"
	"github.com/harvester/harvester/pkg/controller/master/image"
	"github.com/harvester/harvester/pkg/controller/master/keypair"
	"github.com/harvester/harvester/pkg/controller/master/migration"
	"github.com/harvester/harvester/pkg/controller/master/node"
	"github.com/harvester/harvester/pkg/controller/master/nodedrain"
	"github.com/harvester/harvester/pkg/controller/master/rancher"
	"github.com/harvester/harvester/pkg/controller/master/setting"
	"github.com/harvester/harvester/pkg/controller/master/storagenetwork"
	"github.com/harvester/harvester/pkg/controller/master/supportbundle"
	"github.com/harvester/harvester/pkg/controller/master/template"
	"github.com/harvester/harvester/pkg/controller/master/upgrade"
	"github.com/harvester/harvester/pkg/controller/master/upgradelog"
	"github.com/harvester/harvester/pkg/controller/master/virtualmachine"
)

type registerFunc func(context.Context, *config.Management, config.Options) error

var registerFuncs = []registerFunc{
	image.Register,
	keypair.Register,
	migration.Register,
	node.PromoteRegister,
	node.MaintainRegister,
	node.DownRegister,
	setting.Register,
	template.Register,
	virtualmachine.Register,
	backup.RegisterBackup,
	backup.RegisterRestore,
	backup.RegisterBackupTarget,
	backup.RegisterBackupMetadata,
	supportbundle.Register,
	rancher.Register,
	upgrade.Register,
	upgradelog.Register,
	addon.Register,
	storagenetwork.Register,
	nodedrain.Register,
}

func register(ctx context.Context, management *config.Management, options config.Options) error {
	for _, f := range registerFuncs {
		if err := f(ctx, management, options); err != nil {
			return err
		}
	}

	return nil
}

func Setup(ctx context.Context, server *server.Server, controllers *server.Controllers, options config.Options) error {
	scaled := config.ScaledWithContext(ctx)

	go leader.RunOrDie(ctx, "", "harvester-controllers", controllers.K8s, func(ctx context.Context) {
		if err := register(ctx, scaled.Management, options); err != nil {
			panic(err)
		}
		if err := scaled.Management.Start(options.Threadiness); err != nil {
			panic(err)
		}
		<-ctx.Done()
	})

	return nil
}
