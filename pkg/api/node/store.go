package node

import (
	"fmt"
	"strings"

	"github.com/rancher/apiserver/pkg/types"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtcorev1 "kubevirt.io/api/core/v1"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"

	"github.com/harvester/harvester/pkg/util"
)

const (
	queryParamLink                   = "link"
	linkTypeCPUMigrationCapabilities = "getCpuMigrationCapabilities"
	objectType                       = "node"
	objectID                         = "node"
)

var cpuModelLabelPrefixes = []string{
	kubevirtcorev1.CPUModelLabel,
}

type Store struct {
	types.Store
	nodeCache     ctlcorev1.NodeCache
	kubeVirtCache ctlkubevirtv1.KubeVirtCache
}

func (s *Store) List(req *types.APIRequest, schema *types.APISchema) (types.APIObjectList, error) {
	linkType := s.getLinkQueryType(req)
	if linkType == "" {
		return s.Store.List(req, schema)
	}

	switch linkType {
	case linkTypeCPUMigrationCapabilities:
		return s.handleCPUMigrationCapabilitiesRequest()
	default:
		return s.Store.List(req, schema)
	}
}

func (s *Store) getLinkQueryType(req *types.APIRequest) string {
	queryValues, exists := req.Query[queryParamLink]
	if !exists || len(queryValues) == 0 {
		return ""
	}
	return queryValues[0]
}

func (s *Store) handleCPUMigrationCapabilitiesRequest() (types.APIObjectList, error) {
	capabilities, err := s.buildCPUMigrationCapabilities()
	if err != nil {
		return types.APIObjectList{}, err
	}

	obj := types.APIObject{
		Type:   objectType,
		ID:     objectID,
		Object: capabilities,
	}

	return types.APIObjectList{
		Objects: []types.APIObject{obj},
		Count:   1,
	}, nil
}

func (s *Store) buildCPUMigrationCapabilities() (CPUMigrationCapabilities, error) {
	nodes, err := s.nodeCache.List(labels.Everything())
	if err != nil {
		return CPUMigrationCapabilities{}, fmt.Errorf("failed to list nodes: %w", err)
	}

	modelCounts := make(map[string]int)
	readyNodes := 0

	for _, node := range nodes {
		if !isNodeReady(node) {
			continue
		}

		readyNodes++
		nodeModels := collectNodeModels(node)
		for model := range nodeModels {
			modelCounts[model]++
		}
	}

	globalModels, err := s.collectGlobalModels()
	if err != nil {
		return CPUMigrationCapabilities{}, err
	}

	models := make(map[string]CPUModelCapabilities, len(modelCounts))
	for model, count := range modelCounts {
		models[model] = CPUModelCapabilities{
			ReadyCount:    count,
			MigrationSafe: count > 1,
		}
	}

	return CPUMigrationCapabilities{
		TotalNodes:   len(nodes),
		GlobalModels: globalModels,
		Models:       models,
	}, nil
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

func (s *Store) collectGlobalModels() ([]string, error) {
	models := []string{
		kubevirtcorev1.CPUModeHostModel,
		kubevirtcorev1.CPUModeHostPassthrough,
	}

	if s.kubeVirtCache != nil {
		kubevirt, err := s.kubeVirtCache.Get(util.HarvesterSystemNamespaceName, util.KubeVirtObjectName)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get kubevirt configuration: %w", err)
			}
		}
		if kubevirt != nil && kubevirt.Spec.Configuration.CPUModel != "" {
			models = append(models, kubevirt.Spec.Configuration.CPUModel)
		}
	}

	return models, nil
}

func isNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status != corev1.ConditionTrue {
			// skip unready nodes
			return false
		}
	}
	return true
}
