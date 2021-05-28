package clients

import (
	"context"
	"time"

	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/lasso/pkg/dynamic"
	"github.com/rancher/wrangler/pkg/apply"
	admissionreg "github.com/rancher/wrangler/pkg/generated/controllers/admissionregistration.k8s.io"
	admissionregcontrollers "github.com/rancher/wrangler/pkg/generated/controllers/admissionregistration.k8s.io/v1"
	"github.com/rancher/wrangler/pkg/generated/controllers/apiextensions.k8s.io"
	crdcontrollers "github.com/rancher/wrangler/pkg/generated/controllers/apiextensions.k8s.io/v1beta1"
	"github.com/rancher/wrangler/pkg/generated/controllers/apiregistration.k8s.io"
	apicontrollers "github.com/rancher/wrangler/pkg/generated/controllers/apiregistration.k8s.io/v1"
	"github.com/rancher/wrangler/pkg/generated/controllers/apps"
	appcontrollers "github.com/rancher/wrangler/pkg/generated/controllers/apps/v1"
	"github.com/rancher/wrangler/pkg/generated/controllers/batch"
	batchcontrollers "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	"github.com/rancher/wrangler/pkg/generated/controllers/core"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/generated/controllers/rbac"
	rbaccontrollers "github.com/rancher/wrangler/pkg/generated/controllers/rbac/v1"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/ratelimit"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

type Clients struct {
	K8s       kubernetes.Interface
	Core      corecontrollers.Interface
	RBAC      rbaccontrollers.Interface
	Apps      appcontrollers.Interface
	CRD       crdcontrollers.Interface
	API       apicontrollers.Interface
	Admission admissionregcontrollers.Interface
	Batch     batchcontrollers.Interface
	Apply     apply.Apply

	Dynamic                 *dynamic.Controller
	ClientConfig            clientcmd.ClientConfig
	RESTConfig              *rest.Config
	CachedDiscovery         discovery.CachedDiscoveryInterface
	SharedControllerFactory controller.SharedControllerFactory
	RESTMapper              meta.RESTMapper
	FactoryOptions          *generic.FactoryOptions
}

func ensureSharedFactory(cfg *rest.Config, opts *generic.FactoryOptions) (*generic.FactoryOptions, error) {
	if opts == nil {
		opts = &generic.FactoryOptions{}
	}

	copy := *opts
	factory, err := generic.NewFactoryFromConfigWithOptions(cfg, opts)
	if err != nil {
		return nil, err
	}

	copy.SharedControllerFactory = factory.ControllerFactory()
	copy.SharedCacheFactory = factory.ControllerFactory().SharedCacheFactory()
	return &copy, nil
}

func New(clientConfig clientcmd.ClientConfig, opts *generic.FactoryOptions) (*Clients, error) {
	cfg, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	clients, err := NewFromConfig(cfg, opts)
	if err != nil {
		return nil, err
	}
	clients.ClientConfig = clientConfig

	return clients, nil
}

func NewFromConfig(cfg *rest.Config, opts *generic.FactoryOptions) (*Clients, error) {
	cfg = restConfigDefaults(cfg)

	opts, err := ensureSharedFactory(cfg, opts)
	if err != nil {
		return nil, err
	}

	core, err := core.NewFactoryFromConfigWithOptions(cfg, opts)
	if err != nil {
		return nil, err
	}

	rbac, err := rbac.NewFactoryFromConfigWithOptions(cfg, opts)
	if err != nil {
		return nil, err
	}

	apps, err := apps.NewFactoryFromConfigWithOptions(cfg, opts)
	if err != nil {
		return nil, err
	}

	api, err := apiregistration.NewFactoryFromConfigWithOptions(cfg, opts)
	if err != nil {
		return nil, err
	}

	crd, err := apiextensions.NewFactoryFromConfigWithOptions(cfg, opts)
	if err != nil {
		return nil, err
	}

	adminReg, err := admissionreg.NewFactoryFromConfigWithOptions(cfg, opts)
	if err != nil {
		return nil, err
	}

	k8s, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	batch, err := batch.NewFactoryFromConfigWithOptions(cfg, opts)
	if err != nil {
		return nil, err
	}

	cache := memory.NewMemCacheClient(k8s.Discovery())
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(cache)

	apply, err := apply.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &Clients{
		K8s:                     k8s,
		Core:                    core.Core().V1(),
		RBAC:                    rbac.Rbac().V1(),
		Apps:                    apps.Apps().V1(),
		CRD:                     crd.Apiextensions().V1beta1(),
		API:                     api.Apiregistration().V1(),
		Admission:               adminReg.Admissionregistration().V1(),
		Batch:                   batch.Batch().V1(),
		Apply:                   apply.WithSetOwnerReference(false, false),
		RESTConfig:              cfg,
		CachedDiscovery:         cache,
		SharedControllerFactory: opts.SharedControllerFactory,
		RESTMapper:              restMapper,
		FactoryOptions:          opts,
		Dynamic:                 dynamic.New(k8s.Discovery()),
	}, nil
}

func (c *Clients) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return c.ClientConfig
}

func (c *Clients) ToRESTConfig() (*rest.Config, error) {
	return c.RESTConfig, nil
}

func (c *Clients) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return c.CachedDiscovery, nil
}

func (c *Clients) ToRESTMapper() (meta.RESTMapper, error) {
	return c.RESTMapper, nil
}

func (c *Clients) Start(ctx context.Context) error {
	if err := c.Dynamic.Register(ctx, c.SharedControllerFactory); err != nil {
		return err
	}
	return c.SharedControllerFactory.Start(ctx, 5)
}

func restConfigDefaults(cfg *rest.Config) *rest.Config {
	cfg = rest.CopyConfig(cfg)
	cfg.Timeout = 15 * time.Minute
	cfg.RateLimiter = ratelimit.None

	return cfg
}
