package ext

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	utilversion "k8s.io/apiserver/pkg/util/version"
	openapicommon "k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

var (
	schemeBuilder = runtime.NewSchemeBuilder(addKnownTypes, metainternalversion.AddToScheme)
	AddToScheme   = schemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	metav1.AddToGroupVersion(scheme, schema.GroupVersion{Version: "v1"})
	return nil
}

type ExtensionAPIServerOptions struct {
	// GetOpenAPIDefinitions is collection of all definitions. Required.
	GetOpenAPIDefinitions             openapicommon.GetOpenAPIDefinitions
	OpenAPIDefinitionNameReplacements map[string]string

	// Authenticator will be used to authenticate requests coming to the
	// extension API server. Required.
	Authenticator authenticator.Request

	// Authorizer will be used to authorize requests based on the user,
	// operation and resources. Required.
	//
	// Use [NewAccessSetAuthorizer] for an authorizer that uses Steve's access set.
	Authorizer authorizer.Authorizer

	// Listener is the TCP listener that is used to listen to the extension API server
	// that is reached by the main kube-apiserver. Required.
	Listener net.Listener

	// EffectiveVersion determines which features and apis are supported
	// by our custom API server.
	//
	// This is a new alpha feature from Kubernetes, the details can be
	// found here: https://github.com/kubernetes/enhancements/tree/master/keps/sig-architecture/4330-compatibility-versions
	//
	// If nil, the default version is the version of the Kubernetes Go library
	// compiled in the final binary.
	EffectiveVersion utilversion.EffectiveVersion
}

// ExtensionAPIServer wraps a [genericapiserver.GenericAPIServer] to implement
// a Kubernetes extension API server.
//
// Use [NewExtensionAPIServer] to create an ExtensionAPIServer.
//
// Use [InstallStore] to add a new resource store onto an existing ExtensionAPIServer.
// Each resources will then be reachable via /apis/<group>/<version>/<resource> as
// defined by the Kubernetes API.
//
// When Run() is called, a separate HTTPS server is started. This server is meant
// for the main kube-apiserver to communicate with our extension API server. We
// can expect the following requests from the main kube-apiserver:
//
// <path>                 <user>                 <groups>
// /openapi/v2            system:aggregator      [system:authenticated]
// /openapi/v3            system:aggregator      [system:authenticated]
// /apis                  system:kube-aggregator [system:masters system:authenticated]
// /apis/ext.cattle.io/v1 system:kube-aggregator [system:masters system:authenticated]
type ExtensionAPIServer struct {
	codecs serializer.CodecFactory
	scheme *runtime.Scheme

	genericAPIServer *genericapiserver.GenericAPIServer
	apiGroups        map[string]genericapiserver.APIGroupInfo

	authorizer authorizer.Authorizer

	handlerMu sync.RWMutex
	handler   http.Handler
}

type emptyAddresses struct{}

func (e emptyAddresses) ServerAddressByClientCIDRs(clientIP net.IP) []metav1.ServerAddressByClientCIDR {
	return nil
}

func NewExtensionAPIServer(scheme *runtime.Scheme, codecs serializer.CodecFactory, opts ExtensionAPIServerOptions) (*ExtensionAPIServer, error) {
	if opts.Authenticator == nil {
		return nil, fmt.Errorf("authenticator must be provided")
	}

	if opts.Authorizer == nil {
		return nil, fmt.Errorf("authorizer must be provided")
	}

	if opts.Listener == nil {
		return nil, fmt.Errorf("listener must be provided")
	}

	recommendedOpts := genericoptions.NewRecommendedOptions("", codecs.LegacyCodec())
	recommendedOpts.SecureServing.Listener = opts.Listener

	resolver := &request.RequestInfoFactory{APIPrefixes: sets.NewString("apis", "api"), GrouplessAPIPrefixes: sets.NewString("api")}
	config := genericapiserver.NewRecommendedConfig(codecs)
	config.RequestInfoResolver = resolver
	config.Authorization = genericapiserver.AuthorizationInfo{
		Authorizer: opts.Authorizer,
	}
	// The default kube effective version ends up being the version of the
	// library. (The value is hardcoded but it is kept up-to-date via some
	// automation)
	config.EffectiveVersion = utilversion.DefaultKubeEffectiveVersion()
	if opts.EffectiveVersion != nil {
		config.EffectiveVersion = opts.EffectiveVersion
	}

	// This feature is more of an optimization for clients that want to go directly to a custom API server
	// instead of going through the main apiserver. We currently don't need to support this so we're leaving this
	// empty.
	config.DiscoveryAddresses = emptyAddresses{}

	config.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(opts.GetOpenAPIDefinitions, openapi.NewDefinitionNamer(scheme))
	config.OpenAPIConfig.Info.Title = "Ext"
	config.OpenAPIConfig.Info.Version = "0.1"
	config.OpenAPIConfig.GetDefinitionName = getDefinitionName(scheme, opts.OpenAPIDefinitionNameReplacements)
	// Must set to nil otherwise getDefinitionName won't be used for refs
	// which will break kubectl explain
	config.OpenAPIConfig.Definitions = nil

	config.OpenAPIV3Config = genericapiserver.DefaultOpenAPIV3Config(opts.GetOpenAPIDefinitions, openapi.NewDefinitionNamer(scheme))
	config.OpenAPIV3Config.Info.Title = "Ext"
	config.OpenAPIV3Config.Info.Version = "0.1"
	config.OpenAPIV3Config.GetDefinitionName = getDefinitionName(scheme, opts.OpenAPIDefinitionNameReplacements)
	// Must set to nil otherwise getDefinitionName won't be used for refs
	// which will break kubectl explain
	config.OpenAPIV3Config.Definitions = nil

	if err := recommendedOpts.SecureServing.ApplyTo(&config.SecureServing, &config.LoopbackClientConfig); err != nil {
		return nil, fmt.Errorf("applyto secureserving: %w", err)
	}

	config.Authentication.Authenticator = opts.Authenticator

	completedConfig := config.Complete()
	genericServer, err := completedConfig.New("imperative-api", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, fmt.Errorf("new: %w", err)
	}

	extensionAPIServer := &ExtensionAPIServer{
		codecs:           codecs,
		scheme:           scheme,
		genericAPIServer: genericServer,
		apiGroups:        make(map[string]genericapiserver.APIGroupInfo),
		authorizer:       opts.Authorizer,
	}

	return extensionAPIServer, nil
}

// Run prepares and runs the separate HTTPS server. It also configures the handler
// so that ServeHTTP can be used.
func (s *ExtensionAPIServer) Run(ctx context.Context) error {
	for _, apiGroup := range s.apiGroups {
		err := s.genericAPIServer.InstallAPIGroup(&apiGroup)
		if err != nil {
			return fmt.Errorf("installgroup: %w", err)
		}
	}
	prepared := s.genericAPIServer.PrepareRun()
	s.handlerMu.Lock()
	s.handler = prepared.Handler
	s.handlerMu.Unlock()

	return nil
}

func (s *ExtensionAPIServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.handlerMu.RLock()
	defer s.handlerMu.RUnlock()
	s.handler.ServeHTTP(w, req)
}

// InstallStore installs a store on the given ExtensionAPIServer object.
//
// t and TList must be non-nil.
//
// Here's an example store for a Token and TokenList resource in the ext.cattle.io/v1 apiVersion:
//
//	gvk := schema.GroupVersionKind{
//		Group: "ext.cattle.io",
//		Version: "v1",
//		Kind: "Token",
//	}
//	InstallStore(s, &Token{}, &TokenList{}, "tokens", "token", gvk, store)
//
// Note: Not using a method on ExtensionAPIServer object due to Go generic limitations.
func InstallStore[T runtime.Object, TList runtime.Object](
	s *ExtensionAPIServer,
	t T,
	tList TList,
	resourceName string,
	singularName string,
	gvk schema.GroupVersionKind,
	store Store[T, TList],
) error {

	if !meta.IsListType(tList) {
		return fmt.Errorf("tList (%T) is not a list type", tList)
	}

	apiGroup, ok := s.apiGroups[gvk.Group]
	if !ok {
		apiGroup = genericapiserver.NewDefaultAPIGroupInfo(gvk.Group, s.scheme, metav1.ParameterCodec, s.codecs)
	}

	_, ok = apiGroup.VersionedResourcesStorageMap[gvk.Version]
	if !ok {
		apiGroup.VersionedResourcesStorageMap[gvk.Version] = make(map[string]rest.Storage)
	}

	del := &delegateError[T, TList]{
		inner: &delegate[T, TList]{
			scheme: s.scheme,

			t:            t,
			tList:        tList,
			singularName: singularName,
			gvk:          gvk,
			gvr: schema.GroupVersionResource{
				Group:    gvk.Group,
				Version:  gvk.Version,
				Resource: resourceName,
			},
			authorizer: s.authorizer,
			store:      store,
		},
	}

	apiGroup.VersionedResourcesStorageMap[gvk.Version][resourceName] = del
	s.apiGroups[gvk.Group] = apiGroup
	return nil
}

func getDefinitionName(scheme *runtime.Scheme, replacements map[string]string) func(string) (string, spec.Extensions) {
	return func(name string) (string, spec.Extensions) {
		namer := openapi.NewDefinitionNamer(scheme)
		definitionName, defGVK := namer.GetDefinitionName(name)
		for key, val := range replacements {
			if !strings.HasPrefix(definitionName, key) {
				continue
			}

			updatedName := strings.ReplaceAll(definitionName, key, val)
			return updatedName, defGVK
		}
		return definitionName, defGVK
	}
}
