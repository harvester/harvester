package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rancher/dynamiclistener"
	"github.com/rancher/dynamiclistener/server"
	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/steve/pkg/accesscontrol"
	steveauth "github.com/rancher/steve/pkg/auth"
	steveserver "github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/ratelimit"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/harvester/harvester/pkg/api"
	"github.com/harvester/harvester/pkg/api/auth"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/controller/crds"
	"github.com/harvester/harvester/pkg/controller/global"
	"github.com/harvester/harvester/pkg/controller/master"
	"github.com/harvester/harvester/pkg/server/ui"
)

type HarvesterServer struct {
	Context context.Context

	RESTConfig    *restclient.Config
	DynamicClient dynamic.Interface
	ClientSet     *kubernetes.Clientset
	ASL           accesscontrol.AccessSetLookup

	steve          *steveserver.Server
	controllers    *steveserver.Controllers
	startHooks     []StartHook
	postStartHooks []PostStartHook

	Handler http.Handler
}

func New(ctx context.Context, clientConfig clientcmd.ClientConfig, options config.Options) (*HarvesterServer, error) {
	var err error
	server := &HarvesterServer{
		Context: ctx,
	}
	server.RESTConfig, err = clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	server.RESTConfig.RateLimiter = ratelimit.None

	if err := Wait(ctx, server.RESTConfig); err != nil {
		return nil, err
	}

	server.ClientSet, err = kubernetes.NewForConfig(server.RESTConfig)
	if err != nil {
		return nil, fmt.Errorf("kubernetes clientset create error: %s", err.Error())
	}

	server.DynamicClient, err = dynamic.NewForConfig(server.RESTConfig)
	if err != nil {
		return nil, fmt.Errorf("kubernetes dynamic client create error:%s", err.Error())
	}

	if err := server.generateSteveServer(options); err != nil {
		return nil, err
	}

	ui.ConfigureAPIUI(server.steve.APIServer)

	return server, nil
}

func Wait(ctx context.Context, config *rest.Config) error {
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	for {
		_, err := client.Discovery().ServerVersion()
		if err == nil {
			break
		}
		logrus.Infof("Waiting for server to become available: %v", err)
		select {
		case <-ctx.Done():
			return fmt.Errorf("startup canceled")
		case <-time.After(2 * time.Second):
		}
	}

	return nil
}

func (s *HarvesterServer) ListenAndServe(listenerCfg *dynamiclistener.Config, opts config.Options) error {
	listenOpts := &server.ListenOpts{
		Secrets: s.controllers.Core.Secret(),
		TLSListenerConfig: dynamiclistener.Config{
			CloseConnOnCertChange: true,
		},
	}

	if listenerCfg != nil {
		listenOpts.TLSListenerConfig = *listenerCfg
	}

	return s.steve.ListenAndServe(s.Context, opts.HTTPSListenPort, opts.HTTPListenPort, listenOpts)
}

// Scaled returns the *config.Scaled,
// it should call after Start.
func (s *HarvesterServer) Scaled() *config.Scaled {
	return config.ScaledWithContext(s.Context)
}

func (s *HarvesterServer) generateSteveServer(options config.Options) error {
	factory, err := controller.NewSharedControllerFactoryFromConfig(s.RESTConfig, Scheme)
	if err != nil {
		return err
	}

	factoryOpts := &generic.FactoryOptions{
		SharedControllerFactory: factory,
	}

	var scaled *config.Scaled

	s.Context, scaled, err = config.SetupScaled(s.Context, s.RESTConfig, factoryOpts, options.Namespace)
	if err != nil {
		return err
	}

	s.controllers, err = steveserver.NewController(s.RESTConfig, factoryOpts)
	if err != nil {
		return err
	}

	s.ASL = accesscontrol.NewAccessStore(s.Context, true, s.controllers.RBAC)

	router, err := NewRouter(scaled, s.RESTConfig, options)
	if err != nil {
		return err
	}

	if err := crds.Setup(s.Context, s.RESTConfig); err != nil {
		return err
	}

	var authMiddleware steveauth.Middleware
	if !options.SkipAuthentication {
		md, err := auth.NewMiddleware(s.Context, scaled, s.RESTConfig, options.RancherEmbedded)
		if err != nil {
			return err
		}
		authMiddleware = md.ToAuthMiddleware()
	}

	s.steve, err = steveserver.New(s.Context, s.RESTConfig, &steveserver.Options{
		Controllers:     s.controllers,
		AuthMiddleware:  authMiddleware,
		Router:          router.Routes,
		AccessSetLookup: s.ASL,
	})
	if err != nil {
		return err
	}

	s.startHooks = []StartHook{
		master.Setup,
		global.Setup,
		api.Setup,
	}

	s.postStartHooks = []PostStartHook{
		scaled.Start,
	}

	return s.start(options)
}

func (s *HarvesterServer) start(options config.Options) error {
	for _, hook := range s.startHooks {
		if err := hook(s.Context, s.steve, s.controllers, options); err != nil {
			return err
		}
	}

	if err := s.controllers.Start(s.Context); err != nil {
		return err
	}

	for _, hook := range s.postStartHooks {
		if err := hook(options.Threadiness); err != nil {
			return err
		}
	}

	return nil
}

type StartHook func(context.Context, *steveserver.Server, *steveserver.Controllers, config.Options) error
type PostStartHook func(int) error
