package server

import (
	"net/http"
	"net/url"

	"github.com/sirupsen/logrus"

	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/urlbuilder"
	"github.com/rancher/steve/pkg/server/router"
	"k8s.io/client-go/rest"

	"github.com/harvester/harvester/pkg/api/auth"
	"github.com/harvester/harvester/pkg/api/proxy"
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

	m.Path("/v1/{type}").Queries("action", "{action}").Handler(h.K8sResource) //adds collection action support

	loginHandler := auth.NewLoginHandler(r.scaled, r.restConfig)
	m.Path("/v1-public/auth").Handler(loginHandler)
	m.Path("/v1-public/auth-modes").HandlerFunc(auth.ModeHandler)

	vueUI := ui.Vue
	m.Handle("/dashboard/", vueUI.IndexFile())
	m.PathPrefix("/dashboard/").Handler(vueUI.IndexFileOnNotFound())
	m.PathPrefix("/api-ui").Handler(vueUI.ServeAsset())

	if r.options.RancherEmbedded {
		host, err := parseRancherServerURL(r.options.RancherURL)
		if err != nil {
			logrus.Fatal(err)
		}
		v3Handler := &proxy.Handler{
			Host: host,
		}
		m.PathPrefix("/v3-public/").Handler(v3Handler)
		m.PathPrefix("/v3/").Handler(v3Handler)
	}

	m.NotFoundHandler = router.Routes(h)

	return m
}

func parseRancherServerURL(endpoint string) (string, error) {
	if endpoint == "" {
		return "", nil
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}

	return u.Host, nil
}
