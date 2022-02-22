package upgrade

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiv1alpha4 "sigs.k8s.io/cluster-api/api/v1alpha4"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

// nodeHandler syncs node OS upgrade
// We label `harvesterhci.io/pendingOSImage` to new OS version before rebooting a node.
// If `harvesterhci.io/pendingOSImage` is equals to the value of `harvesterhci.io/pendingOSImage`, we know the
// the reboot is done.
type nodeHandler struct {
	namespace     string
	nodeClient    ctlcorev1.NodeClient
	nodeCache     ctlcorev1.NodeCache
	upgradeClient ctlharvesterv1.UpgradeClient
	upgradeCache  ctlharvesterv1.UpgradeCache
	secretClient  ctlcorev1.SecretClient
}

func (h *nodeHandler) OnChanged(key string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil || node.Annotations == nil {
		return node, nil
	}

	expectedVersion, ok := node.Annotations[harvesterNodePendingOSImage]
	if !ok {
		return node, nil
	}

	upgradeControllerLock.Lock()
	defer upgradeControllerLock.Unlock()

	upgrade, err := ensureSingleUpgrade(h.namespace, h.upgradeCache)
	if err != nil {
		return nil, err
	}

	if upgrade.Labels[upgradeStateLabel] != StateUpgradingNodes {
		return node, nil
	}

	if upgrade.Status.NodeStatuses == nil || upgrade.Status.NodeStatuses[node.Name].State == "" {
		return node, nil
	}
	nodeState := upgrade.Status.NodeStatuses[node.Name].State
	if nodeState != nodeStateWatingReboot {
		return node, nil
	}

	machineName, ok := node.Annotations[capiv1alpha4.MachineAnnotation]
	if !ok {
		return node, nil
	}

	// find machine plan secret
	secrets, err := h.secretClient.List(rancherPlanSecretNamespace, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", rancherPlanSecretMachineLabel, machineName),
		FieldSelector: fmt.Sprintf("type=%s", rancherPlanSecretType),
	})

	if err != nil {
		return node, err
	}

	if len(secrets.Items) != 1 {
		return node, fmt.Errorf("Found %d plan secret for machine %s", len(secrets.Items), machineName)
	}

	secret := secrets.Items[0]

	logrus.Debugf("Waiting for node %s's OS to be upgraded (Want: %s, Got: %s).", node.Name, expectedVersion, node.Status.NodeInfo.OSImage)
	if expectedVersion == node.Status.NodeInfo.OSImage {
		upgradeUpdate := upgrade.DeepCopy()
		setNodeUpgradeStatus(upgradeUpdate, node.Name, StateSucceeded, "", "")
		if _, err := h.upgradeClient.Update(upgradeUpdate); err != nil {
			return nil, err
		}

		if upgrade.Status.SingleNode == "" {
			logrus.Infof("Adding post-hook done annotation on %s/%s", secret.Namespace, secret.Name)
			secretUpdate := secret.DeepCopy()
			secretUpdate.Annotations[postDrainAnnotation] = secret.Annotations[rke2PostDrainAnnotation]
			if _, err := h.secretClient.Update(secretUpdate); err != nil {
				return nil, err
			}
		}

		err := h.retryUpdateNodeOnConflict(node.Name, func(n *corev1.Node) {
			if n.Annotations == nil {
				return
			}
			delete(n.Annotations, harvesterNodePendingOSImage)
		})
		if err != nil {
			return nil, err
		}
	}
	return node, nil
}

type NodeUpdateFunc func(node *corev1.Node)

func (h *nodeHandler) retryUpdateNodeOnConflict(nodeName string, updateFunc NodeUpdateFunc) error {
	for i := 1; i < 3; i++ {
		current, err := h.nodeCache.Get(nodeName)
		if err != nil {
			return err
		}
		if current.DeletionTimestamp != nil {
			return nil
		}

		toUpdate := current.DeepCopy()
		updateFunc(toUpdate)
		if reflect.DeepEqual(current, toUpdate) {
			return nil
		}
		_, err = h.nodeClient.Update(toUpdate)
		if err == nil || !apierrors.IsConflict(err) {
			return err
		}
		time.Sleep(2 * time.Second)
	}
	return errors.New("Fail to update node")
}
