package manager

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"
)

func (m *VolumeManager) GetInstanceManager(name string) (*longhorn.InstanceManager, error) {
	return m.ds.GetInstanceManager(name)
}

func (m *VolumeManager) ListInstanceManagers() (map[string]*longhorn.InstanceManager, error) {
	return m.ds.ListInstanceManagers()
}

func (m *VolumeManager) GetNode(name string) (*longhorn.Node, error) {
	return m.ds.GetNode(name)
}

func (m *VolumeManager) GetDiskTags() ([]string, error) {
	foundTags := make(map[string]struct{})
	var tags []string

	nodeList, err := m.ListNodesSorted()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list nodes")
	}
	for _, node := range nodeList {
		for _, disk := range node.Spec.Disks {
			for _, tag := range disk.Tags {
				if _, ok := foundTags[tag]; !ok {
					foundTags[tag] = struct{}{}
					tags = append(tags, tag)
				}
			}
		}
	}
	return tags, nil
}

func (m *VolumeManager) GetNodeTags() ([]string, error) {
	foundTags := make(map[string]struct{})
	var tags []string

	nodeList, err := m.ListNodesSorted()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list nodes")
	}
	for _, node := range nodeList {
		for _, tag := range node.Spec.Tags {
			if _, ok := foundTags[tag]; !ok {
				foundTags[tag] = struct{}{}
				tags = append(tags, tag)
			}
		}
	}
	return tags, nil
}

func (m *VolumeManager) UpdateNode(n *longhorn.Node) (*longhorn.Node, error) {
	node, err := m.ds.UpdateNode(n)
	if err != nil {
		return nil, err
	}
	logrus.Infof("Updated node %v to %+v", node.Spec.Name, node.Spec)
	return node, nil
}

func (m *VolumeManager) ListNodes() (map[string]*longhorn.Node, error) {
	nodeList, err := m.ds.ListNodes()
	if err != nil {
		return nil, err
	}
	return nodeList, nil
}

func (m *VolumeManager) ListReadyNodesWithEngineImage(image string) (map[string]*longhorn.Node, error) {
	return m.ds.ListReadyNodesWithEngineImage(image)
}

func (m *VolumeManager) ListNodesSorted() ([]*longhorn.Node, error) {
	nodeMap, err := m.ListNodes()
	if err != nil {
		return []*longhorn.Node{}, err
	}

	nodes := make([]*longhorn.Node, len(nodeMap))
	nodeNames, err := util.SortKeys(nodeMap)
	if err != nil {
		return []*longhorn.Node{}, err
	}
	for i, nodeName := range nodeNames {
		nodes[i] = nodeMap[nodeName]
	}
	return nodes, nil
}

func (m *VolumeManager) DiskUpdate(name string, updateDisks map[string]longhorn.DiskSpec) (*longhorn.Node, error) {
	node, err := m.ds.GetNode(name)
	if err != nil {
		return nil, err
	}

	node.Spec.Disks = updateDisks

	node, err = m.ds.UpdateNode(node)
	if err != nil {
		return nil, err
	}
	logrus.Infof("Updated node disks of %v to %+v", name, node.Spec.Disks)
	return node, nil
}

func (m *VolumeManager) DeleteNode(name string) error {
	node, err := m.ds.GetNode(name)
	if err != nil {
		return err
	}
	// only remove node from longhorn without any volumes on it
	replicas, err := m.ds.ListReplicasByNodeRO(name)
	if err != nil {
		return err
	}
	engines, err := m.ds.ListEnginesByNodeRO(name)
	if err != nil {
		return err
	}
	condition := types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeReady)
	// Only could delete node from longhorn if kubernetes node missing or manager pod is missing
	if condition.Status == longhorn.ConditionStatusTrue ||
		(condition.Reason != longhorn.NodeConditionReasonKubernetesNodeGone &&
			condition.Reason != longhorn.NodeConditionReasonManagerPodMissing) ||
		node.Spec.AllowScheduling || len(replicas) > 0 || len(engines) > 0 {
		return fmt.Errorf("could not delete node %v with node ready condition is %v, reason is %v, node schedulable %v, and %v replica, %v engine running on it", name,
			condition.Status, condition.Reason, node.Spec.AllowScheduling, len(replicas), len(engines))
	}
	if err := m.ds.DeleteNode(name); err != nil {
		return err
	}
	logrus.Infof("Deleted node %v", name)
	return nil
}
