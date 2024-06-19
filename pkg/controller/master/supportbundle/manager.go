package supportbundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	rancherv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	ctlappsv1 "github.com/rancher/wrangler/pkg/generated/controllers/apps/v1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/supportbundle/types"
	"github.com/harvester/harvester/pkg/settings"
	supportBundleUtil "github.com/harvester/harvester/pkg/util/supportbundle"
)

const (
	HarvesterServiceAccount = "harvester"
	//AdditionalTaintToleration value when passed to SUPPORT_BUNDLE_TAINT_TOLERATION env variable, is processed as v1.TolerationOpExists
	//which allows the support-bundle-kit pods to be scheduled on nodes with custom taints
	AdditionalTaintToleration = ":"
)

type Manager struct {
	deployments ctlappsv1.DeploymentClient
	nodeCache   ctlcorev1.NodeCache
	podCache    ctlcorev1.PodCache
	services    ctlcorev1.ServiceClient

	// to talk with support bundle manager
	httpClient http.Client
}

func (m *Manager) getManagerName(sb *harvesterv1.SupportBundle) string {
	return fmt.Sprintf("supportbundle-manager-%s", sb.Name)
}

func (m *Manager) Create(sb *harvesterv1.SupportBundle, image string, pullPolicy corev1.PullPolicy) error {

	var (
		nodeTimeout time.Duration
		deployName  string
	)

	deployName = m.getManagerName(sb)
	logrus.Debugf("creating deployment %s with image %s", deployName, image)

	if expiration := settings.SupportBundleNodeCollectionTimeout.GetInt(); expiration == 0 {
		nodeTimeout = time.Duration(supportBundleUtil.SupportBundleNodeCollectionTimeoutDefault) * time.Minute
	} else {
		nodeTimeout = time.Duration(expiration) * time.Minute
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: sb.Namespace,
			Labels: map[string]string{
				"app":                       types.AppManager,
				types.SupportBundleLabelKey: sb.Name,
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
				MatchLabels: map[string]string{"app": types.AppManager},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                       types.AppManager,
						types.SupportBundleLabelKey: sb.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "manager",
							Image:           image,
							Args:            []string{"/usr/bin/support-bundle-kit", "manager"},
							ImagePullPolicy: pullPolicy,
							Env: []corev1.EnvVar{
								{
									Name:  "SUPPORT_BUNDLE_TARGET_NAMESPACES",
									Value: m.getCollectNamespaces(),
								},
								{
									Name:  "SUPPORT_BUNDLE_NAME",
									Value: sb.Name,
								},
								{
									Name:  "SUPPORT_BUNDLE_DESCRIPTION",
									Value: sb.Spec.Description,
								},
								{
									Name:  "SUPPORT_BUNDLE_ISSUE_URL",
									Value: sb.Spec.IssueURL,
								},
								{
									Name:  "SUPPORT_BUNDLE_DEBUG",
									Value: "true",
								},
								{
									Name: "SUPPORT_BUNDLE_MANAGER_POD_IP",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "status.podIP",
										},
									},
								},
								{
									Name:  "SUPPORT_BUNDLE_IMAGE",
									Value: image,
								},
								{
									Name:  "SUPPORT_BUNDLE_IMAGE_PULL_POLICY",
									Value: string(pullPolicy),
								},
								{
									Name:  "SUPPORT_BUNDLE_NODE_SELECTOR",
									Value: "harvesterhci.io/managed=true",
								},
								{
									Name:  "SUPPORT_BUNDLE_EXCLUDE_RESOURCES",
									Value: m.getExcludeResources(),
								},
								{
									Name:  "SUPPORT_BUNDLE_EXTRA_COLLECTORS",
									Value: m.getExtraCollectors(),
								},
								{
									Name:  "SUPPORT_BUNDLE_TAINT_TOLERATION",
									Value: AdditionalTaintToleration,
								},
								{
									Name:  "SUPPORT_BUNDLE_NODE_TIMEOUT",
									Value: nodeTimeout.String(),
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

func (m *Manager) getCollectNamespaces() string {
	namespaces := []string{
		"cattle-dashboards",
		"cattle-fleet-local-system",
		"cattle-fleet-system",
		"cattle-fleet-clusters-system",
		"cattle-monitoring-system",
		"fleet-local",
		"harvester-system",
		"local",
		"longhorn-system",
		"cattle-logging-system",
		// namespace for CAPI system components
		// https://github.com/rancher/rancher/blob/4ac81b66b0f971548be78f2d1c72ecb906171a0b/pkg/controllers/dashboard/systemcharts/controller.go#L176
		"cattle-provisioning-capi-system",
	}

	extraNamespaces := settings.SupportBundleNamespaces.Get()
	if extraNamespaces != "" {
		namespaces = append(namespaces, extraNamespaces)
	}

	return strings.Join(namespaces, ",")
}

func (m *Manager) getExcludeResources() string {
	resources := []string{}

	// Sensitive data not go into support bundle
	resources = append(resources, harvesterv1.Resource(harvesterv1.SettingResourceName).String()) // TLS certificate and private key
	resources = append(resources, rancherv3.Resource(rancherv3.AuthConfigResourceName).String())
	resources = append(resources, rancherv3.Resource(rancherv3.AuthTokenResourceName).String())
	resources = append(resources, rancherv3.Resource(rancherv3.SamlTokenResourceName).String())
	resources = append(resources, rancherv3.Resource(rancherv3.TokenResourceName).String())
	resources = append(resources, rancherv3.Resource(rancherv3.UserResourceName).String())

	return strings.Join(resources, ",")
}

func (m *Manager) getExtraCollectors() string {
	extraCollectors := []string{"harvester"}
	return strings.Join(extraCollectors, ",")
}

func (m *Manager) GetStatus(sb *harvesterv1.SupportBundle) (*types.ManagerStatus, error) {
	podIP, err := GetManagerPodIP(m.podCache, sb)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("http://%s:8080/status", podIP)
	resp, err := m.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	s := &types.ManagerStatus{}
	err = json.NewDecoder(resp.Body).Decode(s)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func GetManagerPodIP(podCache ctlcorev1.PodCache, sb *harvesterv1.SupportBundle) (string, error) {
	sets := labels.Set{
		"app":                       types.AppManager,
		types.SupportBundleLabelKey: sb.Name,
	}

	pods, err := podCache.List(sb.Namespace, sets.AsSelector())
	if err != nil {
		return "", err

	}
	if len(pods) == 0 {
		return "", errors.New("manager pod is not created")
	}
	if len(pods) > 1 {
		return "", errors.New("more than one manager pods found")
	}
	if pods[0].Status.PodIP == "" {
		return "", errors.New("manager pod IP is not allocated")
	}
	return pods[0].Status.PodIP, nil
}
