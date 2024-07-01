package secret

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/util"
)

func Test_validateNodeCPUManagerPolicies(t *testing.T) {
	objectMeta := metav1.ObjectMeta{
		Name:      util.NodeCPUManagerPoliciesSecretName,
		Namespace: util.CattleSystemNamespaceName,
	}

	testCases := []struct {
		name   string
		secret *v1.Secret
		errMsg string
	}{
		{
			name: "nil secret",
			secret: &v1.Secret{
				ObjectMeta: objectMeta,
				Data:       map[string][]byte{},
			},
			errMsg: "node cpu manager policies is nil",
		},
		{
			name: "incorrect json string",
			secret: &v1.Secret{
				ObjectMeta: objectMeta,
				Data: map[string][]byte{
					util.NodeCPUManagerPoliciesSecretName: []byte(""),
				},
			},
			errMsg: "json parse error: EOF",
		},
		{
			name: "incorrect json configs",
			secret: &v1.Secret{
				ObjectMeta: objectMeta,
				Data: map[string][]byte{
					util.NodeCPUManagerPoliciesSecretName: []byte(`{"foo": "bar"}`),
				},
			},
			errMsg: "json parse error: json: unknown field \"foo\"",
		},
		{
			name: "invalid policy",
			secret: &v1.Secret{
				ObjectMeta: objectMeta,
				Data: map[string][]byte{
					util.NodeCPUManagerPoliciesSecretName: []byte(`{"configs": [{"name": "node-1", "policy": "bar"}]}`),
				},
			},
			errMsg: "cpu manager policy should be either none or static",
		},
		{
			name: "empty configs",
			secret: &v1.Secret{
				ObjectMeta: objectMeta,
				Data: map[string][]byte{
					util.NodeCPUManagerPoliciesSecretName: []byte("{}"),
				},
			},
			errMsg: "",
		},
		{
			name: "valid policy",
			secret: &v1.Secret{
				ObjectMeta: objectMeta,
				Data: map[string][]byte{
					util.NodeCPUManagerPoliciesSecretName: []byte(`
					{"configs": [
						{"name": "node-1", "policy": "static"},
						{"name": "node-2", "policy": "none"}
					  ]
					}`),
				},
			},
			errMsg: "",
		},
		{
			name: "StringData and Data both exist, and value in StringData is incorrect",
			secret: &v1.Secret{
				ObjectMeta: objectMeta,
				StringData: map[string]string{
					util.NodeCPUManagerPoliciesSecretName: string(`{"foo": "bar"}`),
				},
				Data: map[string][]byte{
					util.NodeCPUManagerPoliciesSecretName: []byte(`{"configs": [{"name": "node-1", "policy": "static"}]}`),
				},
			},
			errMsg: "json parse error: json: unknown field \"foo\"",
		},
		{
			name: "StringData and Data both exist, and value in data is incorrect",
			secret: &v1.Secret{
				ObjectMeta: objectMeta,
				StringData: map[string]string{
					util.NodeCPUManagerPoliciesSecretName: string(`{"configs": [{"name": "node-1", "policy": "static"}]}`),
				},
				Data: map[string][]byte{
					util.NodeCPUManagerPoliciesSecretName: []byte(`{"foo": "bar"}`),
				},
			},
			errMsg: "",
		},
		{
			name: "incorrect namespace",
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.NodeCPUManagerPoliciesSecretName,
					Namespace: "default",
				},
			},
			errMsg: "node-cpu-manager-policies namespace should be cattle-system",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateNodeCPUManagerPolicies(testCase.secret)
			if testCase.errMsg != "" {
				assert.Equal(t, testCase.errMsg, err.Error())
			} else {
				assert.Nil(t, err)
			}
		})

	}
}
