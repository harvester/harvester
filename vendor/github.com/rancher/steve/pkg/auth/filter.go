package auth

import (
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/token/cache"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/plugin/pkg/authenticator/token/webhook"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/transport"
)

var (
	// Default value taken from DefaultAuthWebhookRetryBackoff
	WebhookBackoff = wait.Backoff{
		Duration: 500 * time.Millisecond,
		Factor:   1.5,
		Jitter:   0.2,
		Steps:    5,
	}
)

var ExistingContext = ToMiddleware(AuthenticatorFunc(func(req *http.Request) (user.Info, bool, error) {
	user, ok := request.UserFrom(req.Context())
	return user, ok, nil
}))

type Authenticator interface {
	Authenticate(req *http.Request) (user.Info, bool, error)
}

type AuthenticatorFunc func(req *http.Request) (user.Info, bool, error)

func (a AuthenticatorFunc) Authenticate(req *http.Request) (user.Info, bool, error) {
	return a(req)
}

type Middleware func(next http.Handler) http.Handler

func (m Middleware) Chain(middleware Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		return m(middleware(next))
	}
}

func WebhookConfigForURL(url string) (string, error) {
	config := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"local": {
				Server:                url,
				InsecureSkipTLSVerify: true,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"Default": {
				Cluster:   "local",
				AuthInfo:  "user",
				Namespace: "default",
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"user": {},
		},
		CurrentContext: "Default",
	}

	tmpFile, err := ioutil.TempFile("", "webhook-config")
	if err != nil {
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		return "", err
	}

	return tmpFile.Name(), clientcmd.WriteToFile(config, tmpFile.Name())
}

func NewWebhookAuthenticator(cacheTTL time.Duration, kubeConfigFile string) (Authenticator, error) {
	wh, err := webhook.New(kubeConfigFile, "v1", nil, WebhookBackoff, nil)
	if err != nil {
		return nil, err
	}

	if cacheTTL > 0 {
		return &webhookAuth{
			auth: cache.New(wh, false, cacheTTL, cacheTTL),
		}, nil
	}

	return &webhookAuth{
		auth: wh,
	}, nil
}

func NewWebhookMiddleware(cacheTTL time.Duration, kubeConfigFile string) (Middleware, error) {
	auth, err := NewWebhookAuthenticator(cacheTTL, kubeConfigFile)
	if err != nil {
		return nil, err
	}
	return ToMiddleware(auth), nil
}

type webhookAuth struct {
	auth authenticator.Token
}

func (w *webhookAuth) Authenticate(req *http.Request) (user.Info, bool, error) {
	token := req.Header.Get("Authorization")
	if strings.HasPrefix(token, "Bearer ") {
		token = strings.TrimPrefix(token, "Bearer ")
	} else {
		token = ""
	}

	if token == "" {
		cookie, err := req.Cookie("R_SESS")
		if err != nil && err != http.ErrNoCookie {
			return nil, false, err
		} else if err != http.ErrNoCookie && len(cookie.Value) > 0 {
			token = "cookie://" + cookie.Value
		}
	}

	if token == "" {
		return nil, false, nil
	}

	resp, ok, err := w.auth.AuthenticateToken(req.Context(), token)
	if resp == nil {
		return nil, ok, err
	}
	return resp.User, ok, err
}

func ToMiddleware(auth Authenticator) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			info, ok, err := auth.Authenticate(req)
			if err != nil {
				info = &user.DefaultInfo{
					Name: "system:cattle:error",
					UID:  "system:cattle:error",
					Groups: []string{
						"system:unauthenticated",
						"system:cattle:error",
					},
				}
			} else if !ok {
				info = &user.DefaultInfo{
					Name: "system:unauthenticated",
					UID:  "system:unauthenticated",
					Groups: []string{
						"system:unauthenticated",
					},
				}
			}

			ctx := request.WithUser(req.Context(), info)
			req = req.WithContext(ctx)
			next.ServeHTTP(rw, req)
		})
	}
}

func AlwaysAdmin(req *http.Request) (user.Info, bool, error) {
	return &user.DefaultInfo{
		Name: "admin",
		UID:  "admin",
		Groups: []string{
			"system:masters",
			"system:authenticated",
		},
	}, true, nil
}

func Impersonation(req *http.Request) (user.Info, bool, error) {
	userName := req.Header.Get(transport.ImpersonateUserHeader)
	if userName == "" {
		return nil, false, nil
	}

	result := user.DefaultInfo{
		Name:   userName,
		Groups: req.Header[transport.ImpersonateGroupHeader],
		Extra:  map[string][]string{},
	}

	for k, v := range req.Header {
		if strings.HasPrefix(k, transport.ImpersonateUserExtraHeaderPrefix) {
			result.Extra[k[len(transport.ImpersonateUserExtraHeaderPrefix):]] = v
		}
	}

	return &result, true, nil
}
