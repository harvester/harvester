package readyz

import (
	"fmt"
	"os"
	"strings"
	"time"

	"sync"

	"github.com/harvester/go-common/common"
	"github.com/harvester/harvester/pkg/config"
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
	"github.com/harvester/harvester/pkg/util"
	longhornTypes "github.com/longhorn/longhorn-manager/types"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/rancher/pkg/auth/tokens/hashers"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

const (
	oemConfigPath = "/oem/90_custom.yaml"
)

var (
	tokenOnce    sync.Once
	tokenHash    string
	tokenInitErr error
)

type ReadyzHandler struct {
	scaled *config.Scaled
}

type OEMConfig struct {
	Stages struct {
		Initramfs []struct {
			Files []struct {
				Path    string `yaml:"path"`
				Content string `yaml:"content"`
			} `yaml:"files"`
		} `yaml:"initramfs"`
	} `yaml:"stages"`
}

type RancherdConfig struct {
	Token string `yaml:"token"`
}

func NewReadyzHandler(scaled *config.Scaled) *ReadyzHandler {
	return &ReadyzHandler{
		scaled: scaled,
	}
}

func (h *ReadyzHandler) Do(ctx *harvesterServer.Ctx) (harvesterServer.ResponseBody, error) {
	req := ctx.Req()
	authHeader := req.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, apierror.NewAPIError(validation.Unauthorized, "Bearer token required")
	}

	providedToken := strings.TrimPrefix(authHeader, "Bearer ")
	if !h.validateToken(providedToken) {
		return nil, apierror.NewAPIError(validation.PermissionDenied, "Invalid token")
	}

	ready, msg := h.clusterReady()
	timestamp := time.Now().UTC().Format(time.RFC3339)

	if !ready {
		// Log detailed reason for debugging
		logrus.Infof("Cluster not ready: %s", msg)
		ctx.SetStatusOK()
		return map[string]any{
			"ready":     false,
			"msg":       msg,
			"timestamp": timestamp,
		}, nil
	}

	ctx.SetStatusOK()
	return map[string]any{
		"ready":     true,
		"timestamp": timestamp,
	}, nil
}

func (h *ReadyzHandler) validateToken(providedToken string) bool {
	tokenOnce.Do(func() {
		expectedToken, err := h.getTokenFromOEM()
		if err != nil {
			tokenInitErr = err
			logrus.Errorf("Failed to read token from OEM config: %v", err)
			return
		}
		hasher := hashers.GetHasher()
		tokenHash, tokenInitErr = hasher.CreateHash(expectedToken)
	})

	if tokenInitErr != nil {
		return false
	}
	hasher, err := hashers.GetHasherForHash(tokenHash)
	if err != nil {
		logrus.Errorf("Failed to get hasher: %v", err)
		return false
	}
	return hasher.VerifyHash(tokenHash, providedToken) == nil
}

func (h *ReadyzHandler) getTokenFromOEM() (string, error) {
	data, err := os.ReadFile(oemConfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to read OEM config: %w", err)
	}

	var config OEMConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("failed to parse OEM config: %w", err)
	}

	content := findFileContent(config, "/etc/rancher/rancherd/config.yaml")
	if content == "" {
		return "", fmt.Errorf("token not found in OEM config: file path missing")
	}

	var rancherdConfig RancherdConfig
	if err := yaml.Unmarshal([]byte(content), &rancherdConfig); err != nil {
		return "", fmt.Errorf("failed to parse rancherd config: %w", err)
	}

	if rancherdConfig.Token == "" {
		return "", fmt.Errorf("token is empty in rancherd config")
	}

	return rancherdConfig.Token, nil
}

func findFileContent(config OEMConfig, targetPath string) string {
	for _, stage := range config.Stages.Initramfs {
		for _, file := range stage.Files {
			if file.Path == targetPath {
				return file.Content
			}
		}
	}
	return ""
}

func (h *ReadyzHandler) clusterReady() (bool, string) {
	rkeControlPlane, err := h.scaled.Management.RKEFactory.Rke().V1().RKEControlPlane().Cache().Get(
		util.FleetLocalNamespaceName,
		util.LocalClusterName)
	if err != nil {
		return false, "rkeControlPlane not found"
	}

	ready := false
	for _, cond := range rkeControlPlane.Status.Conditions {
		if cond.Type == "Ready" && cond.Status == corev1.ConditionTrue {
			ready = true
			break
		}
	}
	if !ready {
		return false, "rkeControlPlane is not ready"
	}

	podCache := h.scaled.CoreFactory.Core().V1().Pod().Cache()

	longhornManagerSelector := labels.SelectorFromSet(labels.Set(longhornTypes.GetManagerLabels()))
	longhornPods, err := podCache.List(common.LonghornSystemNamespaceName, longhornManagerSelector)
	if err != nil {
		return false, "failed to check longhorn-manager pods"
	}

	if !hasAtLeastOneReadyPod(longhornPods) {
		return false, "longhorn-manager pods not ready"
	}

	virtControllerSelector := labels.SelectorFromSet(labels.Set{kubevirtv1.AppLabel: "virt-controller"})
	virtPods, err := podCache.List(common.HarvesterSystemNamespaceName, virtControllerSelector)
	if err != nil {
		return false, "failed to check virt-controller pods"
	}

	if !hasAtLeastOneReadyPod(virtPods) {
		return false, "virt-controller pods not ready"
	}

	return true, ""
}

// hasAtLeastOneReadyPod checks if at least one pod in the list is running and ready
func hasAtLeastOneReadyPod(pods []*corev1.Pod) bool {
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodRunning && isPodReadyConditionTrue(pod) {
			return true
		}
	}
	return false
}

func isPodReadyConditionTrue(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
