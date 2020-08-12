package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rancher/apiserver/pkg/writer"
	"github.com/rancher/dynamiclistener/server"
	"github.com/rancher/harvester/pkg/api"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/controller/global"
	"github.com/rancher/harvester/pkg/controller/master"
	"github.com/rancher/harvester/pkg/server/ui"
	"github.com/rancher/harvester/pkg/settings"
	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/steve/pkg/accesscontrol"
	steveserver "github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/ratelimit"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type VMServer struct {
	Context context.Context

	RestConfig    *restclient.Config
	DynamicClient dynamic.Interface
	ClientSet     *kubernetes.Clientset
	ASL           accesscontrol.AccessSetLookup

	steve *steveserver.Server

	Handler http.Handler
}

func New(ctx context.Context, clientConfig clientcmd.ClientConfig) (*VMServer, error) {
	var err error
	server := &VMServer{
		Context: ctx,
	}
	server.RestConfig, err = clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	server.RestConfig.RateLimiter = ratelimit.None

	if err := Wait(ctx, server.RestConfig); err != nil {
		return nil, err
	}

	server.ClientSet, err = kubernetes.NewForConfig(server.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("kubernetes clientset create error: %s", err.Error())
	}

	server.DynamicClient, err = dynamic.NewForConfig(server.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("kubernetes dynamic client create error:%s", err.Error())
	}

	if err := server.generateSteveServer(); err != nil {
		return nil, err
	}

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

func (s *VMServer) Start() error {
	opts := &server.ListenOpts{
		Secrets: s.steve.Controllers.Core.Secret(),
	}
	return s.steve.ListenAndServe(s.Context, config.HTTPSListenPort, config.HTTPListenPort, opts)
}

func (s *VMServer) generateSteveServer() error {
	factory, err := controller.NewSharedControllerFactoryFromConfig(s.RestConfig, Scheme)
	if err != nil {
		return err
	}

	opts := &generic.FactoryOptions{
		SharedControllerFactory: factory,
	}

	var scaled *config.Scaled

	s.Context, scaled, err = config.SetupScaled(s.Context, s.RestConfig, opts)
	if err != nil {
		return err
	}

	steveControllers, err := steveserver.NewController(s.RestConfig, opts)
	if err != nil {
		return err
	}

	s.ASL = accesscontrol.NewAccessStore(s.Context, true, steveControllers.RBAC)

	nextHandler := ui.RegisterAPIUI()

	s.steve = &steveserver.Server{
		Controllers:     steveControllers,
		AccessSetLookup: s.ASL,
		RestConfig:      s.RestConfig,
		AuthMiddleware:  nil,
		Next:            nextHandler,
		Router:          Routes,
		DashboardURL: func() string {
			if settings.UIIndex.Get() == "local" {
				return settings.UIPath.Get()
			}
			return settings.UIIndex.Get()
		},
		StartHooks: []steveserver.StartHook{
			master.Setup,
			global.Setup,
			api.Setup,
		},
		HTMLResponseWriter: writer.HTMLResponseWriter{
			CSSURL:       ui.CSSURLGetter,
			JSURL:        ui.JSURLGetter,
			APIUIVersion: ui.APIUIVersionGetter,
		},
		PostStartHooks: []func() error{
			scaled.Start,
		},
	}

	return nil
}
