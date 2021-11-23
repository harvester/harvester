package pod

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	"github.com/harvester/harvester/pkg/util"
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
						Name:  util.HTTPProxyEnv,
						Value: "http://192.168.0.1:3128",
					},
					{
						Name:  util.HTTPSProxyEnv,
						Value: "http://192.168.0.1:3128",
					},
					{
						Name:  util.NoProxyEnv,
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
						Name:  util.HTTPProxyEnv,
						Value: "http://192.168.0.1:3128",
					},
					{
						Name:  util.HTTPSProxyEnv,
						Value: "http://192.168.0.1:3128",
					},
					{
						Name:  util.NoProxyEnv,
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

func Test_volumePatch(t *testing.T) {

	type input struct {
		target []corev1.Volume
		volume corev1.Volume
	}
	var testCases = []struct {
		name   string
		input  input
		output string
	}{
		{
			name: "add additional ca volume",
			input: input{
				target: []corev1.Volume{
					{
						Name: "foo",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
				},
				volume: corev1.Volume{
					Name: "additional-ca-volume",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							DefaultMode: pointer.Int32(400),
							SecretName:  util.AdditionalCASecretName,
						},
					},
				},
			},
			output: `{"op": "add", "path": "/spec/volumes/-", "value": {"name":"additional-ca-volume","secret":{"secretName":"harvester-additional-ca","defaultMode":400}}}`,
		},
		{
			name: "add additional ca volume to empty volumes",
			input: input{
				target: []corev1.Volume{},
				volume: corev1.Volume{
					Name: "additional-ca-volume",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							DefaultMode: pointer.Int32(400),
							SecretName:  util.AdditionalCASecretName,
						},
					},
				},
			},
			output: `{"op": "add", "path": "/spec/volumes", "value": [{"name":"additional-ca-volume","secret":{"secretName":"harvester-additional-ca","defaultMode":400}}]}`,
		},
	}
	for _, testCase := range testCases {
		result, err := volumePatch(testCase.input.target, testCase.input.volume)
		assert.Equal(t, testCase.output, result)
		assert.Empty(t, err)
	}
}

func Test_volumeMountPatch(t *testing.T) {

	type input struct {
		target      []corev1.VolumeMount
		volumeMount corev1.VolumeMount
		path        string
	}
	var testCases = []struct {
		name   string
		input  input
		output string
	}{
		{
			name: "add additional ca volume mount",
			input: input{
				target: []corev1.VolumeMount{
					{
						Name:      "foo",
						MountPath: "/bar",
					},
				},
				volumeMount: corev1.VolumeMount{
					Name:      "additional-ca-volume",
					MountPath: "/etc/ssl/certs/" + util.AdditionalCAFileName,
					SubPath:   util.AdditionalCAFileName,
					ReadOnly:  true,
				},
				path: "/spec/containers/0/volumeMounts",
			},
			output: `{"op": "add", "path": "/spec/containers/0/volumeMounts/-", "value": {"name":"additional-ca-volume","readOnly":true,"mountPath":"/etc/ssl/certs/additional-ca.pem","subPath":"additional-ca.pem"}}`,
		},
		{
			name: "add additional ca volume mount to empty volumeMounts",
			input: input{
				target: []corev1.VolumeMount{},
				volumeMount: corev1.VolumeMount{
					Name:      "additional-ca-volume",
					MountPath: "/etc/ssl/certs/" + util.AdditionalCAFileName,
					SubPath:   util.AdditionalCAFileName,
					ReadOnly:  true,
				},
				path: "/spec/containers/0/volumeMounts",
			},
			output: `{"op": "add", "path": "/spec/containers/0/volumeMounts", "value": [{"name":"additional-ca-volume","readOnly":true,"mountPath":"/etc/ssl/certs/additional-ca.pem","subPath":"additional-ca.pem"}]}`,
		},
	}
	for _, testCase := range testCases {
		result, err := volumeMountPatch(testCase.input.target, testCase.input.path, testCase.input.volumeMount)
		assert.Equal(t, testCase.output, result)
		assert.Empty(t, err)
	}
}
