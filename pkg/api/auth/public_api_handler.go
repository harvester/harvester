package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rancher/harvester/pkg/config"
	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"

	dashboardauthapi "github.com/kubernetes/dashboard/src/app/backend/auth/api"
	"github.com/pkg/errors"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	kubeconfigutil "k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"
)

const (
	//action
	actionQuery      = "action"
	loginActionName  = "login"
	logoutActionName = "logout"
	//default cluserName/userName/contextName
	defaultRestConfigResourceName = "default"
)

func NewPublicAPIHandler(scaled *config.Scaled, restConfig *rest.Config) *PublicAPIHandler {
	return &PublicAPIHandler{
		secrets:      scaled.CoreFactory.Core().V1().Secret(),
		restConfig:   restConfig,
		tokenManager: scaled.TokenManager,
	}
}

type PublicAPIHandler struct {
	secrets      ctlcorev1.SecretClient
	restConfig   *rest.Config
	tokenManager dashboardauthapi.TokenManager
}

func (h *PublicAPIHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		rw.Write(responseBody(TokenResponse{Errors: []string{"Only POST method is supported"}}))
		return
	}

	action := strings.ToLower(r.URL.Query().Get(actionQuery))
	isSecure := false
	if r.URL.Scheme == "https" {
		isSecure = true
	}

	if action == logoutActionName {
		tokenCookie := &http.Cookie{
			Name:     JWETokenHeader,
			Value:    "",
			Secure:   isSecure,
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
			Expires:  time.Unix(1, 0), //January 1, 1970 UTC
		}
		http.SetCookie(rw, tokenCookie)
		rw.WriteHeader(http.StatusOK)
		return
	}

	if action != loginActionName {
		rw.WriteHeader(http.StatusBadRequest)
		rw.Write(responseBody(TokenResponse{Errors: []string{"Unsupported action"}}))
		return
	}

	var input Login
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		rw.Write(responseBody(TokenResponse{Errors: []string{"Failed to decode request body, " + err.Error()}}))
		return
	}

	tokenResp, statusCode, err := h.login(&input)
	if err != nil {
		rw.WriteHeader(statusCode)
		rw.Write(responseBody(TokenResponse{Errors: []string{err.Error()}}))
		return
	}

	tokenCookie := &http.Cookie{
		Name:     JWETokenHeader,
		Value:    tokenResp.JWEToken,
		Secure:   isSecure,
		Path:     "/",
		HttpOnly: true,
	}

	http.SetCookie(rw, tokenCookie)
	rw.Header().Set("Content-type", "application/json")
	rw.WriteHeader(http.StatusOK)
	rw.Write(responseBody(tokenResp))
}

func (h *PublicAPIHandler) login(input *Login) (tokenResp *TokenResponse, status int, err error) {
	//handle panic from calling kubernetes dashboard tokenManager.Generate
	defer func() {
		if recoveryMessage := recover(); recoveryMessage != nil {
			status = http.StatusInternalServerError
			err = fmt.Errorf("%v", recoveryMessage)
		}
	}()

	authInfo, err := buildAuthInfo(input.Token, input.KubeConfig)
	if err != nil {
		return nil, http.StatusUnauthorized, errors.Wrapf(err, "Failed to build kubernetes api configure from authorization info")
	}

	if err = h.accessCheck(authInfo); err != nil {
		return nil, http.StatusUnauthorized, errors.Wrapf(err, "Failed to login")
	}

	impersonateAuthInfo, err := getImpersonateAuthInfo(authInfo)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "Failed to build impersonate authInfo from authorization info")
	}

	token, err := h.tokenManager.Generate(*impersonateAuthInfo)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "Failed to generate token")
	}

	escapedToken := url.QueryEscape(token)
	return &TokenResponse{JWEToken: escapedToken}, http.StatusOK, nil
}

func (h *PublicAPIHandler) accessCheck(authInfo *clientcmdapi.AuthInfo) error {
	clientConfig := buildCmdConfig(authInfo, h.restConfig)
	rawConfig, err := clientConfig.RawConfig()
	if err != nil {
		return errors.Wrapf(err, "Failed to get kubernetes Config")
	}

	clientSet, err := kubeconfigutil.ToClientSet(&rawConfig)
	if err != nil {
		return errors.New("Failed to get clientSet from built cmdapi.Config")
	}

	_, err = clientSet.ServerVersion()
	return err
}

func responseBody(obj interface{}) []byte {
	respBody, err := json.Marshal(obj)
	if err != nil {
		return []byte(`{\"errors\":[\"Failed to parse response body\"]}`)
	}
	return respBody
}
