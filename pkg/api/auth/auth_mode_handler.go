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
		rw.WriteHeader(http.StatusMethodNotAllowed)
		rw.Write(responseBody(v1alpha1.ErrorResponse{Errors: []string{"Only GET method is supported"}}))
		return
	}

	body, err := json.Marshal(v1alpha1.AuthenticationModesResponse{Modes: authModes()})
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		rw.Write(responseBody(v1alpha1.ErrorResponse{Errors: []string{"Failed to encode authenticationModes, " + err.Error()}}))
		return
	}

	rw.Header().Set("Content-type", "application/json")
	rw.Write(body)
}

func authModes() []v1alpha1.AuthenticationMode {
	var modes []v1alpha1.AuthenticationMode
	ms := strings.Split(settings.AuthenticationMode.Get(), ",")
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
