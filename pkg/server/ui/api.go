package ui

import (
	"fmt"

	apiserver "github.com/rancher/apiserver/pkg/server"
	"github.com/rancher/apiserver/pkg/writer"

	"github.com/harvester/harvester/pkg/settings"
)

const (
	UIAssetPathPrefix = "/dashboard/api-ui"
)

func ConfigureAPIUI(server *apiserver.Server) {
	wi, ok := server.ResponseWriters["html"]
	if !ok {
		return
	}
	gw, ok := wi.(*writer.GzipWriter)
	if !ok {
		return
	}

	w, ok := gw.ResponseWriter.(*writer.HTMLResponseWriter)
	if !ok {
		return
	}
	w.CSSURL = URL("ui.min.css")
	w.JSURL = URL("ui.min.js")
	w.APIUIVersion = settings.APIUIVersion.Get
}

func URL(filename string) func() string {
	return func() string {
		switch settings.UISource.Get() {
		case "auto":
			if !settings.IsRelease() {
				return ""
			}
		case "external":
			return ""
		}
		return fmt.Sprintf("%s/%s", UIAssetPathPrefix, filename)
	}
}
