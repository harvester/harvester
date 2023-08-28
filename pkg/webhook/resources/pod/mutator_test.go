package pod

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterhciv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
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

func Test_sideCarPatch(t *testing.T) {
	assert := require.New(t)
	var testCases = []struct {
		name           string
		input          *corev1.Pod
		vm             *kubevirtv1.VirtualMachine
		expectPatchLen int
		expectError    bool
	}{
		{
			name: "no patch needed",
			input: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-patch-needed",
					Namespace: "default",
					Labels: map[string]string{
						vmLabelPrefix:    "no-patch-needed",
						kubeVirtLabelKey: kubeVirtLabelValue,
					},
				},
			},
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-patch-needed",
					Namespace: "default",
				},
			},
			expectPatchLen: 0,
			expectError:    false,
		},
		{
			name: "patch needed",
			input: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "patch-needed",
					Namespace: "default",
					Labels: map[string]string{
						vmLabelPrefix:    "patch-needed",
						kubeVirtLabelKey: kubeVirtLabelValue,
					},
				},
			},
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "patch-needed",
					Namespace: "default",
					Annotations: map[string]string{
						harvesterv1beta1.SecurityGroupPrefix: "default-sg",
					},
				},
			},
			expectPatchLen: 5,
			expectError:    false,
		},
	}

	clientset := fake.NewSimpleClientset()
	fakeSettingsCache := fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings)
	fakeVMCache := fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines)

	vmNetworkPolicySidecarSetting := &harvesterhciv1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.VMNetworkPolicySideCarSettingName,
		},
		Default: `{"repository":"rancher/harvester-vmnetworkpolicy", "imagePullPolicy":"Always", "tag":"dev"}`,
	}

	m := &podMutator{
		setttingCache: fakeSettingsCache,
		vmCache:       fakeVMCache,
	}
	err := clientset.Tracker().Add(vmNetworkPolicySidecarSetting)
	assert.NoError(err, "expected no error while adding setting")
	logrus.SetLevel(logrus.DebugLevel)

	for _, v := range testCases {
		if v.vm != nil {
			var err = clientset.Tracker().Add(v.vm)
			assert.Nil(err, "expect no error while adding resource to fake client")
		}
		patches, err := m.sideCarPatches(v.input)
		if v.expectError {
			assert.NotEmpty(err, "expected to find error", v.name)
		} else {
			assert.Empty(err, "expected to find no error", v.name)
		}
		assert.Len(patches, v.expectPatchLen, "expected patch length to patch", v.name)

	}
}
