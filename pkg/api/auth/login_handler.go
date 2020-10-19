package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	ctlalpha1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/indexeres"

	dashboardauthapi "github.com/kubernetes/dashboard/src/app/backend/auth/api"
	"github.com/pkg/errors"
	"github.com/rancher/apiserver/pkg/apierror"
	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"golang.org/x/crypto/bcrypt"
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

var (
	publicInfoViewerGroup = "system:unauthenticated"
)

func NewLoginHandler(scaled *config.Scaled, restConfig *rest.Config) *LoginHandler {
	invalidHash, err := bcrypt.GenerateFromPassword([]byte("invalid"), bcrypt.DefaultCost)
	if err != nil {
		panic("failed to generate password invalid hash, " + err.Error())
	}
	return &LoginHandler{
		secrets:      scaled.CoreFactory.Core().V1().Secret(),
		userCache:    scaled.Management.HarvesterFactory.Harvester().V1alpha1().User().Cache(),
		restConfig:   restConfig,
		tokenManager: scaled.TokenManager,
		invalidHash:  invalidHash,
	}
}

type LoginHandler struct {
	secrets      ctlcorev1.SecretClient
	userCache    ctlalpha1.UserCache
	restConfig   *rest.Config
	tokenManager dashboardauthapi.TokenManager
	invalidHash  []byte
}

func (h *LoginHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		rw.Write(responseBody(v1alpha1.ErrorResponse{Errors: []string{"Only POST method is supported"}}))
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
		rw.Write(responseBody(v1alpha1.ErrorResponse{Errors: []string{"Unsupported action"}}))
		return
	}

	var input v1alpha1.Login
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		rw.Write(responseBody(v1alpha1.ErrorResponse{Errors: []string{"Failed to decode request body, " + err.Error()}}))
		return
	}

	authMode, err := authenticationModeVerify(&input)
	if err != nil {
		rw.WriteHeader(http.StatusUnauthorized)
		rw.Write(responseBody(v1alpha1.ErrorResponse{Errors: []string{err.Error()}}))
	}

	tokenResp, err := h.login(&input, authMode)
	if err != nil {
		if e, ok := err.(*apierror.APIError); ok {
			rw.WriteHeader(e.Code.Status)
		} else {
			rw.WriteHeader(http.StatusInternalServerError)
		}
		rw.Write(responseBody(v1alpha1.ErrorResponse{Errors: []string{err.Error()}}))
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

func (h *LoginHandler) login(input *v1alpha1.Login, mode v1alpha1.AuthenticationMode) (tokenResp *v1alpha1.TokenResponse, err error) {
	//handle panic from calling kubernetes dashboard tokenManager.Generate
	defer func() {
		if recoveryMessage := recover(); recoveryMessage != nil {
			err = fmt.Errorf("%v", recoveryMessage)
		}
	}()

	var impersonateAuthInfo *clientcmdapi.AuthInfo
	if mode == v1alpha1.LocalUser {
		impersonateAuthInfo, err = h.userLogin(input)
		if err != nil {
			return nil, err
		}
	} else {
		impersonateAuthInfo, err = h.k8sLogin(input)
		if err != nil {
			return nil, err
		}
	}

	var token string
	token, err = h.tokenManager.Generate(*impersonateAuthInfo)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to generate token")
	}

	escapedToken := url.QueryEscape(token)
	return &v1alpha1.TokenResponse{JWEToken: escapedToken}, nil
}

func (h *LoginHandler) userLogin(input *v1alpha1.Login) (*clientcmdapi.AuthInfo, error) {
	username := input.Username
	pwd := input.Password

	user, err := h.getUser(username)
	if err != nil {
		// If the user don't exist the password is evaluated
		// to avoid user enumeration via timing attack (time based side-channel).
		bcrypt.CompareHashAndPassword(h.invalidHash, []byte(pwd))
		return nil, apierror.NewAPIError(validation.Unauthorized, err.Error())
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(pwd)); err != nil {
		return nil, apierror.NewAPIError(validation.Unauthorized, err.Error())
	}

	if user.IsAdmin {
		return &clientcmdapi.AuthInfo{Impersonate: user.Name}, nil
	}

	return &clientcmdapi.AuthInfo{ImpersonateGroups: []string{publicInfoViewerGroup}}, nil
}

func (h *LoginHandler) k8sLogin(input *v1alpha1.Login) (*clientcmdapi.AuthInfo, error) {
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

func (h *LoginHandler) getUser(username string) (*v1alpha1.User, error) {
	objs, err := h.userCache.GetByIndex(indexeres.UserNameIndex, username)
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, errors.New("authentication failed")
	}
	if len(objs) > 1 {
		return nil, errors.New("found more than one users with username " + username)
	}
	return objs[0], nil
}

func authenticationModeVerify(input *v1alpha1.Login) (v1alpha1.AuthenticationMode, error) {
	if input.Username != "" && input.Password != "" && enableLocalUser() {
		return v1alpha1.LocalUser, nil
	} else if (input.Token != "" || input.KubeConfig != "") && enableKubernetesCredentials() {
		return v1alpha1.KubernetesCredentials, nil
	}
	return "", apierror.NewAPIError(validation.Unauthorized, "unsupported authentication mode")
}

func responseBody(obj interface{}) []byte {
	respBody, err := json.Marshal(obj)
	if err != nil {
		return []byte(`{\"errors\":[\"Failed to parse response body\"]}`)
	}
	return respBody
}
