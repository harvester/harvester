package server

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/urlbuilder"
	"github.com/rancher/steve/pkg/server/router"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"

	"github.com/harvester/harvester/pkg/api/backuptarget"
	"github.com/harvester/harvester/pkg/api/kubeconfig"
	"github.com/harvester/harvester/pkg/api/proxy"
	"github.com/harvester/harvester/pkg/api/readyz"
	"github.com/harvester/harvester/pkg/api/supportbundle"
	"github.com/harvester/harvester/pkg/api/uiinfo"
	"github.com/harvester/harvester/pkg/config"
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
	"github.com/harvester/harvester/pkg/server/ui"
	"github.com/harvester/harvester/pkg/util"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
)

const (
	localRKEStateSecretName = "local-rke-state"
	serverTokenKey          = "serverToken"
)

type Router struct {
	scaled     *config.Scaled
	restConfig *rest.Config
	options    config.Options
}

func NewRouter(scaled *config.Scaled, restConfig *rest.Config, options config.Options) (*Router, error) {
	return &Router{
		scaled:     scaled,
		restConfig: restConfig,
		options:    options,
	}, nil
}

// Routes adds some customize routes to the default router
func (r *Router) Routes(h router.Handlers) http.Handler {
	m := mux.NewRouter()
	m.UseEncodedPath()
	m.StrictSlash(true)
	m.Use(urlbuilder.RedirectRewrite)
	m.Use(recoveryMiddleware)

	m.Path("/").HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		http.Redirect(rw, req, "/dashboard/", http.StatusFound)
	})

	// Those routes should be above /v1/harvester/{type}, otherwise, the response status code would be 404
	kcGenerateHandler := harvesterServer.NewHandler(kubeconfig.NewGenerateHandler(r.scaled, r.options))
	m.Path("/v1/harvester/kubeconfig").Methods("POST").Handler(kcGenerateHandler)

	uiInfoHandler := harvesterServer.NewHandler(uiinfo.NewUIInfoHandler(r.scaled, r.options))
	m.Path("/v1/harvester/ui-info").Methods("GET").Handler(uiInfoHandler)
	m.PathPrefix("/v1/harvester/plugin-assets").Handler(ui.Vue.PluginServeAsset())

	sbDownloadHandler := harvesterServer.NewHandler(supportbundle.NewDownloadHandler(r.scaled, r.options.Namespace))
	m.Path("/v1/harvester/supportbundles/{bundleName}/download").Methods("GET").Handler(sbDownloadHandler)

	btHealthyHandler := harvesterServer.NewHandler(backuptarget.NewHealthyHandler(r.scaled))
	m.Path("/v1/harvester/backuptarget/healthz").Methods("GET").Handler(btHealthyHandler)

	readyzHandlerv1 := harvesterServer.NewHandler(readyz.NewReadyzHandler(
		r.scaled.CoreFactory.Core().V1().Pod().Cache(),
		r.scaled.Management.RKEFactory.Rke().V1().RKEControlPlane().Cache()))
	m.Path("/v1/harvester/readyz").Methods("GET").Handler(authMiddleware(r.scaled.CoreFactory.Core().V1().Secret().Cache(), readyzHandlerv1))

	// --- END of preposition routes ---

	// This is for manually testing the recovery handler below
	m.HandleFunc("/v1/harvester/dont-panic", func(_ http.ResponseWriter, _ *http.Request) {
		panic("Do you know where your towel is?")
	})

	// adds collection action support
	m.Path("/v1/{type}").Queries("action", "{action}").Handler(h.K8sResource)

	// aggregation at /v1/harvester/
	// By default vars are split by slashes. Use a custom matcher to generate the name var.
	matchV1Harvester := func(r *http.Request, match *mux.RouteMatch) bool {
		if r.URL.Path == "/v1/harvester" {
			match.Vars = map[string]string{"name": "v1/harvester"}
			return true
		}
		return false
	}
	m.Path("/v1/harvester").MatcherFunc(matchV1Harvester).Handler(h.APIRoot)
	m.Path("/v1/harvester/{type}").Handler(h.K8sResource)
	m.Path("/v1/harvester/{type}").Queries("action", "{action}").Handler(h.K8sResource)
	m.Path("/v1/harvester/{type}/{nameorns}").Queries("link", "{link}").Handler(h.K8sResource)
	m.Path("/v1/harvester/{type}/{nameorns}").Queries("action", "{action}").Handler(h.K8sResource)
	m.Path("/v1/harvester/{type}/{nameorns}").Handler(h.K8sResource)
	m.Path("/v1/harvester/{type}/{namespace}/{name}").Queries("action", "{action}").Handler(h.K8sResource)
	m.Path("/v1/harvester/{type}/{namespace}/{name}").Queries("link", "{link}").Handler(h.K8sResource)
	m.Path("/v1/harvester/{type}/{namespace}/{name}").Handler(h.K8sResource)
	m.Path("/v1/harvester/{type}/{namespace}/{name}/{link}").Handler(h.K8sResource)

	vueUI := ui.Vue
	m.Handle("/dashboard/", vueUI.IndexFile())
	m.PathPrefix("/dashboard/").Handler(vueUI.IndexFileOnNotFound())

	if r.options.RancherURL != "" {
		host, scheme, err := parseRancherServerURL(r.options.RancherURL)
		if err != nil {
			logrus.Fatal(err)
		}
		rancherHandler := &proxy.Handler{
			Host:   host,
			Scheme: scheme,
		}
		m.PathPrefix("/v3-public/").Handler(rancherHandler)
		m.PathPrefix("/v3/").Handler(rancherHandler)
		m.PathPrefix("/v1/userpreferences").Handler(rancherHandler)
		m.PathPrefix("/v1/management.cattle.io.setting").Handler(rancherHandler)
	}

	m.NotFoundHandler = router.Routes(h)

	return m
}

func authMiddleware(secretCache ctlcorev1.SecretCache, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Bearer token required")) // nolint: errcheck
			return
		}

		providedToken := strings.TrimPrefix(authHeader, "Bearer ")

		actualToken, err := getTokenFromSecret(secretCache)
		if err != nil {
			logrus.WithError(err).Error("Failed to get token from secret")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal server error")) // nolint: errcheck
			return
		}

		if err := validateToken(actualToken, providedToken); err != nil {
			logrus.WithError(err).Warn("Token validation failed")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized")) // nolint: errcheck
			return
		}

		next.ServeHTTP(w, r)
	})
}

func validateToken(actualToken, providedToken string) error {
	actualBytes := []byte(actualToken)
	providedBytes := []byte(providedToken)
	if subtle.ConstantTimeCompare(actualBytes, providedBytes) != 1 {
		return fmt.Errorf("tokens do not match")
	}
	return nil
}

func getTokenFromSecret(secretCache ctlcorev1.SecretCache) (string, error) {
	secret, err := secretCache.Get(util.FleetLocalNamespaceName, localRKEStateSecretName)
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s/%s: %s", util.FleetLocalNamespaceName, localRKEStateSecretName, err.Error())
	}

	tokenBytes, ok := secret.Data[serverTokenKey]
	if !ok {
		return "", fmt.Errorf("token key %s not found in secret", serverTokenKey)
	}

	if len(tokenBytes) == 0 {
		return "", fmt.Errorf("token is empty in secret")
	}

	return string(tokenBytes), nil
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(fmt.Sprint(err))) // nolint: errcheck
				logrus.WithFields(logrus.Fields{
					"err":   err,
					"stack": string(debug.Stack()),
				}).Error("Recovered panic in Routes")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func parseRancherServerURL(endpoint string) (string, string, error) {
	if endpoint == "" {
		return "", "", nil
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return "", "", err
	}

	return u.Host, u.Scheme, nil
}
