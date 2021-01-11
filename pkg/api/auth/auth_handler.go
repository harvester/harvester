package auth

import (
	"fmt"
	"net/http"

	dashboardauthapi "github.com/kubernetes/dashboard/src/app/backend/auth/api"
	steveauth "github.com/rancher/steve/pkg/auth"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/rancher/harvester/pkg/config"
)

const (
	jwtServiceAccountClaimSubject = "sub" // https://github.com/kubernetes/kubernetes/blob/3783e03dc9df61604c470aa21f198a888e3ec692/pkg/serviceaccount/claims.go#L64
)

func NewMiddleware(scaled *config.Scaled) *Middleware {
	return &Middleware{
		tokenManager: scaled.TokenManager,
	}
}

type Middleware struct {
	tokenManager dashboardauthapi.TokenManager
}

func (h *Middleware) ToAuthMiddleware() steveauth.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			jweToken, err := extractJWETokenFromRequest(r)
			if err != nil {
				responseError(rw, http.StatusUnauthorized, err.Error())
				return
			}

			userInfo, err := h.getUserInfoFromToken(jweToken)
			if err != nil {
				responseError(rw, http.StatusUnauthorized, err.Error())
				return
			}

			ctx := request.WithUser(r.Context(), userInfo)
			r = r.WithContext(ctx)
			next.ServeHTTP(rw, r)
		})
	}
}

func (h *Middleware) getUserInfoFromToken(jweToken string) (userInfo user.Info, err error) {
	//handle panic from calling kubernetes dashboard tokenManager.Decrypt
	defer func() {
		if recoveryMessage := recover(); recoveryMessage != nil {
			err = fmt.Errorf("%v", recoveryMessage)
		}
	}()

	var authInfo *api.AuthInfo
	authInfo, err = h.tokenManager.Decrypt(jweToken)
	if err != nil {
		return
	}

	return impersonateAuthInfoToUserInfo(authInfo), nil
}
