package ui

import (
	apiserver "github.com/rancher/apiserver/pkg/server"

	"github.com/rancher/harvester/pkg/settings"
)

func ConfigureAPIUI(server *apiserver.Server) {
	server.CustomAPIUIResponseWriter(CSSURL, JSURL, settings.APIUIVersion.Get)
}

func JSURL() string {
	switch settings.APIUISource.Get() {
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
	switch settings.APIUISource.Get() {
	case "auto":
		if !settings.IsRelease() {
			return ""
		}
	case "external":
		return ""
	}
	return "/api-ui/ui.min.css"
}
