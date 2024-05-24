package auth

import "github.com/manicminer/hamilton/environments"

type TokenVersion int

const (
	TokenVersion2 TokenVersion = iota
	TokenVersion1
)

// Config sets up NewAuthorizer to return an Authorizer based on the provided configuration.
type Config struct {
	// Specifies the national cloud environment to use
	Environment environments.Environment

	// Version specifies the token version  to acquire from Microsoft Identity Platform.
	// Ignored when using Azure CLI or Managed Identity authentication.
	Version TokenVersion

	// Azure Active Directory tenant to connect to, should be a valid UUID
	TenantID string

	// Auxiliary tenant IDs for which to obtain tokens in a multi-tenant scenario
	AuxiliaryTenantIDs []string

	// Client ID for the application used to authenticate the connection
	ClientID string

	// Enables authentication using Azure CLI
	EnableAzureCliToken bool

	// Enables authentication using managed service identity.
	EnableMsiAuth bool

	// Specifies a custom MSI endpoint to connect to
	MsiEndpoint string

	// Enables client certificate authentication using client assertions
	EnableClientCertAuth bool

	// Specifies the contents of a client certificate PKCS#12 bundle
	ClientCertData []byte

	// Specifies the path to a client certificate PKCS#12 bundle (.pfx file)
	ClientCertPath string

	// Specifies the encryption password to unlock a client certificate
	ClientCertPassword string

	// Enables client secret authentication using client credentials
	EnableClientSecretAuth bool

	// Specifies the password to authenticate with using client secret authentication
	ClientSecret string

	// Enables GitHub OIDC authentication
	EnableGitHubOIDCAuth bool

	// The URL for GitHub's OIDC provider
	IDTokenRequestURL string

	// The bearer token for the request to GitHub's OIDC provider
	IDTokenRequestToken string
}
