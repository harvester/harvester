package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=user;users,scope=Cluster
// +kubebuilder:printcolumn:name="DISPLAY_NAME",type=string,JSONPath=`.displayName`
// +kubebuilder:printcolumn:name="USERNAME",type=string,JSONPath=`.username`
// +kubebuilder:printcolumn:name="DESCRIPTION",type=string,JSONPath=`.description`

type User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	DisplayName string `json:"displayName,omitempty"`

	// +optional
	Description string `json:"description,omitempty"`

	// +kubebuilder:validation:Required
	Username string `json:"username,omitempty"`

	// +kubebuilder:validation:Required
	Password string `json:"password,omitempty"`

	// +optional
	IsAdmin bool `json:"isAdmin,omitempty"`
}

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
	LocalUser             AuthenticationMode = "localUser"
	Rancher               AuthenticationMode = "rancher"
)

type AuthenticationModesResponse struct {
	Modes []AuthenticationMode `json:"modes,omitempty"`
}
