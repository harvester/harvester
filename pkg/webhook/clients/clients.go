package clients

import (
	"context"

	ctlfleetv1 "github.com/rancher/rancher/pkg/generated/controllers/fleet.cattle.io"
	rancherv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io"
	"github.com/rancher/wrangler/v3/pkg/clients"
	ctrlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core"
	storagev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage"
	"github.com/rancher/wrangler/v3/pkg/schemes"
	v1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/client-go/rest"

	ctlclusterv1 "github.com/harvester/harvester/pkg/generated/controllers/cluster.x-k8s.io"
	ctlharvestercorev1 "github.com/harvester/harvester/pkg/generated/controllers/core"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io"
	ctlloggingv1 "github.com/harvester/harvester/pkg/generated/controllers/logging.banzaicloud.io"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io"
	ctlnetwork "github.com/harvester/harvester/pkg/generated/controllers/network.harvesterhci.io"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io"
)

type Clients struct {
	clients.Clients

	HarvesterFactory         *ctlharvesterv1.Factory
	HarvesterCoreFactory     *ctlharvestercorev1.Factory
	KubevirtFactory          *ctlkubevirtv1.Factory
	CNIFactory               *ctlcniv1.Factory
	SnapshotFactory          *ctlsnapshotv1.Factory
	FleetFactory             *ctlfleetv1.Factory
	StorageFactory           *storagev1.Factory
	LonghornFactory          *ctllonghornv1.Factory
	ClusterFactory           *ctlclusterv1.Factory
	RancherManagementFactory *rancherv3.Factory
	CoreFactory              *ctrlcorev1.Factory
	HarvesterNetworkFactory  *ctlnetwork.Factory
	LoggingFactory           *ctlloggingv1.Factory
}

func New(ctx context.Context, rest *rest.Config, threadiness int) (*Clients, error) {
	clients, err := clients.NewFromConfig(rest, nil)
	if err != nil {
		return nil, err
	}

	if err := schemes.Register(v1.AddToScheme); err != nil {
		return nil, err
	}

	harvesterFactory, err := ctlharvesterv1.NewFactoryFromConfigWithOptions(rest, clients.FactoryOptions)
	if err != nil {
		return nil, err
	}

	if err = harvesterFactory.Start(ctx, threadiness); err != nil {
		return nil, err
	}

	harvesterCoreFactory, err := ctlharvestercorev1.NewFactoryFromConfigWithOptions(rest, clients.FactoryOptions)
	if err != nil {
		return nil, err
	}

	if err = harvesterCoreFactory.Start(ctx, threadiness); err != nil {
		return nil, err
	}

	kubevirtFactory, err := ctlkubevirtv1.NewFactoryFromConfigWithOptions(rest, clients.FactoryOptions)
	if err != nil {
		return nil, err
	}

	if err = kubevirtFactory.Start(ctx, threadiness); err != nil {
		return nil, err
	}

	cniFactory, err := ctlcniv1.NewFactoryFromConfigWithOptions(rest, clients.FactoryOptions)
	if err != nil {
		return nil, err
	}

	if err = cniFactory.Start(ctx, threadiness); err != nil {
		return nil, err
	}

	snapshotFactory, err := ctlsnapshotv1.NewFactoryFromConfigWithOptions(rest, clients.FactoryOptions)
	if err != nil {
		return nil, err
	}

	if err = snapshotFactory.Start(ctx, threadiness); err != nil {
		return nil, err
	}

	fleetFactory, err := ctlfleetv1.NewFactoryFromConfigWithOptions(rest, clients.FactoryOptions)
	if err != nil {
		return nil, err
	}

	if err = fleetFactory.Start(ctx, threadiness); err != nil {
		return nil, err
	}

	storageFactory, err := storagev1.NewFactoryFromConfigWithOptions(rest, clients.FactoryOptions)
	if err != nil {
		return nil, err
	}

	if err = storageFactory.Start(ctx, threadiness); err != nil {
		return nil, err
	}

	longhornFactory, err := ctllonghornv1.NewFactoryFromConfigWithOptions(rest, clients.FactoryOptions)
	if err != nil {
		return nil, err
	}

	clusterFactory, err := ctlclusterv1.NewFactoryFromConfigWithOptions(rest, clients.FactoryOptions)
	if err != nil {
		return nil, err
	}

	rancherFactory, err := rancherv3.NewFactoryFromConfigWithOptions(rest, clients.FactoryOptions)
	if err != nil {
		return nil, err
	}

	coreFactory, err := ctrlcorev1.NewFactoryFromConfigWithOptions(rest, clients.FactoryOptions)
	if err != nil {
		return nil, err
	}

	harvesterNetworkFactory, err := ctlnetwork.NewFactoryFromConfigWithOptions(rest, (*ctlnetwork.FactoryOptions)(clients.FactoryOptions))
	if err != nil {
		return nil, err
	}

	if err = harvesterNetworkFactory.Start(ctx, threadiness); err != nil {
		return nil, err
	}

	loggingFactory, err := ctlloggingv1.NewFactoryFromConfigWithOptions(rest, (*ctlloggingv1.FactoryOptions)(clients.FactoryOptions))
	if err != nil {
		return nil, err
	}
	if err = loggingFactory.Start(ctx, threadiness); err != nil {
		return nil, err
	}

	return &Clients{
		Clients:                  *clients,
		HarvesterFactory:         harvesterFactory,
		HarvesterCoreFactory:     harvesterCoreFactory,
		KubevirtFactory:          kubevirtFactory,
		CNIFactory:               cniFactory,
		SnapshotFactory:          snapshotFactory,
		FleetFactory:             fleetFactory,
		StorageFactory:           storageFactory,
		LonghornFactory:          longhornFactory,
		ClusterFactory:           clusterFactory,
		RancherManagementFactory: rancherFactory,
		CoreFactory:              coreFactory,
		HarvesterNetworkFactory:  harvesterNetworkFactory,
		LoggingFactory:           loggingFactory,
	}, nil
}
