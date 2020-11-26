package helper

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/guonaihong/gout"
	"github.com/guonaihong/gout/dataflow"

	"github.com/rancher/harvester/pkg/config"
)

func NewHTTPClient() *dataflow.Gout {
	return gout.
		New(&http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		})
}

func BuildAPIURL(version, resource string) string {
	return fmt.Sprintf("https://localhost:%d/%s/%s", config.HTTPSListenPort, version, resource)
}
