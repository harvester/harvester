package supportbundle

import (
	"fmt"
	"strings"

	ctlappsv1 "github.com/rancher/wrangler/pkg/generated/controllers/apps/v1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/version"
)

const (
	HarvesterServiceAccount = "harvester"
)

type Manager struct {
	deployments ctlappsv1.DeploymentClient
	nodeCache   ctlcorev1.NodeCache
	services    ctlcorev1.ServiceClient
}

func (m *Manager) getManagerName(sb *harvesterv1.SupportBundle) string {
	return fmt.Sprintf("supportbundle-manager-%s", sb.Name)
}

func (m *Manager) Create(sb *harvesterv1.SupportBundle, image string) error {
	deployName := m.getManagerName(sb)
	logrus.Debugf("creating deployment %s with image %s", deployName, image)

	pullPolicy := m.getImagePullPolicy()

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: sb.Namespace,
			Labels: map[string]string{
				"app":                 AppManager,
				SupportBundleLabelKey: sb.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       sb.Name,
					Kind:       sb.Kind,
					UID:        sb.UID,
					APIVersion: sb.APIVersion,
				},
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": AppManager},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                 AppManager,
						SupportBundleLabelKey: sb.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "manager",
							Image:           image,
							Args:            []string{"/usr/bin/support-bundle-utils", "manager"},
							ImagePullPolicy: pullPolicy,
							Env: []corev1.EnvVar{
								{
									Name:  "HARVESTER_NAMESPACE",
									Value: sb.Namespace,
								},
								{
									Name:  "HARVESTER_VERSION",
									Value: version.FriendlyVersion(),
								},
								{
									Name:  "HARVESTER_SUPPORT_BUNDLE_NAME",
									Value: sb.Name,
								},
								{
									Name:  "HARVESTER_SUPPORT_BUNDLE_DEBUG",
									Value: "true",
								},
								{
									Name: "HARVESTER_SUPPORT_BUNDLE_MANAGER_POD_IP",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "status.podIP",
										},
									},
								},
								{
									Name:  "HARVESTER_SUPPORT_BUNDLE_IMAGE",
									Value: image,
								},
								{
									Name:  "HARVESTER_SUPPORT_BUNDLE_IMAGE_PULL_POLICY",
									Value: string(pullPolicy),
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
								},
							},
						},
					},
					ServiceAccountName: HarvesterServiceAccount,
				},
			},
		},
	}

	_, err := m.deployments.Create(deployment)
	return err
}

func (m *Manager) getImagePullPolicy() corev1.PullPolicy {
	switch strings.ToLower(settings.SupportBundleImagePullPolicy.Get()) {
	case "always":
		return corev1.PullAlways
	case "ifnotpresent":
		return corev1.PullIfNotPresent
	case "never":
		return corev1.PullNever
	default:
		return corev1.PullIfNotPresent
	}
}
