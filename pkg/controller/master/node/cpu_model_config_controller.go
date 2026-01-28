package node

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtcorev1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/yaml"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"

	"github.com/harvester/harvester/pkg/config"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	CPUModelConfigControllerName = "node-cpu-model-config-controller"
	configMapName                = "node-cpu-model-configuration"
	configMapDataKey             = "cpuModels"
)

var cpuModelLabelPrefixes = []string{
	kubevirtcorev1.CPUModelLabel,
}

type Checksum struct {
	Checksum  string `json:"checksum"`
	UpdatedAt string `json:"updatedAt"`
}

type CPUModelData struct {
	TotalNodes   int                             `json:"totalNodes"`
	GlobalModels []string                        `json:"globalModels"`
	Models       map[string]CPUModelCapabilities `json:"models"`
	Checksum
}

type CPUModelCapabilities struct {
	ReadyCount    int  `json:"readyCount"`
	MigrationSafe bool `json:"migrationSafe"`
}

type cpuModelConfigHandler struct {
	nodeCache       ctlcorev1.NodeCache
	kubevirtCache   ctlkubevirtv1.KubeVirtCache
	configMapClient ctlcorev1.ConfigMapClient
	configMapCache  ctlcorev1.ConfigMapCache
}

func CPUModelConfigRegister(ctx context.Context, management *config.Management, options config.Options) error {
	nodes := management.CoreFactory.Core().V1().Node()
	kubevirt := management.VirtFactory.Kubevirt().V1().KubeVirt()
	configMap := management.CoreFactory.Core().V1().ConfigMap()

	h := &cpuModelConfigHandler{
		nodeCache:       nodes.Cache(),
		kubevirtCache:   kubevirt.Cache(),
		configMapClient: configMap,
		configMapCache:  configMap.Cache(),
	}

	nodes.OnChange(ctx, CPUModelConfigControllerName, h.OnNodeChanged)
	nodes.OnRemove(ctx, CPUModelConfigControllerName, h.OnNodeChanged)
	kubevirt.OnChange(ctx, CPUModelConfigControllerName, h.OnKubeVirtChanged)

	// For ConfigMap cache
	if err := management.CoreFactory.Sync(ctx); err != nil {
		return fmt.Errorf("failed to sync core factory: %w", err)
	}

	if err := h.reconcile(); err != nil {
		return fmt.Errorf("failed to reconcile CPU model config at startup: %w", err)
	}

	return nil
}

func (h *cpuModelConfigHandler) OnNodeChanged(key string, node *corev1.Node) (*corev1.Node, error) {
	if err := h.reconcile(); err != nil {
		return node, err
	}
	return node, nil
}

func (h *cpuModelConfigHandler) OnKubeVirtChanged(key string, kv *kubevirtcorev1.KubeVirt) (*kubevirtcorev1.KubeVirt, error) {
	if kv == nil || kv.Namespace != util.HarvesterSystemNamespaceName || kv.Name != util.KubeVirtObjectName {
		return kv, nil
	}
	if err := h.reconcile(); err != nil {
		return kv, err
	}
	return kv, nil
}

func (h *cpuModelConfigHandler) reconcile() error {
	nodes, err := h.nodeCache.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	modelCounts := make(map[string]int)

	for _, node := range nodes {
		if !isNodeReady(node) {
			continue
		}

		collectNodeModels(node, modelCounts)
	}

	globalModels, err := h.collectGlobalModels()
	if err != nil {
		return err
	}

	models := make(map[string]CPUModelCapabilities, len(modelCounts))
	for model, count := range modelCounts {
		models[model] = CPUModelCapabilities{
			ReadyCount:    count,
			MigrationSafe: count > 1,
		}
	}

	data := CPUModelData{
		TotalNodes:   len(nodes),
		GlobalModels: globalModels,
		Models:       models,
	}

	return h.updateConfigMap(data)
}

func (h *cpuModelConfigHandler) updateConfigMap(data CPUModelData) error {
	configMap, err := h.configMapCache.Get(util.HarvesterSystemNamespaceName, configMapName)
	if err != nil {
		return fmt.Errorf("failed to get configmap: %w", err)
	}

	dataBytes, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal CPU model data: %w", err)
	}

	newChecksum := fmt.Sprintf("%x", sha256.Sum256(dataBytes))

	if existingDataStr, ok := configMap.Data[configMapDataKey]; ok {
		var existingChecksum Checksum
		if err := yaml.Unmarshal([]byte(existingDataStr), &existingChecksum); err == nil {
			if existingChecksum.Checksum == newChecksum {
				return nil
			}
		}
	}

	data.Checksum.Checksum = newChecksum
	data.Checksum.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	dataBytes, err = yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal CPU model data with checksum: %w", err)
	}

	toUpdate := configMap.DeepCopy()
	if toUpdate.Data == nil {
		toUpdate.Data = make(map[string]string)
	}
	toUpdate.Data[configMapDataKey] = string(dataBytes)

	_, err = h.configMapClient.Update(toUpdate)
	return err
}

func collectNodeModels(node *corev1.Node, modelCounts map[string]int) {
	for labelKey := range node.Labels {
		for _, prefix := range cpuModelLabelPrefixes {
			if !strings.HasPrefix(labelKey, prefix) {
				continue
			}
			model := strings.TrimPrefix(labelKey, prefix)
			if model == "" {
				continue
			}
			modelCounts[model]++
		}
	}
}

func (h *cpuModelConfigHandler) collectGlobalModels() ([]string, error) {
	models := []string{
		kubevirtcorev1.CPUModeHostModel,
		kubevirtcorev1.CPUModeHostPassthrough,
	}

	kubevirt, err := h.kubevirtCache.Get(util.HarvesterSystemNamespaceName, util.KubeVirtObjectName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get kubevirt configuration: %w", err)
		}
	}
	if kubevirt != nil && kubevirt.Spec.Configuration.CPUModel != "" {
		models = append(models, kubevirt.Spec.Configuration.CPUModel)
	}

	return models, nil
}

func isNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
