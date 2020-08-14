package config

import (
	"context"

	kubevirt "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io"
	vm "github.com/rancher/harvester/pkg/generated/controllers/vm.cattle.io"
	"github.com/rancher/lasso/pkg/controller"
	corev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core"
	storagev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/storage"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/start"
	"k8s.io/client-go/rest"
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
)

type Scaled struct {
	ctx               context.Context
	ControllerFactory controller.SharedControllerFactory

	VirtFactory *kubevirt.Factory
	VMFactory   *vm.Factory
	CoreFactory *corev1.Factory

	starters []start.Starter

	Management *Management
}

type Management struct {
	ctx               context.Context
	ControllerFactory controller.SharedControllerFactory

	VirtFactory    *kubevirt.Factory
	VMFactory      *vm.Factory
	CoreFactory    *corev1.Factory
	StorageFactory *storagev1.Factory

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

	vm, err := vm.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, nil, err
	}
	scaled.VMFactory = vm
	scaled.starters = append(scaled.starters, vm)

	core, err := corev1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, nil, err
	}
	scaled.CoreFactory = core
	scaled.starters = append(scaled.starters, core)

	scaled.Management, err = setupManagement(ctx, restConfig, opts)
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

	vm, err := vm.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.VMFactory = vm
	management.starters = append(management.starters, vm)

	core, err := corev1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.CoreFactory = core
	management.starters = append(management.starters, core)

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
