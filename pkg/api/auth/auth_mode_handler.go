package auth

import (
	"encoding/json"
	"net/http"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

func ModeHandler(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		util.ResponseErrorMsg(rw, http.StatusMethodNotAllowed, "Only GET method is supported")
		return
	}

	body, err := json.Marshal(harvesterv1.AuthenticationModesResponse{Modes: []harvesterv1.AuthenticationMode{harvesterv1.Rancher}})
	if err != nil {
		util.ResponseErrorMsg(rw, http.StatusInternalServerError, "Failed to encode authenticationModes, "+err.Error())
		return
	}

	rw.Header().Set("Content-type", "application/json")
	_, _ = rw.Write(body)
}
