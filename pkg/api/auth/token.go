package auth

import (
	"net/http"
	"net/url"

	"github.com/pkg/errors"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	JWETokenHeader = "jweToken"
)

func extractJWETokenFromRequest(req *http.Request) (string, error) {
	var token string
	if tokenAuthValue := req.Header.Get(JWETokenHeader); tokenAuthValue != "" {
		token = tokenAuthValue
	}

	if cookie, err := req.Cookie(JWETokenHeader); err == nil {
		token = cookie.Value
	}

	if token != "" {
		decodedToken, err := url.QueryUnescape(token)
		if err != nil {
			return "", errors.New("Failed to parse jweToken from request")
		}
		return decodedToken, nil
	}

	return "", errors.New("Failed to get JWE token from request")
}

// Based on auth info and rest config creates client cmd config.
func buildCmdConfig(authInfo *clientcmdapi.AuthInfo, cfg *rest.Config) clientcmd.ClientConfig {
	cmdCfg := clientcmdapi.NewConfig()
	cmdCfg.Clusters[defaultRestConfigResourceName] = &clientcmdapi.Cluster{
		Server:                   cfg.Host,
		CertificateAuthority:     cfg.TLSClientConfig.CAFile,
		CertificateAuthorityData: cfg.TLSClientConfig.CAData,
		InsecureSkipTLSVerify:    cfg.TLSClientConfig.Insecure,
	}
	cmdCfg.AuthInfos[defaultRestConfigResourceName] = authInfo
	cmdCfg.Contexts[defaultRestConfigResourceName] = &clientcmdapi.Context{
		Cluster:  defaultRestConfigResourceName,
		AuthInfo: defaultRestConfigResourceName,
	}
	cmdCfg.CurrentContext = defaultRestConfigResourceName

	return clientcmd.NewDefaultClientConfig(
		*cmdCfg,
		&clientcmd.ConfigOverrides{},
	)
}

func buildAuthInfo(token, kubeConfig string) (*clientcmdapi.AuthInfo, error) {
	if token == "" && kubeConfig == "" {
		return nil, errors.New("No authentication information provided")
	}

	if token != "" {
		return &clientcmdapi.AuthInfo{
			Token: token,
		}, nil
	}

	cf, err := clientcmd.NewClientConfigFromBytes([]byte(kubeConfig))
	if err != nil {
		return nil, err
	}

	rawConf, err := cf.RawConfig()
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to get RawConfig from kubConfig file")
	}

	currentCtx, ok := rawConf.Contexts[rawConf.CurrentContext]
	if !ok {
		return nil, errors.New("Failed to find context " + rawConf.CurrentContext)
	}

	authInfo, ok := rawConf.AuthInfos[currentCtx.AuthInfo]
	if !ok {
		return nil, errors.New("Failed to find authInfo for context " + rawConf.CurrentContext)
	}
	return authInfo, nil
}
