package upgrade

import (
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"kubevirt.io/kubevirt/pkg/apimachinery/patch"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlcorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
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
// Returns unmodified patchOps if auto mode is configured or annotation is already explicitly specified.
func (m *upgradeMutator) patchPauseNodeAnnotations(upgrade *harvesterv1.Upgrade, patchOps types.PatchOps) (types.PatchOps, error) {
	if _, ok := upgrade.Annotations[util.AnnotationNodeUpgradePauseMap]; ok {
		return patchOps, nil
	}

	upgradeConfig, err := m.getUpgradeConfig()
	if err != nil {
		return nil, err
	}

	// Skip if auto mode or config not fully specified because by default it's auto mode
	if !isNodeUpgradeManualMode(upgradeConfig) {
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

	nodes, err := m.node.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	existingNodeNames := make([]string, 0, len(nodes))
	for _, node := range nodes {
		existingNodeNames = append(existingNodeNames, node.Name)
	}

	if len(config.NodeUpgradeOption.Strategy.PauseNodes) == 0 {
		return existingNodeNames, nil
	}

	// Filter out non-existent node names specified in the PauseNodes setting
	return sets.NewString(existingNodeNames...).Intersection(
		sets.NewString(config.NodeUpgradeOption.Strategy.PauseNodes...),
	).List(), nil
}

func isNodeUpgradeManualMode(config *settings.UpgradeConfig) bool {
	return config != nil &&
		config.NodeUpgradeOption != nil &&
		config.NodeUpgradeOption.Strategy != nil &&
		config.NodeUpgradeOption.Strategy.Mode != nil &&
		*config.NodeUpgradeOption.Strategy.Mode == settings.ManualType
}

func addPauseAnnotations(upgrade *harvesterv1.Upgrade, patchOps types.PatchOps, nodeNames []string) types.PatchOps {
	if len(nodeNames) == 0 {
		return nil
	}
	nodeMap := make(map[string]string, len(nodeNames))
	for _, nodeName := range nodeNames {
		nodeMap[nodeName] = util.NodePause
	}

	nodeMapJSON, err := json.Marshal(nodeMap)
	if err != nil {
		logrus.Errorf("failed to marshal node map for upgrade %s/%s: %v, the patch is skipped", upgrade.Namespace, upgrade.Name, err)
		return nil
	}

	if upgrade.Annotations == nil {
		patchOps = append(patchOps, `{"op": "add", "path": "/metadata/annotations", "value": {}}`)
	}

	patchOps = append(patchOps, fmt.Sprintf(`{"op": "add", "path": "/metadata/annotations/%s", "value": %q}`, patch.EscapeJSONPointer(util.AnnotationNodeUpgradePauseMap), string(nodeMapJSON)))

	return patchOps
}
