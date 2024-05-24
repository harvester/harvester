package server

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/server"
	"github.com/rancher/apiserver/pkg/store/apiroot"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/channelserver/pkg/config"
	"github.com/rancher/channelserver/pkg/model"
	"github.com/rancher/channelserver/pkg/server/store"
	"github.com/rancher/channelserver/pkg/server/store/appdefault"
	"github.com/rancher/channelserver/pkg/server/store/release"
)

func ListenAndServe(ctx context.Context, address string, configs map[string]*config.Config) error {
	h := NewHandler(configs)

	next := handlers.LoggingHandler(os.Stdout, h)
	handler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		user := req.Header.Get("X-SUC-Cluster-ID")
		if user != "" && req.URL != nil {
			req.URL.User = url.User(user)
		}
		next.ServeHTTP(rw, req)
	})

	return http.ListenAndServe(address, handler)
}

func NewHandler(configs map[string]*config.Config) http.Handler {
	router := mux.NewRouter()
	for prefix, config := range configs {
		server := server.DefaultAPIServer()
		server.Schemas.MustImportAndCustomize(model.Channel{}, func(schema *types.APISchema) {
			schema.Store = store.New(config)
			schema.CollectionMethods = []string{http.MethodGet}
			schema.ResourceMethods = []string{http.MethodGet}
		})
		server.Schemas.MustImportAndCustomize(model.Release{}, func(schema *types.APISchema) {
			schema.Store = release.New(config)
			schema.CollectionMethods = []string{http.MethodGet}
		})
		server.Schemas.MustImportAndCustomize(model.AppDefault{}, func(schema *types.APISchema) {
			schema.Store = appdefault.New(config)
			schema.CollectionMethods = []string{http.MethodGet}
		})
		prefix = strings.Trim(prefix, "/")
		apiroot.Register(server.Schemas, []string{prefix})
		router.MatcherFunc(setType("apiRoot", prefix)).Path("/").Handler(server)
		router.MatcherFunc(setType("apiRoot", prefix)).Path("/{name}").Handler(server)
		router.Path("/{prefix:" + prefix + "}/{type}").Handler(server)
		router.Path("/{prefix:" + prefix + "}/{type}/{name}").Handler(server)
	}
	return router
}

func setType(t string, pathPrefix string) mux.MatcherFunc {
	return func(request *http.Request, match *mux.RouteMatch) bool {
		if match.Vars == nil {
			match.Vars = map[string]string{}
		}
		match.Vars["type"] = t
		match.Vars["prefix"] = pathPrefix
		return true
	}
}
