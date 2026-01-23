package pod

import (
	"fmt"
	"strings"
	"testing"

	"sigs.k8s.io/json"

	"github.com/rancher/wrangler/v3/pkg/patch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

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
							DefaultMode: ptr.To(int32(400)),
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
							DefaultMode: ptr.To(int32(400)),
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

func Test_shouldPatch(t *testing.T) {
	type input struct {
		pod corev1.Pod
	}
	var testCases = []struct {
		name   string
		input  input
		output bool
	}{
		{
			name: "should patch some pods on longhorn-system namespace",
			input: input{
				pod: corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "longhorn-system",
						Labels: map[string]string{
							"longhorn.io/component": "backing-image-data-source",
						},
					},
				},
			},
			output: true,
		},
		{
			name: "should patch some pods on harvester-system namespace",
			input: input{
				pod: corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "harvester-system",
						Labels: map[string]string{
							"app.kubernetes.io/name":      "harvester",
							"app.kubernetes.io/component": "apiserver",
						},
					},
				},
			},
			output: true,
		},
		{
			name: "should patch pods with label app=rancher on cattle-system namespace",
			input: input{
				pod: corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cattle-system",
						Labels: map[string]string{
							"app": "rancher",
						},
					},
				},
			},
			output: true,
		},
		{
			name: "should not patch pods with label app=rancher on other namespaces",
			input: input{
				pod: corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Labels: map[string]string{
							"app": "rancher",
						},
					},
				},
			},
			output: false,
		},
	}
	for _, testCase := range testCases {
		result := shouldPatch(&testCase.input.pod)
		assert.Equal(t, testCase.output, result, "case %q", testCase.name)
	}
}

const (
	podJSON = `{
    "apiVersion": "v1",
    "kind": "Pod",
    "metadata": {
        "annotations": {
            "cni.projectcalico.org/containerID": "1e516216321c29a5672c435447d77c95880b184e0005e7fe04f2507d46b92d33",
            "cni.projectcalico.org/podIP": "",
            "cni.projectcalico.org/podIPs": "",
            "k8s.v1.cni.cncf.io/network-status": "[{\n    \"name\": \"k8s-pod-network\",\n    \"ips\": [\n        \"10.52.1.181\"\n    ],\n    \"default\": true,\n    \"dns\": {}\n}]"
        },
        "creationTimestamp": "2026-02-01T23:37:56Z",
        "generateName": "vmware-gm-ubuntu-test-migration-vm-13007-",
        "generation": 1,
        "labels": {
            "forklift.app": "virt-v2v",
            "migration": "3abfbc07-fb99-46db-bd30-b69242686b7d",
            "plan": "e3f762d7-4a72-47c2-a333-0651c9614bcc",
            "vmID": "vm-13007"
        },
        "name": "vmware-gm-ubuntu-test-migration-vm-13007-p7r8g",
        "namespace": "default",
        "resourceVersion": "19875589",
        "uid": "7f9eba5e-0564-4b5f-8526-b44331eea8a1"
    },
    "spec": {
        "affinity": {
            "podAntiAffinity": {
                "preferredDuringSchedulingIgnoredDuringExecution": [
                    {
                        "podAffinityTerm": {
                            "labelSelector": {
                                "matchExpressions": [
                                    {
                                        "key": "forklift.app",
                                        "operator": "In",
                                        "values": [
                                            "virt-v2v"
                                        ]
                                    }
                                ]
                            },
                            "namespaceSelector": {},
                            "topologyKey": "kubernetes.io/hostname"
                        },
                        "weight": 100
                    }
                ]
            }
        },
        "containers": [
            {
                "env": [
                    {
                        "name": "V2V_HOSTNAME",
                        "value": "ubuntu"
                    },
                    {
                        "name": "V2V_vmName",
                        "value": "gm-ubuntu-test"
                    },
                    {
                        "name": "V2V_libvirtURL",
                        "value": "vpx://fakeendpoint"
                    },
                    {
                        "name": "V2V_source",
                        "value": "vSphere"
                    },
                    {
                        "name": "V2V_fingerprint",
                        "value": "fake-v2v-fingerprint"
                    },
                    {
                        "name": "V2V_extra_args",
                        "value": "[]"
                    },
                    {
                        "name": "LOCAL_MIGRATION",
                        "value": "true"
                    }
                ],
                "envFrom": [
                    {
                        "prefix": "V2V_",
                        "secretRef": {
                            "name": "vmware-gm-ubuntu-test-migration-vm-13007-drhh2"
                        }
                    }
                ],
                "image": "gmehta3/harvester-forklift-virt-v2v:main-head",
                "imagePullPolicy": "Always",
                "name": "virt-v2v",
                "ports": [
                    {
                        "containerPort": 2112,
                        "name": "metrics",
                        "protocol": "TCP"
                    }
                ],
                "resources": {
                    "limits": {
                        "cpu": "4",
                        "devices.kubevirt.io/kvm": "1",
                        "memory": "8Gi"
                    },
                    "requests": {
                        "cpu": "1",
                        "devices.kubevirt.io/kvm": "1",
                        "memory": "1Gi"
                    }
                },
                "securityContext": {
                    "allowPrivilegeEscalation": false,
                    "capabilities": {
                        "drop": [
                            "ALL"
                        ]
                    }
                },
                "terminationMessagePath": "/dev/termination-log",
                "terminationMessagePolicy": "File",
                "volumeDevices": [
                    {
                        "devicePath": "/dev/block0",
                        "name": "vmware-gm-ubuntu-test-migration-vm-13007-ndzrv"
                    }
                ],
                "volumeMounts": [
                    {
                        "mountPath": "/mnt/v2v",
                        "name": "libvirt-domain-xml"
                    },
                    {
                        "mountPath": "/opt",
                        "name": "vddk-vol-mount"
                    },
                    {
                        "mountPath": "/etc/secret",
                        "name": "secret-volume",
                        "readOnly": true
                    },
                    {
                        "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount",
                        "name": "kube-api-access-7bhl8",
                        "readOnly": true
                    }
                ]
            }
        ],
        "dnsPolicy": "ClusterFirst",
        "enableServiceLinks": true,
        "nodeName": "dell-140-tink-system",
        "nodeSelector": {
            "kubevirt.io/schedulable": "true"
        },
        "preemptionPolicy": "PreemptLowerPriority",
        "priority": 0,
        "restartPolicy": "Never",
        "schedulerName": "default-scheduler",
        "securityContext": {
            "fsGroup": 107,
            "runAsNonRoot": true,
            "runAsUser": 107,
            "seccompProfile": {
                "type": "RuntimeDefault"
            }
        },
        "serviceAccount": "default",
        "serviceAccountName": "default",
        "terminationGracePeriodSeconds": 30,
        "tolerations": [
            {
                "effect": "NoExecute",
                "key": "node.kubernetes.io/not-ready",
                "operator": "Exists",
                "tolerationSeconds": 300
            },
            {
                "effect": "NoExecute",
                "key": "node.kubernetes.io/unreachable",
                "operator": "Exists",
                "tolerationSeconds": 300
            }
        ],
        "volumes": [
            {
                "name": "vmware-gm-ubuntu-test-migration-vm-13007-ndzrv",
                "persistentVolumeClaim": {
                    "claimName": "vmware-gm-ubuntu-test-migration-vm-13007-ndzrv"
                }
            },
            {
                "configMap": {
                    "defaultMode": 420,
                    "name": "vmware-gm-ubuntu-test-migration-vm-13007-clsv4"
                },
                "name": "libvirt-domain-xml"
            },
            {
                "emptyDir": {},
                "name": "vddk-vol-mount"
            },
            {
                "name": "secret-volume",
                "secret": {
                    "defaultMode": 420,
                    "secretName": "vmware-gm-ubuntu-test-migration-vm-13007-drhh2"
                }
            },
            {
                "name": "kube-api-access-7bhl8",
                "projected": {
                    "defaultMode": 420,
                    "sources": [
                        {
                            "serviceAccountToken": {
                                "expirationSeconds": 3607,
                                "path": "token"
                            }
                        },
                        {
                            "configMap": {
                                "items": [
                                    {
                                        "key": "ca.crt",
                                        "path": "ca.crt"
                                    }
                                ],
                                "name": "kube-root-ca.crt"
                            }
                        },
                        {
                            "downwardAPI": {
                                "items": [
                                    {
                                        "fieldRef": {
                                            "apiVersion": "v1",
                                            "fieldPath": "metadata.namespace"
                                        },
                                        "path": "namespace"
                                    }
                                ]
                            }
                        }
                    ]
                }
            }
        ]
    }
}`
)

func Test_generateForkliftV2VPatch(t *testing.T) {
	assert := require.New(t)
	sourcePod, err := generatePod([]byte(podJSON))
	assert.NoError(err, "expected no error during generation of source pod")
	patchOps, err := generateForkliftV2VPatch(sourcePod)
	assert.NotEmpty(patchOps, "expected to find patchops for forklift v2v pod")
	assert.NoError(err, "expected no error during generation of patch")
	patchData := fmt.Sprintf("[%s]", strings.Join(patchOps, ","))
	patchedPodBytes, err := patch.Apply([]byte(podJSON), []byte(patchData))
	assert.NoError(err, "expected no error during application of patch to source pod")
	targetPod, err := generatePod(patchedPodBytes)
	assert.NoError(err, "expected no error during generation of target pod")
	assert.Equal(corev1.SeccompProfileTypeUnconfined, targetPod.Spec.SecurityContext.SeccompProfile.Type, "expected pod seccomp profile to be unconfined")
	for i := range targetPod.Spec.Containers {
		assert.Equal(true, *targetPod.Spec.Containers[i].SecurityContext.Privileged, "expected to find privileged container security context")
		assert.Equal(corev1.PullIfNotPresent, targetPod.Spec.Containers[i].ImagePullPolicy, "expected to find imagePullPolicy IfNotPresent")
	}

}

func generatePod(content []byte) (*corev1.Pod, error) {
	pod := &corev1.Pod{}
	err := json.UnmarshalCaseSensitivePreserveInts(content, pod)
	return pod, err
}
