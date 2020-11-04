package config

import (
	"context"

	dashboardapi "github.com/kubernetes/dashboard/src/app/backend/auth/api"
	"github.com/rancher/lasso/pkg/controller"
	appsv1 "github.com/rancher/wrangler-api/pkg/generated/controllers/apps"
	corev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core"
	rbacv1 "github.com/rancher/wrangler-api/pkg/generated/controllers/rbac"
	storagev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/storage"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/start"
	"k8s.io/client-go/rest"

	"github.com/rancher/harvester/pkg/auth/jwe"
	"github.com/rancher/harvester/pkg/generated/controllers/cdi.kubevirt.io"
	"github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io"
	cniv1 "github.com/rancher/harvester/pkg/generated/controllers/k8s.cni.cncf.io"
	"github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io"
)

type (
	_scaledKey     struct{}
	_managementKey struct{}
)

var (
	Namespace       string
	Threadiness     int
	HTTPListenPort  int
	HTTPSListenPort int

	ImageStorageEndpoint  string
	ImageStorageAccessKey string
	ImageStorageSecretKey string
	SkipAuthentication    bool
)

type Scaled struct {
	ctx               context.Context
	ControllerFactory controller.SharedControllerFactory

	VirtFactory      *kubevirt.Factory
	CDIFactory       *cdi.Factory
	HarvesterFactory *harvester.Factory
	CoreFactory      *corev1.Factory
	AppsFactory      *appsv1.Factory
	RbacFactory      *rbacv1.Factory
	CniFactory       *cniv1.Factory
	starters         []start.Starter

	Management   *Management
	TokenManager dashboardapi.TokenManager
}

type Management struct {
	ctx               context.Context
	ControllerFactory controller.SharedControllerFactory

	VirtFactory      *kubevirt.Factory
	CDIFactory       *cdi.Factory
	HarvesterFactory *harvester.Factory
	CoreFactory      *corev1.Factory
	AppsFactory      *appsv1.Factory
	RbacFactory      *rbacv1.Factory
	StorageFactory   *storagev1.Factory

	starters []start.Starter
}

func SetupScaled(ctx context.Context, restConfig *rest.Config, opts *generic.FactoryOptions) (context.Context, *Scaled, error) {
	scaled := &Scaled{
		ctx: ctx,
	}
	virt, err := kubevirt.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, nil, err
	}
	scaled.VirtFactory = virt
	scaled.starters = append(scaled.starters, virt)

	cdiFactory, err := cdi.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, nil, err
	}
	scaled.CDIFactory = cdiFactory
	scaled.starters = append(scaled.starters, cdiFactory)

	harvesterFactory, err := harvester.NewFactoryFromConfigWithOptions(restConfig, opts)
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

	scaled.Management, err = setupManagement(ctx, restConfig, opts)
	if err != nil {
		return nil, nil, err
	}

	scaled.TokenManager, err = jwe.NewJWETokenManager(scaled.CoreFactory.Core().V1().Secret(), Namespace)
	if err != nil {
		return nil, nil, err
	}
	return context.WithValue(scaled.ctx, _scaledKey{}, scaled), scaled, nil
}

func setupManagement(ctx context.Context, restConfig *rest.Config, opts *generic.FactoryOptions) (*Management, error) {
	management := &Management{
		ctx: ctx,
	}

	virt, err := kubevirt.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.VirtFactory = virt
	management.starters = append(management.starters, virt)

	cdiFactory, err := cdi.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.CDIFactory = cdiFactory
	management.starters = append(management.starters, cdiFactory)

	harv, err := harvester.NewFactoryFromConfigWithOptions(restConfig, opts)
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

	rbac, err := rbacv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.RbacFactory = rbac
	management.starters = append(management.starters, rbac)

	return management, nil
}

func ScaledWithContext(ctx context.Context) *Scaled {
	return ctx.Value(_scaledKey{}).(*Scaled)
}

func (s *Scaled) Start() error {
	return start.All(s.ctx, Threadiness, s.starters...)
}
func (s *Management) Start() error {
	return start.All(s.ctx, Threadiness, s.starters...)
}
