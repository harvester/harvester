package auth

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"

	"github.com/rancher/harvester/pkg/auth/jwt"
	"github.com/rancher/harvester/pkg/config"

	dashboardauthapi "github.com/kubernetes/dashboard/src/app/backend/auth/api"
	"github.com/pkg/errors"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/tools/clientcmd/api"
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

func (h *Middleware) AuthMiddleware(rw http.ResponseWriter, r *http.Request, handler http.Handler) {
	jweToken, err := extractJWETokenFromRequest(r)
	if err != nil {
		rw.WriteHeader(http.StatusUnauthorized)
		rw.Write(responseBody(TokenResponse{Errors: []string{err.Error()}}))
		return
	}

	userInfo, err := h.getUserInfoFromToken(jweToken)
	if err != nil {
		rw.WriteHeader(http.StatusUnauthorized)
		rw.Write(responseBody(TokenResponse{Errors: []string{err.Error()}}))
		return
	}

	ctx := request.WithUser(r.Context(), userInfo)
	r = r.WithContext(ctx)
	handler.ServeHTTP(rw, r)
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

	userInfo, err = getUserFromAuthInfo(authInfo)
	if err != nil {
		return
	}
	return
}

func getUserFromAuthInfo(authInfo *api.AuthInfo) (user.Info, error) {
	if authInfo.Token != "" {
		claims, err := jwt.GetJWTTokenClaims(authInfo.Token)
		if err != nil {
			return nil, err
		}
		subject, ok1 := claims[jwtServiceAccountClaimSubject]
		if ok1 {
			return &user.DefaultInfo{
				Name: subject.(string),
			}, nil
		}
	}

	if len(authInfo.ClientCertificateData) != 0 && len(authInfo.ClientKeyData) != 0 {
		block, _ := pem.Decode(authInfo.ClientCertificateData)
		clientCert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, errors.Wrapf(err, "kubeconfig's ClientCertificateData is not a valid CA")
		}

		if clientCert.Subject.CommonName == "" {
			return nil, errors.New("Subject.CommonName from kubeconfig's ClientCertificateData is empty")
		}

		return &user.DefaultInfo{Name: clientCert.Subject.CommonName, Groups: clientCert.Subject.Organization}, nil
	}

	return nil, errors.New("Failed to get userInfo from authInfo")
}
