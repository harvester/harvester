package ui

import (
	"net/http"

	"github.com/rancher/vm/pkg/settings"
)

func Content() http.Handler {
	return http.FileServer(http.Dir(settings.UIPath.Get()))
}

func JSURLGetter() string {
	if settings.UIIndex.Get() == "local" {
		return "/api-ui/ui.min.js"
	}
	return ""
}

func CSSURLGetter() string {
	if settings.UIIndex.Get() == "local" {
		return "/api-ui/ui.min.css"
	}
	return ""
}

func APIUIVersionGetter() string {
	return settings.APIUIVersion.Get()
}
