package ui

import (
	apiserver "github.com/rancher/apiserver/pkg/server"
	"github.com/rancher/apiserver/pkg/writer"

	"github.com/harvester/harvester/pkg/settings"
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
	w.CSSURL = CSSURL
	w.JSURL = JSURL
	w.APIUIVersion = settings.APIUIVersion.Get
}

func JSURL() string {
	switch settings.UISource.Get() {
	case "auto":
		if !settings.IsRelease() {
			return ""
		}
	case "external":
		return ""
	}
	return "/api-ui/ui.min.js"
}

func CSSURL() string {
	switch settings.UISource.Get() {
	case "auto":
		if !settings.IsRelease() {
			return ""
		}
	case "external":
		return ""
	}
	return "/api-ui/ui.min.css"
}
