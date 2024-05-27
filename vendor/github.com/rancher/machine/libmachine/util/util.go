package util

import (
	"net/http"
	"net/url"
	"os"
)

func FindEnvAny(names ...string) string {
	for _, n := range names {
		if val := os.Getenv(n); val != "" {
			return val
		}
	}
	return ""
}

// GetProxyURL returns the URL of the proxy to use for this given hostUrl as indicated by the environment variables
// HTTP_PROXY, HTTPS_PROXY and NO_PROXY (or the lowercase versions thereof).
// HTTPS_PROXY takes precedence over HTTP_PROXY for https requests.
// The hostUrl may be either a complete URL or a "host[:port]", in which case the "http" scheme is assumed.
func GetProxyURL(hostUrl string) (*url.URL, error) {
	req, err := http.NewRequest(http.MethodGet, hostUrl, nil)
	if err != nil {
		return nil, err
	}
	proxy, err := http.ProxyFromEnvironment(req)
	if err != nil {
		return nil, err
	}
	return proxy, nil
}
