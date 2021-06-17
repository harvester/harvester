package v1beta1

// Login
type Login struct {
	// Token is the bearer token for authentication to the kubernetes cluster, from serviceAccount's secret.
	Token string `json:"token,omitempty"`
	// KubeConfig is the content of users' kubeconfig file.
	// Currently support Kubeconfig with 1. token, 2. clientCertificateData and clientKeyData
	// Can't contain any paths. All data has to be provided within the file.
	KubeConfig string `json:"kubeconfig,omitempty"`
	// Username and password is for local auth.
	Username string `json:"username"`
	Password string `json:"password"`
}

type TokenResponse struct {
	// JWE format token generated during login, it AuthInfo data in the payload, need to use private key to decrypt.
	JWEToken string `json:"jweToken,omitempty"`
}

type ErrorResponse struct {
	// Errors happened during request.
	Errors []string `json:"errors,omitempty"`
}

// AuthenticationMode
type AuthenticationMode string

const (
	KubernetesCredentials AuthenticationMode = "kubernetesCredentials"
	Rancher               AuthenticationMode = "rancher"
)

type AuthenticationModesResponse struct {
	Modes []AuthenticationMode `json:"modes,omitempty"`
}
