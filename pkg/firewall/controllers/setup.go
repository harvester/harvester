package controllers

import (
	"fmt"
	"os"

	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/schemes"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/rancher/wrangler/pkg/start"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io"
	kubevirt "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io"
	"github.com/harvester/harvester/pkg/server"
)

var (
	localSchemeBuilder = runtime.SchemeBuilder{
		harvesterv1.AddToScheme,
		kubevirtv1.AddToScheme,
	}
	AddToScheme = localSchemeBuilder.AddToScheme
	Scheme      = runtime.NewScheme()
)

func init() {
	utilruntime.Must(AddToScheme(Scheme))
	utilruntime.Must(schemes.AddToScheme(Scheme))
}

func NewFirewallHandler(opts *config.CommonOptions) error {
	ctx := signals.SetupSignalContext()
	client, err := server.GetConfig(opts.KubeConfig)
	if err != nil {
		return err
	}

	restConfig, err := client.ClientConfig()
	if err != nil {
		return err
	}

	vmName, ok := os.LookupEnv("HARVESTER_VM_NAME")
	if !ok {
		return fmt.Errorf("unable to find env variable HARVESTER_VM_NAME")
	}

	vmNamespace, ok := os.LookupEnv("HARVESTER_VM_NAMESPACE")
	if !ok {
		return fmt.Errorf("unable to find env variable HARVESTER_VM_NAMESPACE")
	}

	factory, err := controller.NewSharedControllerFactoryFromConfig(restConfig, Scheme)
	if err != nil {
		return err
	}

	// only want to look at objects in the current namespace
	factoryOpts := &generic.FactoryOptions{
		SharedControllerFactory: factory,
		Namespace:               vmNamespace,
	}

	kubevirtFactory, err := kubevirt.NewFactoryFromConfigWithOptions(restConfig, factoryOpts)
	if err != nil {
		return err
	}

	harvesterFactory, err := ctlharvesterv1.NewFactoryFromConfigWithOptions(restConfig, factoryOpts)
	if err != nil {
		return err
	}

	//register handlers
	h := &FirewallHandler{
		ctx:          ctx,
		vmName:       vmName,
		vmNamespace:  vmNamespace,
		vmController: kubevirtFactory.Kubevirt().V1().VirtualMachine(),
		sgController: harvesterFactory.Harvesterhci().V1beta1().SecurityGroup(),
	}

	err = h.Register()
	if err != nil {
		return err
	}

	if err := start.All(ctx, 1, kubevirtFactory, harvesterFactory); err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}
