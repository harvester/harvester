package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	dashboardauthapi "github.com/kubernetes/dashboard/src/app/backend/auth/api"
	"github.com/pkg/errors"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/rancher/pkg/auth/tokens"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"golang.org/x/crypto/bcrypt"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	kubeconfigutil "k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/auth"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	//action
	actionQuery      = "action"
	loginActionName  = "login"
	logoutActionName = "logout"
	//default cluserName/userName/contextName
	defaultRestConfigResourceName = "default"
)

func NewLoginHandler(scaled *config.Scaled, restConfig *rest.Config) *LoginHandler {
	invalidHash, err := bcrypt.GenerateFromPassword([]byte("invalid"), bcrypt.DefaultCost)
	if err != nil {
		panic("failed to generate password invalid hash, " + err.Error())
	}
	return &LoginHandler{
		secrets:      scaled.CoreFactory.Core().V1().Secret(),
		restConfig:   restConfig,
		tokenManager: scaled.TokenManager,
		invalidHash:  invalidHash,
	}
}

type LoginHandler struct {
	secrets      ctlcorev1.SecretClient
	restConfig   *rest.Config
	tokenManager dashboardauthapi.TokenManager
	invalidHash  []byte
}

func (h *LoginHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		util.ResponseErrorMsg(rw, http.StatusMethodNotAllowed, "Only POST method is supported")
		return
	}

	action := strings.ToLower(r.URL.Query().Get(actionQuery))
	isSecure := false
	if r.URL.Scheme == "https" {
		isSecure = true
	}

	if action == logoutActionName {
		resetCookie(rw, JWETokenHeader, isSecure)
		if auth.IsRancherAuthMode() {
			resetCookie(rw, tokens.CookieName, isSecure)
		}
		util.ResponseOK(rw)
		return
	}

	if action != loginActionName {
		util.ResponseErrorMsg(rw, http.StatusBadRequest, "Unsupported action")
		return
	}

	var input harvesterv1.Login
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		util.ResponseErrorMsg(rw, http.StatusBadRequest, "Failed to decode request body, "+err.Error())
		return
	}

	_, err := authenticationModeVerify(&input)
	if err != nil {
		util.ResponseErrorMsg(rw, http.StatusUnauthorized, err.Error())
		return
	}

	tokenResp, err := h.login(&input)
	if err != nil {
		status := http.StatusInternalServerError
		if e, ok := err.(*apierror.APIError); ok {
			status = e.Code.Status
		}
		util.ResponseError(rw, status, err)
		return
	}

	if !strings.EqualFold(settings.FirstLogin.Get(), "false") {
		if err := settings.FirstLogin.Set("false"); err != nil {
			util.ResponseError(rw, http.StatusInternalServerError, err)
		}
	}

	tokenCookie := &http.Cookie{
		Name:     JWETokenHeader,
		Value:    tokenResp.JWEToken,
		Secure:   isSecure,
		Path:     "/",
		HttpOnly: true,
	}

	http.SetCookie(rw, tokenCookie)
	util.ResponseOKWithBody(rw, tokenResp)
}

func (h *LoginHandler) login(input *harvesterv1.Login) (tokenResp *harvesterv1.TokenResponse, err error) {
	//handle panic from calling kubernetes dashboard tokenManager.Generate
	defer func() {
		if recoveryMessage := recover(); recoveryMessage != nil {
			err = fmt.Errorf("%v", recoveryMessage)
		}
	}()

	var impersonateAuthInfo *clientcmdapi.AuthInfo
	impersonateAuthInfo, err = h.k8sLogin(input)
	if err != nil {
		return nil, err
	}

	var token string
	token, err = h.tokenManager.Generate(*impersonateAuthInfo)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to generate token")
	}

	escapedToken := url.QueryEscape(token)
	return &harvesterv1.TokenResponse{JWEToken: escapedToken}, nil
}

func (h *LoginHandler) k8sLogin(input *harvesterv1.Login) (*clientcmdapi.AuthInfo, error) {
	authInfo, err := buildAuthInfo(input.Token, input.KubeConfig)
	if err != nil {
		return nil, apierror.NewAPIError(validation.Unauthorized, "Failed to build kubernetes api configure from authorization info, "+err.Error())
	}

	if err = h.accessCheck(authInfo); err != nil {
		return nil, apierror.NewAPIError(validation.Unauthorized, "Failed to access kubernetes cluster, "+err.Error())
	}

	impersonateAuthInfo, err := getImpersonateAuthInfo(authInfo)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to build impersonate authInfo from authorization info")
	}
	return impersonateAuthInfo, nil
}

func (h *LoginHandler) accessCheck(authInfo *clientcmdapi.AuthInfo) error {
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

func authenticationModeVerify(input *harvesterv1.Login) (harvesterv1.AuthenticationMode, error) {
	if (input.Token != "" || input.KubeConfig != "") && enableKubernetesCredentials() {
		return harvesterv1.KubernetesCredentials, nil
	}
	return "", apierror.NewAPIError(validation.Unauthorized, "unsupported authentication mode")
}

func resetCookie(rw http.ResponseWriter, name string, isSecure bool) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    "",
		Secure:   isSecure,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0), //January 1, 1970 UTC
	}
	http.SetCookie(rw, cookie)
}
