package proxy

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
)

// Handler proxies requests to the rancher service
type Handler struct {
	Scheme string
	Host   string
}

const (
	ForwardedAPIHostHeader = "X-API-Host"
	ForwardedProtoHeader   = "X-Forwarded-Proto"
	ForwardedHostHeader    = "X-Forwarded-Host"
	PrefixHeader           = "X-API-URL-Prefix"
	OriginHeader           = "Origin"
)

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	director := func(r *http.Request) {
		r.URL.Scheme = "https"
		r.URL.Host = "rancher.cattle-system"
		r.URL.Path = req.URL.Path
		if h.Host != "" {
			r.URL.Host = h.Host
		}
		if h.Scheme != "" {
			r.URL.Scheme = h.Scheme
		}
		// set forwarded header to fix rancher api link
		r.Header.Set(ForwardedAPIHostHeader, GetLastExistValue(req.Host, req.Header.Get(ForwardedAPIHostHeader)))
		r.Header.Set(ForwardedHostHeader, GetLastExistValue(req.Host, req.Header.Get(ForwardedHostHeader)))
		r.Header.Set(ForwardedProtoHeader, GetLastExistValue(req.URL.Scheme, req.Header.Get(ForwardedProtoHeader)))
		r.Header.Set(PrefixHeader, GetLastExistValue(req.Header.Get(PrefixHeader)))
		// set host and origin to fit scenarios: harvester->nginx->rancher
		r.Host = r.URL.Host
		r.Header.Set(OriginHeader, fmt.Sprintf("%s://%s", GetOriginScheme(r.URL.Scheme), r.Host))
	}
	httpProxy := &httputil.ReverseProxy{
		Director: director,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	httpProxy.ServeHTTP(rw, req)
}

func GetOriginScheme(scheme string) string {
	switch scheme {
	case "ws":
		return "http"
	case "wss":
		return "https"
	default:
		return scheme
	}
}

func GetLastExistValue(values ...string) string {
	var result string
	for _, value := range values {
		if value != "" {
			result = value
		}
	}
	return result
}
