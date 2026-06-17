package router

import (
	"net/http"
	"strings"

	"github.com/rancher/apiserver/pkg/urlbuilder"
)

type RouterFunc func(h Handlers) http.Handler

type Handlers struct {
	K8sResource http.Handler
	APIRoot     http.Handler
	K8sProxy    http.Handler
	Next        http.Handler
	// ExtensionAPIServer serves under /ext. If nil, the default unknown path
	// handler is served.
	ExtensionAPIServer http.Handler
}

func Routes(h Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/{$}", func(rw http.ResponseWriter, req *http.Request) {
		if strings.Contains(req.Header.Get("Accept"), "json") {
			h.APIRoot.ServeHTTP(rw, req)
		} else {
			h.Next.ServeHTTP(rw, req)
		}
	})
	mux.Handle("/v1", h.APIRoot)
	mux.Handle("/v1/{$}", h.APIRoot)

	if h.ExtensionAPIServer != nil {
		mux.Handle("/ext", http.StripPrefix("/ext", h.ExtensionAPIServer))
		mux.Handle("/ext/", http.StripPrefix("/ext", h.ExtensionAPIServer))
	}

	// K8s Resources
	mux.Handle("/v1/{type}", h.K8sResource)
	mux.Handle("/v1/{type}/{$}", h.K8sResource)
	mux.Handle("/v1/{type}/{nameorns}", h.K8sResource)
	mux.Handle("/v1/{type}/{nameorns}/{$}", h.K8sResource)
	mux.Handle("/v1/{type}/{namespace}/{name}", h.K8sResource)
	mux.Handle("/v1/{type}/{namespace}/{name}/{$}", h.K8sResource)
	mux.Handle("/v1/{type}/{namespace}/{name}/{link}", h.K8sResource)
	mux.Handle("/v1/{type}/{namespace}/{name}/{link}/{$}", h.K8sResource)

	// K8s Proxies
	mux.Handle("/api", h.K8sProxy)
	mux.Handle("/api/", h.K8sProxy)
	mux.Handle("/apis", h.K8sProxy)
	mux.Handle("/apis/", h.K8sProxy)
	mux.Handle("/openapi", h.K8sProxy)
	mux.Handle("/openapi/", h.K8sProxy)
	mux.Handle("/version", h.K8sProxy)
	mux.Handle("/version/", h.K8sProxy)

	mux.Handle("/", h.Next)

	return urlbuilder.RedirectRewrite(mux)
}
