package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	harvesterv1 "github.com/rancher/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/rancher/harvester/pkg/settings"
)

func ModeHandler(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		responseError(rw, http.StatusMethodNotAllowed, "Only GET method is supported")
		return
	}

	body, err := json.Marshal(harvesterv1.AuthenticationModesResponse{Modes: authModes()})
	if err != nil {
		responseError(rw, http.StatusInternalServerError, "Failed to encode authenticationModes, "+err.Error())
		return
	}

	rw.Header().Set("Content-type", "application/json")
	_, _ = rw.Write(body)
}

func authModes() []harvesterv1.AuthenticationMode {
	ms := strings.Split(settings.AuthenticationMode.Get(), ",")
	modes := make([]harvesterv1.AuthenticationMode, 0, len(ms))
	for _, v := range ms {
		modes = append(modes, harvesterv1.AuthenticationMode(strings.TrimSpace(v)))
	}
	return modes
}

func enableKubernetesCredentials() bool {
	return strings.Contains(settings.AuthenticationMode.Get(), string(harvesterv1.KubernetesCredentials))
}

func enableLocalUser() bool {
	return strings.Contains(settings.AuthenticationMode.Get(), string(harvesterv1.LocalUser))
}
