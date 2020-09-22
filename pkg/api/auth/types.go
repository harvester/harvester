package auth

type Login struct {
	// Token is the bearer token for authentication to the kubernetes cluster, from serviceAccount's secret.
	Token string `json:"token,omitempty"`
	// KubeConfig is the content of users' kubeconfig file.
	// Currently support Kubeconfig with 1. token, 2. clientCertificateData and clientKeyData
	// Can't contain any paths. All data has to be provided within the file.
	KubeConfig string `json:"kubeconfig,omitempty"`
}

type TokenResponse struct {
	// JWE format token generated during login, it AuthInfo data in the payload, need to use private key to decrypt.
	JWEToken string `json:"jweToken,omitempty"`
	// Errors happened during login.
	Errors []string `json:"errors,omitempty"`
}
