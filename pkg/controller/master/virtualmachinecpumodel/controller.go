package virtualmachinecpumodel

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtcorev1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
)

const (
	controllerName = "harvester-virtualmachinecpumodel-controller"
	singletonName  = "harvester"
)

var cpuModelLabelPrefixes = []string{
	kubevirtcorev1.CPUModelLabel,
}

type Handler struct {
	nodeCache            ctlcorev1.NodeCache
	kubevirtCache        ctlkubevirtv1.KubeVirtCache
	vmCpuModelClient     ctlharvesterv1.VirtualMachineCPUModelClient
	vmCpuModelController ctlharvesterv1.VirtualMachineCPUModelController
	vmCpuModelCache      ctlharvesterv1.VirtualMachineCPUModelCache
}

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	vmCpuModel := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineCPUModel()
	nodes := management.CoreFactory.Core().V1().Node()
	kubevirt := management.VirtFactory.Kubevirt().V1().KubeVirt()

	h := &Handler{
		nodeCache:            nodes.Cache(),
		kubevirtCache:        kubevirt.Cache(),
		vmCpuModelClient:     vmCpuModel,
		vmCpuModelController: vmCpuModel,
		vmCpuModelCache:      vmCpuModel.Cache(),
	}

	if err := h.createSingleton(); err != nil {
		return err
	}

	vmCpuModel.OnChange(ctx, controllerName, h.OnChanged)
	nodes.OnChange(ctx, controllerName, h.OnNodeChanged)
	nodes.OnRemove(ctx, controllerName, h.OnNodeChanged)
	kubevirt.OnChange(ctx, controllerName, h.OnKubeVirtChanged)

	return nil
}

func (h *Handler) OnChanged(key string, obj *harvesterv1.VirtualMachineCPUModel) (*harvesterv1.VirtualMachineCPUModel, error) {
	if obj == nil || obj.DeletionTimestamp != nil {
		return obj, nil
	}

	return h.reconcile(obj)
}

func (h *Handler) createSingleton() error {
	_, err := h.vmCpuModelClient.Get(singletonName, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	_, err = h.vmCpuModelClient.Create(&harvesterv1.VirtualMachineCPUModel{
		ObjectMeta: metav1.ObjectMeta{
			Name: singletonName,
		},
		Status: harvesterv1.VirtualMachineCPUModelStatus{
			TotalNodes:   0,
			GlobalModels: []string{},
			Models:       map[string]harvesterv1.CPUModelCapabilities{},
		},
	})

	return err
}

func (h *Handler) OnNodeChanged(key string, node *corev1.Node) (*corev1.Node, error) {
	h.enqueueSingleton()
	return node, nil
}

func (h *Handler) OnKubeVirtChanged(key string, kv *kubevirtcorev1.KubeVirt) (*kubevirtcorev1.KubeVirt, error) {
	if kv == nil || kv.Namespace != util.HarvesterSystemNamespaceName || kv.Name != util.KubeVirtObjectName {
		return kv, nil
	}
	h.enqueueSingleton()
	return kv, nil
}

func (h *Handler) enqueueSingleton() {
	h.vmCpuModelController.Enqueue(singletonName)
}

func (h *Handler) reconcile(obj *harvesterv1.VirtualMachineCPUModel) (*harvesterv1.VirtualMachineCPUModel, error) {
	nodes, err := h.nodeCache.List(labels.Everything())
	if err != nil {
		return obj, fmt.Errorf("failed to list nodes: %w", err)
	}

	modelCounts := make(map[string]int)

	for _, node := range nodes {
		if !isNodeReady(node) {
			continue
		}

		nodeModels := collectNodeModels(node)
		for model := range nodeModels {
			modelCounts[model]++
		}
	}

	globalModels, err := h.collectGlobalModels()
	if err != nil {
		return obj, err
	}

	models := make(map[string]harvesterv1.CPUModelCapabilities, len(modelCounts))
	for model, count := range modelCounts {
		models[model] = harvesterv1.CPUModelCapabilities{
			ReadyCount:    count,
			MigrationSafe: count > 1,
		}
	}

	status := harvesterv1.VirtualMachineCPUModelStatus{
		TotalNodes:   len(nodes),
		GlobalModels: globalModels,
		Models:       models,
	}

	if reflect.DeepEqual(obj.Status, status) {
		return obj, nil
	}

	toUpdate := obj.DeepCopy()
	toUpdate.Status = status
	return h.vmCpuModelClient.Update(toUpdate)
}

func collectNodeModels(node *corev1.Node) map[string]struct{} {
	models := make(map[string]struct{})
	for labelKey, labelValue := range node.Labels {
		if labelValue != "true" {
			continue
		}
		for _, prefix := range cpuModelLabelPrefixes {
			if !strings.HasPrefix(labelKey, prefix) {
				continue
			}
			model := strings.TrimPrefix(labelKey, prefix)
			if model == "" {
				continue
			}
			models[model] = struct{}{}
		}
	}
	return models
}

func (h *Handler) collectGlobalModels() ([]string, error) {
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
