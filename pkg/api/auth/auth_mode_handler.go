package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/settings"
)

func ModeHandler(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		responseError(rw, http.StatusMethodNotAllowed, "Only GET method is supported")
		return
	}

	body, err := json.Marshal(v1alpha1.AuthenticationModesResponse{Modes: authModes()})
	if err != nil {
		responseError(rw, http.StatusInternalServerError, "Failed to encode authenticationModes, "+err.Error())
		return
	}

	rw.Header().Set("Content-type", "application/json")
	_, _ = rw.Write(body)
}

func authModes() []v1alpha1.AuthenticationMode {
	ms := strings.Split(settings.AuthenticationMode.Get(), ",")
	modes := make([]v1alpha1.AuthenticationMode, 0, len(ms))
	for _, v := range ms {
		modes = append(modes, v1alpha1.AuthenticationMode(strings.TrimSpace(v)))
	}
	return modes
}

func enableKubernetesCredentials() bool {
	return strings.Contains(settings.AuthenticationMode.Get(), string(v1alpha1.KubernetesCredentials))
}

func enableLocalUser() bool {
	return strings.Contains(settings.AuthenticationMode.Get(), string(v1alpha1.LocalUser))
}
