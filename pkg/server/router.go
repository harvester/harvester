package server

import (
	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/urlbuilder"
	"github.com/rancher/steve/pkg/server/router"
	"net/http"
)

// Routes adds collection action support
func Routes(h router.Handlers) http.Handler {
	m := mux.NewRouter()
	m.UseEncodedPath()
	m.StrictSlash(true)
	m.Use(urlbuilder.RedirectRewrite)

	m.Path("/v1/{type}").Queries("action", "{action}").Handler(h.K8sResource)
	m.NotFoundHandler = router.Routes(h)

	return m
}
