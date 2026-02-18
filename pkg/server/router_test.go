package server

import (
	"testing"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTokenValidation(t *testing.T) {
	const testToken = "test-server-token-12345"

	tests := []struct {
		name          string
		secret        *corev1.Secret
		providedToken string
		expectLoadErr bool
		expectValErr  bool
	}{
		{
			name:          "Valid token matches",
			secret:        createTokenSecret(testToken),
			providedToken: testToken,
			expectLoadErr: false,
			expectValErr:  false,
		},
		{
			name:          "Invalid token doesn't match",
			secret:        createTokenSecret(testToken),
			providedToken: "wrong-token",
			expectLoadErr: false,
			expectValErr:  true,
		},
		{
			name:          "Secret not found",
			secret:        nil,
			providedToken: testToken,
			expectLoadErr: true,
			expectValErr:  false,
		},
		{
			name: "Secret missing token key",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      localRKEStateSecretName,
					Namespace: util.FleetLocalNamespaceName,
				},
				Data: map[string][]byte{
					"wrongKey": []byte("some-value"),
				},
			},
			providedToken: testToken,
			expectLoadErr: true,
			expectValErr:  false,
		},
		{
			name: "Secret with empty token",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      localRKEStateSecretName,
					Namespace: util.FleetLocalNamespaceName,
				},
				Data: map[string][]byte{
					serverTokenKey: []byte(""),
				},
			},
			providedToken: testToken,
			expectLoadErr: true,
			expectValErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()

			if tt.secret != nil {
				err := clientset.Tracker().Add(tt.secret)
				require.NoError(t, err, "Failed to add secret to fake clientset")
			}

			secretCache := fakeclients.SecretCache(clientset.CoreV1().Secrets)

			actualToken, err := getTokenFromSecret(secretCache)
			if tt.expectLoadErr {
				assert.Error(t, err, "Expected error during token load")
				return
			}
			assert.NoError(t, err, "Expected no error during token load")

			err = validateToken(actualToken, tt.providedToken)
			if tt.expectValErr {
				assert.Error(t, err, "Expected error during token validation")
			} else {
				assert.NoError(t, err, "Expected no error during token validation")
			}
		})
	}
}

func createTokenSecret(token string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      localRKEStateSecretName,
			Namespace: util.FleetLocalNamespaceName,
		},
		Data: map[string][]byte{
			serverTokenKey: []byte(token),
		},
	}
}
