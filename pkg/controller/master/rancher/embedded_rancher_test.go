package rancher

import (
	"fmt"
	"testing"

	rancherv3api "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	capiDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      capiControllerDeploymentName,
			Namespace: capiControllerDeploymentNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "manager",
							Env: []corev1.EnvVar{
								{
									Name: "POD_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
								{
									Name: "POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name: "POD_UID",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.uid",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	nonCapiDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello-world",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "manager",
							Env: []corev1.EnvVar{
								{
									Name: "POD_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
								{
									Name: "POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name: "POD_UID",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.uid",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
)

func Test_PatchCAPIDeployment(t *testing.T) {
	fake := fake.NewSimpleClientset(capiDeployment, nonCapiDeployment)
	deployment := fakeclients.DeploymentClient(fake.AppsV1().Deployments)
	h := &Handler{
		Deployments: deployment,
	}

	var tests = []struct {
		Name             string
		Deployment       *appsv1.Deployment
		ExpectEnvRemoved bool
	}{
		{
			Name:             "capi-deployment",
			Deployment:       capiDeployment,
			ExpectEnvRemoved: true,
		},
		{
			Name:             "non capi-deployment",
			Deployment:       nonCapiDeployment,
			ExpectEnvRemoved: false,
		},
	}

	assert := require.New(t)

	for _, tt := range tests {
		deploymentObj, err := h.PatchCAPIDeployment("", tt.Deployment)
		assert.NoError(err, "expected no error during reconcile of deployment in case %s", tt.Name)
		if tt.ExpectEnvRemoved {
			for _, v := range deploymentObj.Spec.Template.Spec.Containers {
				assert.Empty(v.Env, fmt.Sprintf("expected to find no env variables in test case: %s", tt.Name))
			}
		} else {
			for _, v := range deploymentObj.Spec.Template.Spec.Containers {
				assert.NotEmpty(v.Env, fmt.Sprintf("expected to find no env variables in test case: %s", tt.Name))
			}
		}
	}
}

func Test_initializeTlsInternalCnAllowedServices(t *testing.T) {
	var tests = []struct {
		Name     string
		Given    *rancherv3api.Setting
		Expected *rancherv3api.Setting
	}{
		{
			Name: "empty value",
			Given: &rancherv3api.Setting{
				Value: "",
			},
			Expected: &rancherv3api.Setting{
				Value: "kube-system/rke2-traefik",
			},
		},
		{
			Name: "should keep existing values and append with kube-system/rke2-traefik",
			Given: &rancherv3api.Setting{
				Value: "default/abc-service,service-in-the-same-ns",
			},
			Expected: &rancherv3api.Setting{
				Value: "default/abc-service,service-in-the-same-ns,kube-system/rke2-traefik",
			},
		},
		{
			Name: "do nothing if kube-system/rke2-traefik is already present",
			Given: &rancherv3api.Setting{
				Value: "default/abc-service,kube-system/rke2-traefik,service-in-the-same-ns",
			},
			Expected: &rancherv3api.Setting{
				Value: "default/abc-service,kube-system/rke2-traefik,service-in-the-same-ns",
			},
		},
	}

	assert := require.New(t)
	for _, tt := range tests {
		objs := []runtime.Object{tt.Given}
		clientset := fake.NewSimpleClientset(objs...)
		handler := &Handler{
			RancherSettings: fakeclients.RancherSettingClient(clientset.ManagementV3().Settings),
		}
		actual, err := handler.initializeTlsInternalCnAllowedServices(tt.Given)
		assert.NoError(err, "expected no error in case %s", tt.Name)
		assert.Equal(tt.Expected.Value, actual.Value, "didn't get expected Value in case %s", tt.Name)
	}
}
