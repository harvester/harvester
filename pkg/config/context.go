package config

import (
	"context"

	helmv1 "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io"
	dashboardapi "github.com/kubernetes/dashboard/src/app/backend/auth/api"
	"github.com/rancher/lasso/pkg/controller"
	rancherv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io"
	provisioningv1 "github.com/rancher/rancher/pkg/generated/controllers/provisioning.cattle.io"
	"github.com/rancher/wrangler/pkg/apply"
	appsv1 "github.com/rancher/wrangler/pkg/generated/controllers/apps"
	batchv1 "github.com/rancher/wrangler/pkg/generated/controllers/batch"
	corev1 "github.com/rancher/wrangler/pkg/generated/controllers/core"
	rbacv1 "github.com/rancher/wrangler/pkg/generated/controllers/rbac"
	storagev1 "github.com/rancher/wrangler/pkg/generated/controllers/storage"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/start"
	"github.com/sirupsen/logrus"
	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/scheme"
	"github.com/harvester/harvester/pkg/generated/controllers/cluster.x-k8s.io"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io"
	cniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io"
	"github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io"
	longhornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io"
	"github.com/harvester/harvester/pkg/generated/controllers/networking.k8s.io"
	snapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io"
	"github.com/harvester/harvester/pkg/generated/controllers/upgrade.cattle.io"
)

type (
	_scaledKey struct{}
)

type Options struct {
	Namespace       string
	Threadiness     int
	HTTPListenPort  int
	HTTPSListenPort int

	RancherEmbedded bool
	RancherURL      string
	HCIMode         bool
}

type Scaled struct {
	Ctx               context.Context
	ControllerFactory controller.SharedControllerFactory

	VirtFactory              *kubevirt.Factory
	HarvesterFactory         *ctlharvesterv1.Factory
	CoreFactory              *corev1.Factory
	AppsFactory              *appsv1.Factory
	BatchFactory             *batchv1.Factory
	RbacFactory              *rbacv1.Factory
	CniFactory               *cniv1.Factory
	SnapshotFactory          *snapshotv1.Factory
	LonghornFactory          *longhornv1.Factory
	RancherManagementFactory *rancherv3.Factory
	starters                 []start.Starter

	Management   *Management
	TokenManager dashboardapi.TokenManager
}

type Management struct {
	ctx               context.Context
	Apply             apply.Apply
	ControllerFactory controller.SharedControllerFactory

	VirtFactory              *kubevirt.Factory
	HarvesterFactory         *ctlharvesterv1.Factory
	CoreFactory              *corev1.Factory
	AppsFactory              *appsv1.Factory
	BatchFactory             *batchv1.Factory
	RbacFactory              *rbacv1.Factory
	StorageFactory           *storagev1.Factory
	SnapshotFactory          *snapshotv1.Factory
	LonghornFactory          *longhornv1.Factory
	ProvisioningFactory      *provisioningv1.Factory
	RancherManagementFactory *rancherv3.Factory
	HelmFactory              *helmv1.Factory

	NetworkingFactory *networking.Factory
	UpgradeFactory    *upgrade.Factory
	ClusterFactory    *cluster.Factory

	ClientSet  *kubernetes.Clientset
	RestConfig *rest.Config

	starters []start.Starter
}

func SetupScaled(ctx context.Context, restConfig *rest.Config, opts *generic.FactoryOptions, namespace string) (context.Context, *Scaled, error) {
	scaled := &Scaled{
		Ctx: ctx,
	}

	virt, err := kubevirt.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, nil, err
	}
	scaled.VirtFactory = virt
	scaled.starters = append(scaled.starters, virt)

	harvesterFactory, err := ctlharvesterv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, nil, err
	}
	scaled.HarvesterFactory = harvesterFactory
	scaled.starters = append(scaled.starters, harvesterFactory)

	core, err := corev1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, nil, err
	}
	scaled.CoreFactory = core
	scaled.starters = append(scaled.starters, core)

	apps, err := appsv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, nil, err
	}
	scaled.AppsFactory = apps
	scaled.starters = append(scaled.starters, apps)

	batch, err := batchv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, nil, err
	}
	scaled.BatchFactory = batch
	scaled.starters = append(scaled.starters, batch)

	rbac, err := rbacv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, nil, err
	}
	scaled.RbacFactory = rbac
	scaled.starters = append(scaled.starters, rbac)

	cni, err := cniv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, nil, err
	}
	scaled.CniFactory = cni
	scaled.starters = append(scaled.starters, cni)

	snapshot, err := snapshotv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, nil, err
	}
	scaled.SnapshotFactory = snapshot
	scaled.starters = append(scaled.starters, snapshot)

	longhorn, err := longhornv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, nil, err
	}
	scaled.LonghornFactory = longhorn
	scaled.starters = append(scaled.starters, longhorn)

	rancher, err := rancherv3.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, nil, err
	}
	scaled.RancherManagementFactory = rancher
	scaled.starters = append(scaled.starters, rancher)

	scaled.Management, err = setupManagement(ctx, restConfig, opts)
	if err != nil {
		return nil, nil, err
	}

	return context.WithValue(scaled.Ctx, _scaledKey{}, scaled), scaled, nil
}

func setupManagement(ctx context.Context, restConfig *rest.Config, opts *generic.FactoryOptions) (*Management, error) {
	management := &Management{
		ctx: ctx,
	}

	apply, err := apply.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	management.Apply = apply

	virt, err := kubevirt.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.VirtFactory = virt
	management.starters = append(management.starters, virt)

	harv, err := ctlharvesterv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.HarvesterFactory = harv
	management.starters = append(management.starters, harv)

	core, err := corev1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.CoreFactory = core
	management.starters = append(management.starters, core)

	apps, err := appsv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.AppsFactory = apps
	management.starters = append(management.starters, apps)

	batch, err := batchv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.BatchFactory = batch
	management.starters = append(management.starters, batch)

	rbac, err := rbacv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.RbacFactory = rbac
	management.starters = append(management.starters, rbac)

	upgrade, err := upgrade.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.UpgradeFactory = upgrade
	management.starters = append(management.starters, upgrade)

	storage, err := storagev1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.StorageFactory = storage
	management.starters = append(management.starters, storage)

	longhorn, err := longhornv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.LonghornFactory = longhorn
	management.starters = append(management.starters, longhorn)

	snapshot, err := snapshotv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.SnapshotFactory = snapshot
	management.starters = append(management.starters, snapshot)

	provisioning, err := provisioningv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.ProvisioningFactory = provisioning
	management.starters = append(management.starters, provisioning)

	helm, err := helmv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.HelmFactory = helm
	management.starters = append(management.starters, helm)

	networking, err := networking.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.NetworkingFactory = networking
	management.starters = append(management.starters, networking)

	rancher, err := rancherv3.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.RancherManagementFactory = rancher
	management.starters = append(management.starters, rancher)

	cluster, err := cluster.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.ClusterFactory = cluster
	management.starters = append(management.starters, cluster)

	management.RestConfig = restConfig
	management.ClientSet, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	return management, nil
}

func ScaledWithContext(ctx context.Context) *Scaled {
	return ctx.Value(_scaledKey{}).(*Scaled)
}

func (s *Scaled) Start(threadiness int) error {
	return start.All(s.Ctx, threadiness, s.starters...)
}
func (s *Management) Start(threadiness int) error {
	return start.All(s.ctx, threadiness, s.starters...)
}

func (s *Management) NewRecorder(componentName, namespace, nodeName string) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: s.ClientSet.CoreV1().Events(namespace)})
	return eventBroadcaster.NewRecorder(scheme.Scheme, k8sv1.EventSource{Component: componentName, Host: nodeName})
}
