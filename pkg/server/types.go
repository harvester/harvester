package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rancher/apiserver/pkg/parse"
	"github.com/rancher/apiserver/pkg/store/apiroot"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/apiserver/pkg/urlbuilder"
	"github.com/rancher/dynamiclistener"
	"github.com/rancher/dynamiclistener/server"
	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/aggregation"
	steveauth "github.com/rancher/steve/pkg/auth"
	steveserver "github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/v3/pkg/generic"
	"github.com/rancher/wrangler/v3/pkg/ratelimit"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/harvester/harvester/pkg/api"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/controller/admission"
	"github.com/harvester/harvester/pkg/controller/global"
	"github.com/harvester/harvester/pkg/controller/master"
	"github.com/harvester/harvester/pkg/data"
	"github.com/harvester/harvester/pkg/indexeres"
	"github.com/harvester/harvester/pkg/server/ui"
)

type HarvesterServer struct {
	Context context.Context

	RancherRESTConfig *rest.Config

	RESTConfig    *rest.Config
	DynamicClient dynamic.Interface
	ClientSet     *kubernetes.Clientset
	ASL           accesscontrol.AccessSetLookup

	steve          *steveserver.Server
	controllers    *steveserver.Controllers
	startHooks     []StartHook
	postStartHooks []PostStartHook

	Handler http.Handler
}

var (
	whiteListedCiphers = []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	}
)

const (
	RancherKubeConfigSecretName = "rancher-kubeconfig"
	RancherKubeConfigSecretKey  = "kubernetes.kubeconfig"
)

func RancherRESTConfig(ctx context.Context, restConfig *rest.Config, options config.Options) (*rest.Config, error) {
	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	secret, err := clientSet.CoreV1().Secrets(options.Namespace).Get(ctx, RancherKubeConfigSecretName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return restConfig, nil
		}
		return nil, err
	}

	rancherClientConfig, err := clientcmd.NewClientConfigFromBytes(secret.Data[RancherKubeConfigSecretKey])
	if err != nil {
		return nil, err
	}

	return rancherClientConfig.ClientConfig()
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

	server.RancherRESTConfig, err = RancherRESTConfig(ctx, server.RESTConfig, options)
	if err != nil {
		return nil, err
	}

	server.RancherRESTConfig.RateLimiter = ratelimit.None
	if err := Wait(ctx, server.RancherRESTConfig); err != nil {
		return nil, err
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
			TLSConfig: &tls.Config{
				MinVersion:   tls.VersionTLS12,
				CipherSuites: whiteListedCiphers,
			},
		},
	}

	if listenerCfg != nil {
		listenOpts.TLSListenerConfig = *listenerCfg
	}

	s.steve.StartAggregation(s.Context)
	s.startAggregation(opts)

	if err := server.ListenAndServe(s.Context, opts.HTTPSListenPort, opts.HTTPListenPort, s.Handler, listenOpts); err != nil {
		return err
	}

	<-s.Context.Done()
	return s.Context.Err()
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

	s.Context, scaled, err = config.SetupScaled(s.Context, s.RESTConfig, factoryOpts)
	if err != nil {
		return err
	}

	s.controllers, err = steveserver.NewController(s.RESTConfig, factoryOpts)
	if err != nil {
		return err
	}

	s.ASL = accesscontrol.NewAccessStore(s.Context, true, s.controllers.RBAC)

	if err := data.Init(s.Context, scaled.Management, options); err != nil {
		return err
	}

	// Once the controller starts its works, the controller might manipulate resources.
	// Make sure admission webhook server is ready before that.
	if err := admission.Wait(s.Context, s.ClientSet); err != nil {
		return err
	}

	router, err := NewRouter(scaled, s.RESTConfig, options)
	if err != nil {
		return err
	}

	s.steve, err = steveserver.New(s.Context, s.RESTConfig, &steveserver.Options{
		Controllers:     s.controllers,
		AuthMiddleware:  steveauth.ExistingContext,
		Router:          router.Routes,
		AccessSetLookup: s.ASL,
	})
	if err != nil {
		return err
	}

	// Serve APIs under /v1/harvester as the Rancher aggregation services, in addition to the default /v1
	s.steve.APIServer.Parser = dynamicURLBuilderParse
	apiroot.Register(s.steve.BaseSchemas, []string{"v1", "v1/harvester"}, "proxy:/apis")

	authMiddleware := steveauth.ToMiddleware(steveauth.AuthenticatorFunc(steveauth.AlwaysAdmin))
	s.Handler = authMiddleware(s.steve)

	s.startHooks = []StartHook{
		indexeres.Setup,
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

func (s *HarvesterServer) startAggregation(options config.Options) {
	aggregation.Watch(s.Context,
		s.Scaled().Management.CoreFactory.Core().V1().Secret(),
		options.Namespace,
		data.AggregationSecretName,
		s.steve)
}

type StartHook func(context.Context, *steveserver.Server, *steveserver.Controllers, config.Options) error
type PostStartHook func(int) error

// dynamicURLBuilderParse sets the urlBuilder according to the request path instead of the fixed default in
// https://github.com/rancher/steve/blob/52f86dce9bd4b2c28eb190711cfc594b0f5a7dbb/pkg/server/handler/apiserver.go#L74
func dynamicURLBuilderParse(apiOp *types.APIRequest, urlParser parse.URLParser) error {
	var err error
	if strings.HasPrefix(apiOp.Request.URL.Path, "/v1/harvester/") {
		apiOp.URLBuilder, err = urlbuilder.NewPrefixed(apiOp.Request, apiOp.Schemas, "v1/harvester")
		if err != nil {
			return err
		}
	}
	return parse.Parse(apiOp, urlParser)
}
