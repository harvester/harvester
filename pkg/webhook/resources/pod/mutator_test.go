package pod

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/harvester/harvester/pkg/webhook/types"
)

func Test_envPatches(t *testing.T) {

	type input struct {
		targetEnvs []corev1.EnvVar
		proxyEnvs  []corev1.EnvVar
		basePath   string
	}
	var testCases = []struct {
		name   string
		input  input
		output types.PatchOps
	}{
		{
			name: "add proxy envs",
			input: input{
				targetEnvs: []corev1.EnvVar{
					{
						Name:  "foo",
						Value: "bar",
					},
				},
				proxyEnvs: []corev1.EnvVar{
					{
						Name:  "HTTP_PROXY",
						Value: "http://192.168.0.1:3128",
					},
					{
						Name:  "HTTPS_PROXY",
						Value: "http://192.168.0.1:3128",
					},
					{
						Name:  "NO_PROXY",
						Value: "127.0.0.1,0.0.0.0,10.0.0.0/8",
					},
				},
				basePath: "/spec/containers/0/env",
			},
			output: []string{
				`{"op": "add", "path": "/spec/containers/0/env/-", "value": {"name":"HTTP_PROXY","value":"http://192.168.0.1:3128"}}`,
				`{"op": "add", "path": "/spec/containers/0/env/-", "value": {"name":"HTTPS_PROXY","value":"http://192.168.0.1:3128"}}`,
				`{"op": "add", "path": "/spec/containers/0/env/-", "value": {"name":"NO_PROXY","value":"127.0.0.1,0.0.0.0,10.0.0.0/8"}}`,
			},
		},
		{
			name: "add proxy envs to empty envs",
			input: input{
				targetEnvs: []corev1.EnvVar{},
				proxyEnvs: []corev1.EnvVar{
					{
						Name:  "HTTP_PROXY",
						Value: "http://192.168.0.1:3128",
					},
					{
						Name:  "HTTPS_PROXY",
						Value: "http://192.168.0.1:3128",
					},
					{
						Name:  "NO_PROXY",
						Value: "127.0.0.1,0.0.0.0,10.0.0.0/8",
					},
				},
				basePath: "/spec/containers/0/env",
			},
			output: []string{
				`{"op": "add", "path": "/spec/containers/0/env", "value": [{"name":"HTTP_PROXY","value":"http://192.168.0.1:3128"}]}`,
				`{"op": "add", "path": "/spec/containers/0/env/-", "value": {"name":"HTTPS_PROXY","value":"http://192.168.0.1:3128"}}`,
				`{"op": "add", "path": "/spec/containers/0/env/-", "value": {"name":"NO_PROXY","value":"127.0.0.1,0.0.0.0,10.0.0.0/8"}}`,
			},
		},
	}
	for _, testCase := range testCases {
		result, err := envPatches(testCase.input.targetEnvs, testCase.input.proxyEnvs, testCase.input.basePath)
		assert.Equal(t, testCase.output, result)
		assert.Empty(t, err)
	}
}
