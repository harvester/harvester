package util

import (
	"strings"

	"github.com/harvester/go-common/ds"
	"github.com/rancher/wrangler/v3/pkg/slice"
)

var builtInNoProxy = []string{
	"localhost",
	"127.0.0.1",
	"0.0.0.0",
	"10.0.0.0/8",
	"longhorn-system",
	"cattle-system",
	"cattle-system.svc",
	"harvester-system",
	".svc",
	".cluster.local",
}

type HTTPProxyConfig struct {
	HTTPProxy  string `json:"httpProxy,omitempty"`
	HTTPSProxy string `json:"httpsProxy,omitempty"`
	NoProxy    string `json:"noProxy,omitempty"`
}

func AddBuiltInNoProxy(noProxy string) string {
	noProxySlice := ds.SliceMapFunc(strings.Split(noProxy, ","),
		func(v string, _ int) string { return strings.TrimSpace(v) })
	for _, item := range builtInNoProxy {
		if !slice.ContainsString(noProxySlice, item) {
			noProxySlice = append(noProxySlice, item)
		}
	}
	return strings.Join(noProxySlice, ",")
}
