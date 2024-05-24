package auth

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"strings"

	"golang.org/x/crypto/pkcs12"
	"golang.org/x/oauth2"

	"github.com/manicminer/hamilton/environments"
)

// Authorizer is anything that can return an access token for authorizing API connections
type Authorizer interface {
	Token() (*oauth2.Token, error)
	AuxiliaryTokens() ([]*oauth2.Token, error)
}

// NewAuthorizer returns a suitable Authorizer depending on what is defined in the Config
// Authorizers are selected for authentication methods in the following preferential order:
// - Client certificate authentication
// - Client secret authentication
// - GitHub OIDC authentication
// - MSI authentication
// - Azure CLI authentication
//
// Whether one of these is returned depends on whether it is enabled in the Config, and whether sufficient
// configuration fields are set to enable that authentication method.
//
// For client certificate authentication, specify TenantID, ClientID and ClientCertData / ClientCertPath.
// For client secret authentication, specify TenantID, ClientID and ClientSecret.
// For GitHub OIDC authentication, specify TenantID, ClientID, IDTokenRequestURL and IDTokenRequestToken.
// MSI authentication (if enabled) using the Azure Metadata Service is then attempted
// Azure CLI authentication (if enabled) is attempted last
//
// It's recommended to only enable the mechanisms you have configured and are known to work in the execution
// environment. If any authentication mechanism fails due to misconfiguration or some other error, the function
// will return (nil, error) and later mechanisms will not be attempted.
func (c *Config) NewAuthorizer(ctx context.Context, api environments.Api) (Authorizer, error) {
	if c.EnableClientCertAuth && strings.TrimSpace(c.TenantID) != "" && strings.TrimSpace(c.ClientID) != "" && (len(c.ClientCertData) > 0 || strings.TrimSpace(c.ClientCertPath) != "") {
		a, err := NewClientCertificateAuthorizer(ctx, c.Environment, api, c.Version, c.TenantID, c.AuxiliaryTenantIDs, c.ClientID, c.ClientCertData, c.ClientCertPath, c.ClientCertPassword)
		if err != nil {
			return nil, fmt.Errorf("could not configure ClientCertificate Authorizer: %s", err)
		}
		if a != nil {
			return a, nil
		}
	}

	if c.EnableClientSecretAuth && strings.TrimSpace(c.TenantID) != "" && strings.TrimSpace(c.ClientID) != "" && strings.TrimSpace(c.ClientSecret) != "" {
		a, err := NewClientSecretAuthorizer(ctx, c.Environment, api, c.Version, c.TenantID, c.AuxiliaryTenantIDs, c.ClientID, c.ClientSecret)
		if err != nil {
			return nil, fmt.Errorf("could not configure ClientCertificate Authorizer: %s", err)
		}
		if a != nil {
			return a, nil
		}
	}

	if c.EnableGitHubOIDCAuth {
		a, err := NewGitHubOIDCAuthorizer(context.Background(), c.Environment, api, c.TenantID, c.AuxiliaryTenantIDs, c.ClientID, c.IDTokenRequestURL, c.IDTokenRequestToken)
		if err != nil {
			return nil, fmt.Errorf("could not configure GitHubOIDC Authorizer: %s", err)
		}
		if a != nil {
			return a, nil
		}
	}

	if c.EnableMsiAuth {
		a, err := NewMsiAuthorizer(ctx, api, c.MsiEndpoint, c.ClientID)
		if err != nil {
			return nil, fmt.Errorf("could not configure MSI Authorizer: %s", err)
		}
		if a != nil {
			return a, nil
		}
	}

	if c.EnableAzureCliToken {
		a, err := NewAzureCliAuthorizer(ctx, api, c.TenantID)
		if err != nil {
			return nil, fmt.Errorf("could not configure AzureCli Authorizer: %s", err)
		}
		if a != nil {
			return a, nil
		}
	}

	return nil, fmt.Errorf("no Authorizer could be configured, please check your configuration")
}

// NewAzureCliAuthorizer returns an Authorizer which authenticates using the Azure CLI.
func NewAzureCliAuthorizer(ctx context.Context, api environments.Api, tenantId string) (Authorizer, error) {
	conf, err := NewAzureCliConfig(api, tenantId)
	if err != nil {
		return nil, err
	}
	return conf.TokenSource(ctx), nil
}

// NewMsiAuthorizer returns an authorizer which uses managed service identity to for authentication.
func NewMsiAuthorizer(ctx context.Context, api environments.Api, msiEndpoint, clientId string) (Authorizer, error) {
	conf, err := NewMsiConfig(api.Resource(), msiEndpoint, clientId)
	if err != nil {
		return nil, err
	}
	return conf.TokenSource(ctx), nil
}

// NewClientCertificateAuthorizer returns an authorizer which uses client certificate authentication.
func NewClientCertificateAuthorizer(ctx context.Context, environment environments.Environment, api environments.Api, tokenVersion TokenVersion, tenantId string, auxTenantIds []string, clientId string, pfxData []byte, pfxPath, pfxPass string) (Authorizer, error) {
	if len(pfxData) == 0 {
		var err error
		pfxData, err = ioutil.ReadFile(pfxPath)
		if err != nil {
			return nil, fmt.Errorf("could not read pkcs12 store at %q: %s", pfxPath, err)
		}
	}

	key, cert, err := pkcs12.Decode(pfxData, pfxPass)
	if err != nil {
		return nil, fmt.Errorf("could not decode pkcs12 credential store: %s", err)
	}

	priv, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("unsupported non-rsa key was found in pkcs12 store %q", pfxPath)
	}

	conf := ClientCredentialsConfig{
		Environment:        environment,
		TenantID:           tenantId,
		AuxiliaryTenantIDs: auxTenantIds,
		ClientID:           clientId,
		PrivateKey:         x509.MarshalPKCS1PrivateKey(priv),
		Certificate:        cert.Raw,
		Resource:           api.Resource(),
		Scopes:             []string{api.DefaultScope()},
		TokenVersion:       tokenVersion,
	}

	return conf.TokenSource(ctx, ClientCredentialsAssertionType), nil
}

// NewClientSecretAuthorizer returns an authorizer which uses client secret authentication.
func NewClientSecretAuthorizer(ctx context.Context, environment environments.Environment, api environments.Api, tokenVersion TokenVersion, tenantId string, auxTenantIds []string, clientId, clientSecret string) (Authorizer, error) {
	conf := ClientCredentialsConfig{
		Environment:        environment,
		TenantID:           tenantId,
		AuxiliaryTenantIDs: auxTenantIds,
		ClientID:           clientId,
		ClientSecret:       clientSecret,
		Resource:           api.Resource(),
		Scopes:             []string{api.DefaultScope()},
		TokenVersion:       tokenVersion,
	}

	return conf.TokenSource(ctx, ClientCredentialsSecretType), nil
}

// NewGitHubOIDCAuthorizer returns an authorizer which acquires a client assertion from a GitHub endpoint, then uses client assertion authentication to obtain an access token.
func NewGitHubOIDCAuthorizer(ctx context.Context, environment environments.Environment, api environments.Api, tenantId string, auxTenantIds []string, clientId, idTokenRequestUrl, idTokenRequestToken string) (Authorizer, error) {
	conf := GitHubOIDCConfig{
		Environment:         environment,
		TenantID:            tenantId,
		AuxiliaryTenantIDs:  auxTenantIds,
		ClientID:            clientId,
		IDTokenRequestURL:   idTokenRequestUrl,
		IDTokenRequestToken: idTokenRequestToken,
		Scopes:              []string{api.DefaultScope()},
	}

	return conf.TokenSource(ctx), nil
}

func TokenEndpoint(endpoint environments.AzureADEndpoint, tenant string, version TokenVersion) (e string) {
	if tenant == "" {
		tenant = "common"
	}
	e = fmt.Sprintf("%s/%s/oauth2", endpoint, tenant)
	if version == TokenVersion2 {
		e = fmt.Sprintf("%s/%s", e, "v2.0")
	}
	e = fmt.Sprintf("%s/token", e)
	return
}
