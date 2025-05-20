package ext

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/apis/apiserver"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/authenticatorfactory"
	"k8s.io/apiserver/pkg/authentication/request/headerrequest"
	authenticatorunion "k8s.io/apiserver/pkg/authentication/request/union"
	"k8s.io/apiserver/pkg/server/dynamiccertificates"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/kubernetes"
)

var _ dynamiccertificates.ControllerRunner = &UnionAuthenticator{}
var _ dynamiccertificates.CAContentProvider = &UnionAuthenticator{}

// UnionAuthenticator chains authenticators together to allow many ways of authenticating
// requests for the extension API server. For example, we might want to use Rancher's
// token authentication and fallback to the default authentication (mTLS) defined
// by Kubernetes.
//
// UnionAuthenticator is both a [dynamiccertificates.ControllerRunner] and a
// [dynamiccertificates.CAContentProvider].
type UnionAuthenticator struct {
	unionAuthenticator     authenticator.Request
	unionCAContentProvider dynamiccertificates.CAContentProvider
}

// NewUnionAuthenticator creates a [UnionAuthenticator].
//
// The authenticators will be tried one by one, in the order they are given, until
// one succeed or all fails.
//
// Here's an example usage:
//
//	customAuth := authenticator.RequestFunc(func(req *http.Request) (*Response, bool, error) {
//		// use request to determine what the user is, otherwise return false
//	})
//	default, err := NewDefaultAuthenticator(client)
//	if err != nil {
//		return err
//	}
//	auth := NewUnionAuthenticator(customAuth, default)
//	err = auth.RunOnce(ctx)
func NewUnionAuthenticator(authenticators ...authenticator.Request) *UnionAuthenticator {
	caContentProviders := make([]dynamiccertificates.CAContentProvider, 0, len(authenticators))
	for _, auth := range authenticators {
		auth, ok := auth.(dynamiccertificates.CAContentProvider)
		if !ok {
			continue
		}
		caContentProviders = append(caContentProviders, auth)
	}
	return &UnionAuthenticator{
		unionAuthenticator:     authenticatorunion.New(authenticators...),
		unionCAContentProvider: dynamiccertificates.NewUnionCAContentProvider(caContentProviders...),
	}
}

// AuthenticateRequest implements [authenticator.Request]
func (u *UnionAuthenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	return u.unionAuthenticator.AuthenticateRequest(req)
}

// AuthenticateRequest implements [dynamiccertificates.Notifier]
// This is part of the [dynamiccertificates.CAContentProvider] interface.
func (u *UnionAuthenticator) AddListener(listener dynamiccertificates.Listener) {
	u.unionCAContentProvider.AddListener(listener)
}

// AuthenticateRequest implements [dynamiccertificates.CAContentProvider]
func (u *UnionAuthenticator) Name() string {
	return u.unionCAContentProvider.Name()
}

// AuthenticateRequest implements [dynamiccertificates.CAContentProvider]
func (u *UnionAuthenticator) CurrentCABundleContent() []byte {
	return u.unionCAContentProvider.CurrentCABundleContent()
}

// AuthenticateRequest implements [dynamiccertificates.CAContentProvider]
func (u *UnionAuthenticator) VerifyOptions() (x509.VerifyOptions, bool) {
	return u.unionCAContentProvider.VerifyOptions()
}

// AuthenticateRequest implements [dynamiccertificates.CAContentProvider]
func (u *UnionAuthenticator) RunOnce(ctx context.Context) error {
	runner, ok := u.unionCAContentProvider.(dynamiccertificates.ControllerRunner)
	if !ok {
		return nil
	}
	return runner.RunOnce(ctx)
}

// AuthenticateRequest implements [dynamiccertificates.CAContentProvider]
func (u *UnionAuthenticator) Run(ctx context.Context, workers int) {
	runner, ok := u.unionCAContentProvider.(dynamiccertificates.ControllerRunner)
	if !ok {
		return
	}

	runner.Run(ctx, workers)
}

const (
	authenticationConfigMapNamespace = metav1.NamespaceSystem
	authenticationConfigMapName      = "extension-apiserver-authentication"
)

var _ dynamiccertificates.ControllerRunner = &DefaultAuthenticator{}
var _ dynamiccertificates.CAContentProvider = &DefaultAuthenticator{}

// DefaultAuthenticator is an [authenticator.Request] that authenticates a user by:
//   - making sure the client uses a certificate signed by the CA defined in the
//     `extension-apiserver-authentication` configmap in the `kube-system` namespace and
//   - making sure the CN of the cert is part of the allow list, also defined in the same configmap
//
// This authentication is better explained in https://kubernetes.io/docs/tasks/extend-kubernetes/configure-aggregation-layer/
//
// This authenticator is a [dynamiccertificates.ControllerRunner] which means
// it will run in the background to dynamically watch the content of the configmap.
//
// When using the DefaultAuthenticator, it is suggested to call RunOnce() to initialize
// the CA state. It is also possible to watch for changes to the CA bundle with the AddListener()
// method.  Here's an example usage:
//
//	auth, err := NewDefaultAuthenticator(client)
//	if err != nil {
//	   return err
//	}
//	auth.AddListener(myListener{auth: auth}) // myListener should react to CA bundle changes
//	err = auth.RunOnce(ctx)
type DefaultAuthenticator struct {
	requestHeaderConfig *authenticatorfactory.RequestHeaderConfig
	authenticator       authenticator.Request
}

// NewDefaultAuthenticator creates a DefaultAuthenticator
func NewDefaultAuthenticator(client kubernetes.Interface) (*DefaultAuthenticator, error) {
	requestHeaderConfig, err := createRequestHeaderConfig(client)
	if err != nil {
		return nil, fmt.Errorf("requestheaderconfig: %w", err)
	}

	cfg := authenticatorfactory.DelegatingAuthenticatorConfig{
		Anonymous: &apiserver.AnonymousAuthConfig{
			Enabled: false,
		},
		RequestHeaderConfig: requestHeaderConfig,
	}

	authenticator, _, err := cfg.New()
	if err != nil {
		return nil, err
	}

	return &DefaultAuthenticator{
		requestHeaderConfig: requestHeaderConfig,
		authenticator:       authenticator,
	}, nil
}

// AuthenticateRequest implements [authenticator.Request]
func (b *DefaultAuthenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	return b.authenticator.AuthenticateRequest(req)
}

// AuthenticateRequest implements [dynamiccertificates.Notifier]
// This is part of the [dynamiccertificates.CAContentProvider] interface.
func (b *DefaultAuthenticator) AddListener(listener dynamiccertificates.Listener) {
	b.requestHeaderConfig.CAContentProvider.AddListener(listener)
}

// AuthenticateRequest implements [dynamiccertificates.CAContentProvider]
func (b *DefaultAuthenticator) Name() string {
	return b.requestHeaderConfig.CAContentProvider.Name()
}

// AuthenticateRequest implements [dynamiccertificates.CAContentProvider]
func (b *DefaultAuthenticator) CurrentCABundleContent() []byte {
	return b.requestHeaderConfig.CAContentProvider.CurrentCABundleContent()
}

// AuthenticateRequest implements [dynamiccertificates.CAContentProvider]
func (b *DefaultAuthenticator) VerifyOptions() (x509.VerifyOptions, bool) {
	return b.requestHeaderConfig.CAContentProvider.VerifyOptions()
}

// AuthenticateRequest implements [dynamiccertificates.ControllerRunner]
func (b *DefaultAuthenticator) RunOnce(ctx context.Context) error {
	runner, ok := b.requestHeaderConfig.CAContentProvider.(dynamiccertificates.ControllerRunner)
	if !ok {
		return nil
	}
	return runner.RunOnce(ctx)
}

// AuthenticateRequest implements [dynamiccertificates.ControllerRunner].
//
// It will be called by the "SecureServing" when starting the extension API server
func (b *DefaultAuthenticator) Run(ctx context.Context, workers int) {
	runner, ok := b.requestHeaderConfig.CAContentProvider.(dynamiccertificates.ControllerRunner)
	if !ok {
		return
	}

	runner.Run(ctx, workers)
}

// Copied from https://github.com/kubernetes/apiserver/blob/v0.30.1/pkg/server/options/authentication.go#L407
func createRequestHeaderConfig(client kubernetes.Interface) (*authenticatorfactory.RequestHeaderConfig, error) {
	dynamicRequestHeaderProvider, err := newDynamicRequestHeaderController(client)
	if err != nil {
		return nil, fmt.Errorf("unable to create request header authentication config: %v", err)
	}

	return &authenticatorfactory.RequestHeaderConfig{
		CAContentProvider:   dynamicRequestHeaderProvider,
		UsernameHeaders:     headerrequest.StringSliceProvider(headerrequest.StringSliceProviderFunc(dynamicRequestHeaderProvider.UsernameHeaders)),
		GroupHeaders:        headerrequest.StringSliceProvider(headerrequest.StringSliceProviderFunc(dynamicRequestHeaderProvider.GroupHeaders)),
		ExtraHeaderPrefixes: headerrequest.StringSliceProvider(headerrequest.StringSliceProviderFunc(dynamicRequestHeaderProvider.ExtraHeaderPrefixes)),
		AllowedClientNames:  headerrequest.StringSliceProvider(headerrequest.StringSliceProviderFunc(dynamicRequestHeaderProvider.AllowedClientNames)),
	}, nil
}

// Copied from https://github.com/kubernetes/apiserver/blob/v0.30.1/pkg/server/options/authentication_dynamic_request_header.go#L42
func newDynamicRequestHeaderController(client kubernetes.Interface) (*options.DynamicRequestHeaderController, error) {
	requestHeaderCAController, err := dynamiccertificates.NewDynamicCAFromConfigMapController(
		"client-ca",
		authenticationConfigMapNamespace,
		authenticationConfigMapName,
		"requestheader-client-ca-file",
		client)
	if err != nil {
		return nil, fmt.Errorf("unable to create DynamicCAFromConfigMap controller: %v", err)
	}

	requestHeaderAuthRequestController := headerrequest.NewRequestHeaderAuthRequestController(
		authenticationConfigMapName,
		authenticationConfigMapNamespace,
		client,
		"requestheader-username-headers",
		"requestheader-group-headers",
		"requestheader-extra-headers-prefix",
		"requestheader-allowed-names",
	)
	return &options.DynamicRequestHeaderController{
		ConfigMapCAController:              requestHeaderCAController,
		RequestHeaderAuthRequestController: requestHeaderAuthRequestController,
	}, nil
}
