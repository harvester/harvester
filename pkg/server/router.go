package server

import (
	"net/http"

	"github.com/rancher/harvester/pkg/api/auth"
	"github.com/rancher/harvester/pkg/config"

	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/urlbuilder"
	"github.com/rancher/steve/pkg/server/router"
	"k8s.io/client-go/rest"
)

type Router struct {
	scaled     *config.Scaled
	restConfig *rest.Config
}

func NewRouter(scaled *config.Scaled, restConfig *rest.Config) (*Router, error) {
	return &Router{
		scaled:     scaled,
		restConfig: restConfig,
	}, nil
}

//Routes adds some customize routes to the default router
func (r *Router) Routes(h router.Handlers) http.Handler {
	m := mux.NewRouter()
	m.UseEncodedPath()
	m.StrictSlash(true)
	m.Use(urlbuilder.RedirectRewrite)

	m.Path("/v1/{type}").Queries("action", "{action}").Handler(h.K8sResource) //adds collection action support
	m.Path("/").HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		http.Redirect(rw, req, "/dashboard/", http.StatusFound)
	})

	loginHandler := auth.NewLoginHandler(r.scaled, r.restConfig)
	m.Path("/v1-public/auth").Handler(loginHandler)
	m.Path("/v1-public/auth-modes").HandlerFunc(auth.ModeHandler)

	m.NotFoundHandler = router.Routes(h)

	return m
}
