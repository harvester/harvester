package setting

import (
	"encoding/json"
	"fmt"
	"reflect"

	nodev1 "github.com/harvester/node-manager/pkg/apis/node.harvesterhci.io/v1beta1"
	longhorntypes "github.com/longhorn/longhorn-manager/types"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
	harvSettings "github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

// syncNodeConfig handles changes to "longhorn-v2-data-engine-enabled" and
// "ntp-servers" settings, as both of these need to be pushed out to all nodes
// via each node's nodeconfig CR.
func (h *Handler) syncNodeConfig(setting *harvesterv1.Setting) error {
	logrus.WithFields(logrus.Fields{
		"name":  setting.Name,
		"value": setting.Value,
	}).Info("Processing setting")

	nodes, err := h.nodeClient.List(metav1.ListOptions{})
	if err != nil {
		logrus.WithError(err).Error("Failed to list all nodes")
		return err
	}

	// Enqueue all the nodes so that nodeOnChanged can sync the node config.
	for _, node := range nodes.Items {
		h.nodeClient.Enqueue(node.Name)
	}

	// Enabling (or disabling) the Longhorn v2 data engine means we also need
	// to set lhs/v2-data-engine, so Loghorn can pick up the change.
	if setting.Name == harvSettings.LonghornV2DataEngineSettingName {
		enableV2DataEngine := setting.Value == "true"
		lhsV2DataEngine, err := h.longhornSettingCache.Get(util.LonghornSystemNamespaceName, string(longhorntypes.SettingNameV2DataEngine))
		if err != nil {
			return err
		}
		lhsV2DataEngineCpy := lhsV2DataEngine.DeepCopy()
		if enableV2DataEngine {
			lhsV2DataEngineCpy.Value = "true"
		} else {
			lhsV2DataEngineCpy.Value = "false"
		}
		if !reflect.DeepEqual(lhsV2DataEngine, lhsV2DataEngineCpy) {
			// This will not work immediately if there's not enough hugepages,
			// on each node, but because we return failure here when the
			// setting doesn't apply, the change is continually requeued, so it
			// will eventually go through once either the kubelets are restarted
			// and pick up the new number of hugepages, or the nodes are rebooted
			// and the allocation succeeds.
			if _, err := h.longhornSettings.Update(lhsV2DataEngineCpy); err != nil {
				return err
			}
		}
	}

	return nil
}

func (h *Handler) nodeOnChanged(_ string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return node, nil
	}

	// Get NTP Server settings
	ntpServersSetting, err := h.settingCache.Get(harvSettings.NTPServersSettingName)
	if err != nil {
		return nil, err
	}
	ntpSettings := &util.NTPSettings{}
	if ntpServersSetting.Value != "" {
		if err := json.Unmarshal([]byte(ntpServersSetting.Value), ntpSettings); err != nil {
			return nil, fmt.Errorf("failed to parse NTP settings: %v", err)
		}
	}
	ntpServers := util.ReGenerateNTPServers(ntpSettings)

	// Figure out if we should enable the LH V2 data engine
	enableV2DataEngineSetting, err := h.settingCache.Get(harvSettings.LonghornV2DataEngineSettingName)
	if err != nil {
		return nil, err
	}
	enableV2DataEngine := enableV2DataEngineSetting.Value == "true"
	// Don't enable for witness nodes
	if _, found := node.Labels[ctlnode.HarvesterWitnessNodeLabelKey]; found {
		enableV2DataEngine = false
	}
	// Don't enable if explicitly disabled for this node
	if value, found := node.Labels[longhorntypes.NodeDisableV2DataEngineLabelKey]; found && value == longhorntypes.NodeDisableV2DataEngineLabelKeyTrue {
		enableV2DataEngine = false
	}

	// Create new node config if it doesn't exist (think: new node added to cluster)
	nodeConfig, err := h.nodeConfigs.Get(util.HarvesterSystemNamespaceName, node.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			logrus.WithFields(logrus.Fields{
				"name":      node.Name,
				"namespace": util.HarvesterSystemNamespaceName,
			}).WithError(err).Error("Failed to get node config")
			return nil, err
		}
		_, err = h.nodeConfigs.Create(&nodev1.NodeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      node.Name,
				Namespace: util.HarvesterSystemNamespaceName,
			},
			Spec: nodev1.NodeConfigSpec{
				NTPConfig: &nodev1.NTPConfig{
					NTPServers: ntpServers,
				},
				LonghornConfig: &nodev1.LonghornConfig{
					EnableV2DataEngine: enableV2DataEngine,
				},
			},
		})
		if err != nil {
			return nil, err
		}
		logrus.WithFields(logrus.Fields{
			"name":      node.Name,
			"namespace": util.HarvesterSystemNamespaceName,
		}).Info("Created new node config")
		return node, nil
	}

	// If the node config does exist, update it if any of the settings have changed
	nodeConfigCpy := nodeConfig.DeepCopy()
	nodeConfigCpy.Spec.NTPConfig.NTPServers = ntpServers
	// Need to make sure Spec.LonghornConfig actually exists, because
	// it won't be present in node config objects from older clusters
	// that have been upgraded.
	if nodeConfigCpy.Spec.LonghornConfig == nil {
		nodeConfigCpy.Spec.LonghornConfig = &nodev1.LonghornConfig{}
	}
	nodeConfigCpy.Spec.LonghornConfig.EnableV2DataEngine = enableV2DataEngine
	if !reflect.DeepEqual(nodeConfig, nodeConfigCpy) {
		if _, err := h.nodeConfigs.Update(nodeConfigCpy); err != nil {
			logrus.WithFields(logrus.Fields{
				"name":      node.Name,
				"namespace": util.HarvesterSystemNamespaceName,
			}).WithError(err).Error("Failed to update node config")
			return nil, err
		}
	}
	return node, nil
}
