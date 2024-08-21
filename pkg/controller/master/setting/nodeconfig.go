package setting

import (
	"encoding/json"
	"fmt"
	"reflect"

	nodev1 "github.com/harvester/node-manager/pkg/apis/node.harvesterhci.io/v1beta1"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

func (h *Handler) syncNodeConfig(setting *harvesterv1.Setting) error {
	logrus.Infof("Processing setting %s, value: %s", setting.Name, setting.Value)

	ntpSettings := &util.NTPSettings{}
	if err := json.Unmarshal([]byte(setting.Value), ntpSettings); err != nil {
		return fmt.Errorf("failed to parse NTP settings: %v", err)
	}
	if ntpSettings.NTPServers == nil {
		return fmt.Errorf("NTP servers can't be empty")
	}

	nodes, err := h.nodeClient.List(metav1.ListOptions{})
	if err != nil {
		logrus.Errorf("List all node failure: %v", err)
		return err
	}

	parsedNTPServers := util.ReGenerateNTPServers(ntpSettings)
	for _, node := range nodes.Items {
		nodeName := node.Name

		nodeConfig, err := h.nodeConfigs.Get(util.HarvesterSystemNamespaceName, nodeName, metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				logrus.Errorf("Get node config with nodeName %s failure: %v", nodeName, err)
				return err
			}

			newNodeConfig := generateNodeConfig(nodeName, parsedNTPServers)
			if _, err := h.nodeConfigs.Create(newNodeConfig); err != nil {
				logrus.Errorf("Create node config with nodeName %s failure: %v", nodeName, err)
				return err
			}
		} else {
			logrus.Debugf("Nodeconfig content: %+v", nodeConfig)
			nodeConfigCpy := nodeConfig.DeepCopy()
			updateNodeConfig(nodeConfigCpy, parsedNTPServers)
			if !reflect.DeepEqual(nodeConfig, nodeConfigCpy) {
				if _, err := h.nodeConfigs.Update(nodeConfigCpy); err != nil {
					logrus.Errorf("Update node config with nodeName %s failure: %v", nodeName, err)
					return err
				}
			}
		}

	}

	return nil
}

func updateNodeConfig(nodeConfig *nodev1.NodeConfig, ntpServers string) {
	nodeConfig.Spec.NTPConfig.NTPServers = ntpServers
}

func generateNodeConfig(nodeName, ntpServers string) *nodev1.NodeConfig {
	ntpConfig := generateNTPConfig(ntpServers)
	return &nodev1.NodeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeName,
			Namespace: util.HarvesterSystemNamespaceName,
		},
		Spec: nodev1.NodeConfigSpec{
			NTPConfig: ntpConfig,
		},
	}
}

func generateNTPConfig(ntpServers string) *nodev1.NTPConfig {
	return &nodev1.NTPConfig{
		NTPServers: ntpServers,
	}
}
