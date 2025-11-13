package upgrade

import (
	"fmt"

	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlcorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewMutator(
	node ctlcorev1.NodeCache,
	setting ctlharvesterv1.SettingCache,
) types.Mutator {
	return &upgradeMutator{
		node:    node,
		setting: setting,
	}
}

type upgradeMutator struct {
	types.DefaultMutator
	node    ctlcorev1.NodeCache
	setting ctlharvesterv1.SettingCache
}

func (m *upgradeMutator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"upgrades"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   harvesterv1.SchemeGroupVersion.Group,
		APIVersion: harvesterv1.SchemeGroupVersion.Version,
		ObjectType: &harvesterv1.Upgrade{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
		},
	}
}
func (m *upgradeMutator) Create(_ *types.Request, newObj runtime.Object) (types.PatchOps, error) {
	upgrade := newObj.(*harvesterv1.Upgrade)

	logrus.Debugf("create upgrade %s/%s", upgrade.Namespace, upgrade.Name)

	return m.patchPauseNodeAnnotations(upgrade, nil)
}

// patchPauseNodeAnnotations adds pause node annotations to upgrade based on upgrade-config setting.
// In manual mode, it pauses explicitly configured nodes or all nodes if none specified.
// Returns unmodified patchOps if auto mode is configured.
func (m *upgradeMutator) patchPauseNodeAnnotations(upgrade *harvesterv1.Upgrade, patchOps types.PatchOps) (types.PatchOps, error) {
	upgradeConfig, err := m.getUpgradeConfig()
	if err != nil {
		return nil, err
	}

	// Skip if auto mode or config not fully specified because by default it's auto mode
	if !shouldPauseNodes(upgradeConfig) {
		return patchOps, nil
	}

	nodeNames, err := m.getNodesToPause(upgradeConfig)
	if err != nil {
		return nil, err
	}

	return addPauseAnnotations(upgrade, patchOps, nodeNames), nil
}

func (m *upgradeMutator) getUpgradeConfig() (*settings.UpgradeConfig, error) {
	setting, err := m.setting.Get(settings.UpgradeConfigSettingName)
	if err != nil {
		return nil, err
	}

	value := setting.Value
	if value == "" {
		value = setting.Default
	}

	return settings.DecodeConfig[settings.UpgradeConfig](value)
}

func (m *upgradeMutator) getNodesToPause(config *settings.UpgradeConfig) ([]string, error) {
	if *config.NodeUpgradeOption.Strategy.Mode != settings.ManualType {
		return nil, nil
	}

	// Use explicitly configured pause nodes if provided
	if len(config.NodeUpgradeOption.Strategy.PauseNodes) > 0 {
		return config.NodeUpgradeOption.Strategy.PauseNodes, nil
	}

	// Otherwise, pause all nodes
	nodes, err := m.node.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	nodeNames := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}
	return nodeNames, nil
}

func shouldPauseNodes(config *settings.UpgradeConfig) bool {
	return config != nil &&
		config.NodeUpgradeOption != nil &&
		config.NodeUpgradeOption.Strategy != nil &&
		config.NodeUpgradeOption.Strategy.Mode != nil &&
		*config.NodeUpgradeOption.Strategy.Mode != settings.AutoType
}

func addPauseAnnotations(upgrade *harvesterv1.Upgrade, patchOps types.PatchOps, nodeNames []string) types.PatchOps {
	if upgrade.Annotations == nil {
		patchOps = append(patchOps, `{"op": "add", "path": "/metadata/annotations", "value": {}}`)
	}

	for _, nodeName := range nodeNames {
		patchOps = append(patchOps,
			fmt.Sprintf(`{"op": "add", "path": "/metadata/annotations/harvesterhci.io~1%s", "value": "pause"}`, nodeName))
	}

	return patchOps
}
