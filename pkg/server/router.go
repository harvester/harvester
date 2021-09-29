package server

import (
	"net/http"
	"net/url"

	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/urlbuilder"
	"github.com/rancher/steve/pkg/server/router"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"

	"github.com/harvester/harvester/pkg/api/kubeconfig"
	"github.com/harvester/harvester/pkg/api/proxy"
	"github.com/harvester/harvester/pkg/api/supportbundle"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/server/ui"
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

//Routes adds some customize routes to the default router
func (r *Router) Routes(h router.Handlers) http.Handler {
	m := mux.NewRouter()
	m.UseEncodedPath()
	m.StrictSlash(true)
	m.Use(urlbuilder.RedirectRewrite)

	m.Path("/").HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		http.Redirect(rw, req, "/dashboard/", http.StatusFound)
	})

	// Those routes should be above /v1/harvester/{type}, otherwise, the response status code would be 404
	kcGenerateHandler := kubeconfig.NewGenerateHandler(r.scaled, r.options)
	m.Path("/v1/harvester/kubeconfig").Methods("POST").Handler(kcGenerateHandler)

	sbDownloadHandler := supportbundle.NewDownloadHandler(r.scaled, r.options.Namespace)
	m.Path("/v1/harvester/supportbundles/{bundleName}/download").Methods("GET").Handler(sbDownloadHandler)
	// --- END of preposition routes ---

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
	m.PathPrefix("/api-ui").Handler(vueUI.ServeAsset())

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
