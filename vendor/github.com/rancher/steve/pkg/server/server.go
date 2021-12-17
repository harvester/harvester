package server

import (
	"context"
	"errors"
	"net/http"

	apiserver "github.com/rancher/apiserver/pkg/server"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/dynamiclistener/server"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/aggregation"
	"github.com/rancher/steve/pkg/auth"
	"github.com/rancher/steve/pkg/client"
	"github.com/rancher/steve/pkg/clustercache"
	schemacontroller "github.com/rancher/steve/pkg/controllers/schema"
	"github.com/rancher/steve/pkg/resources"
	"github.com/rancher/steve/pkg/resources/common"
	"github.com/rancher/steve/pkg/resources/schemas"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server/handler"
	"github.com/rancher/steve/pkg/server/router"
	"github.com/rancher/steve/pkg/summarycache"
	"k8s.io/client-go/rest"
)

var ErrConfigRequired = errors.New("rest config is required")

type Server struct {
	http.Handler

	ClientFactory   *client.Factory
	ClusterCache    clustercache.ClusterCache
	SchemaFactory   schema.Factory
	RESTConfig      *rest.Config
	BaseSchemas     *types.APISchemas
	AccessSetLookup accesscontrol.AccessSetLookup
	APIServer       *apiserver.Server
	ClusterRegistry string
	Version         string

	authMiddleware      auth.Middleware
	controllers         *Controllers
	needControllerStart bool
	next                http.Handler
	router              router.RouterFunc

	aggregationSecretNamespace string
	aggregationSecretName      string
}

type Options struct {
	// Controllers If the controllers are passed in the caller must also start the controllers
	Controllers                *Controllers
	ClientFactory              *client.Factory
	AccessSetLookup            accesscontrol.AccessSetLookup
	AuthMiddleware             auth.Middleware
	Next                       http.Handler
	Router                     router.RouterFunc
	AggregationSecretNamespace string
	AggregationSecretName      string
	ClusterRegistry            string
	ServerVersion              string
}

func New(ctx context.Context, restConfig *rest.Config, opts *Options) (*Server, error) {
	if opts == nil {
		opts = &Options{}
	}

	server := &Server{
		RESTConfig:                 restConfig,
		ClientFactory:              opts.ClientFactory,
		AccessSetLookup:            opts.AccessSetLookup,
		authMiddleware:             opts.AuthMiddleware,
		controllers:                opts.Controllers,
		next:                       opts.Next,
		router:                     opts.Router,
		aggregationSecretNamespace: opts.AggregationSecretNamespace,
		aggregationSecretName:      opts.AggregationSecretName,
		ClusterRegistry:            opts.ClusterRegistry,
		Version:                    opts.ServerVersion,
	}

	if err := setup(ctx, server); err != nil {
		return nil, err
	}

	return server, server.start(ctx)
}

func setDefaults(server *Server) error {
	if server.RESTConfig == nil {
		return ErrConfigRequired
	}

	if server.controllers == nil {
		var err error
		server.controllers, err = NewController(server.RESTConfig, nil)
		server.needControllerStart = true
		if err != nil {
			return err
		}
	}

	if server.next == nil {
		server.next = http.NotFoundHandler()
	}

	if server.BaseSchemas == nil {
		server.BaseSchemas = types.EmptyAPISchemas()
	}

	return nil
}

func setup(ctx context.Context, server *Server) error {
	err := setDefaults(server)
	if err != nil {
		return err
	}

	cf := server.ClientFactory
	if cf == nil {
		cf, err = client.NewFactory(server.RESTConfig, server.authMiddleware != nil)
		if err != nil {
			return err
		}
		server.ClientFactory = cf
	}

	asl := server.AccessSetLookup
	if asl == nil {
		asl = accesscontrol.NewAccessStore(ctx, true, server.controllers.RBAC)
	}

	ccache := clustercache.NewClusterCache(ctx, cf.AdminDynamicClient())
	server.ClusterCache = ccache
	sf := schema.NewCollection(ctx, server.BaseSchemas, asl)

	if err = resources.DefaultSchemas(ctx, server.BaseSchemas, ccache, server.ClientFactory, sf, server.Version); err != nil {
		return err
	}

	summaryCache := summarycache.New(sf, ccache)
	summaryCache.Start(ctx)

	for _, template := range resources.DefaultSchemaTemplates(cf, server.BaseSchemas, summaryCache, asl, server.controllers.K8s.Discovery()) {
		sf.AddTemplate(template)
	}

	cols, err := common.NewDynamicColumns(server.RESTConfig)
	if err != nil {
		return err
	}

	schemas.SetupWatcher(ctx, server.BaseSchemas, asl, sf)

	schemacontroller.Register(ctx,
		cols,
		server.controllers.K8s.Discovery(),
		server.controllers.CRD.CustomResourceDefinition(),
		server.controllers.API.APIService(),
		server.controllers.K8s.AuthorizationV1().SelfSubjectAccessReviews(),
		ccache,
		sf)

	apiServer, handler, err := handler.New(server.RESTConfig, sf, server.authMiddleware, server.next, server.router)
	if err != nil {
		return err
	}

	server.APIServer = apiServer
	server.Handler = handler
	server.SchemaFactory = sf
	return nil
}

func (c *Server) start(ctx context.Context) error {
	if c.needControllerStart {
		if err := c.controllers.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (c *Server) StartAggregation(ctx context.Context) {
	aggregation.Watch(ctx, c.controllers.Core.Secret(), c.aggregationSecretNamespace,
		c.aggregationSecretName, c)
}

func (c *Server) ListenAndServe(ctx context.Context, httpsPort, httpPort int, opts *server.ListenOpts) error {
	if opts == nil {
		opts = &server.ListenOpts{}
	}
	if opts.Storage == nil && opts.Secrets == nil {
		opts.Secrets = c.controllers.Core.Secret()
	}

	c.StartAggregation(ctx)

	if err := server.ListenAndServe(ctx, httpsPort, httpPort, c, opts); err != nil {
		return err
	}

	<-ctx.Done()
	return ctx.Err()
}
