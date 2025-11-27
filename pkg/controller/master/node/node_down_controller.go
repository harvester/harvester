package node

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"time"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	virtconfig "kubevirt.io/kubevirt/pkg/virt-config"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/controller/master/upgrade"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	nodeDownControllerName = "node-down-controller"
)

// kubevirtDrainTaint is used to trigger migration
var kubevirtDrainTaint = corev1.Taint{
	Key:    virtconfig.NodeDrainTaintDefaultKey,
	Effect: corev1.TaintEffectNoSchedule,
}

var nonGracefulTaint = corev1.Taint{
	Key:    corev1.TaintNodeOutOfService,
	Effect: corev1.TaintEffectNoExecute,
}

var (
	// vars for HarvesterNodeDrained condition
	HarvesterNodeCondDrained               = corev1.NodeConditionType("Drained")
	HarvesterNodeDrainedCondReasonDraining = "HarvesterNodeIsDraining"
	HarvesterNodeDrainedCondReasonDrained  = "HarvesterNodeIsDrained"
	HarvesterNodeDrainedCondMsg            = "Node is draining due to kubelet/node not ready."
)

// nodeDownHandler force deletes VMI's pod when a node is down, so VMI can be reschduled to anothor healthy node
type nodeDownHandler struct {
	nodes     ctlcorev1.NodeController
	nodeCache ctlcorev1.NodeCache

	upgrades ctlharvesterv1.UpgradeClient
}

// DownRegister registers a controller to delete VMI when node is down
func DownRegister(ctx context.Context, management *config.Management, _ config.Options) error {
	nodes := management.CoreFactory.Core().V1().Node()
	upgrades := management.HarvesterFactory.Harvesterhci().V1beta1().Upgrade()
	nodeDownHandler := &nodeDownHandler{
		nodes:     nodes,
		nodeCache: nodes.Cache(),
		upgrades:  upgrades,
	}

	nodes.OnChange(ctx, nodeDownControllerName, nodeDownHandler.OnNodeChanged)

	return nil
}

// OnNodeChanged monitors we focus the following 3 things
// 2. Kubelet not ready -> after vmForceResetPolicy.Period, add annotation to kickoff VM migration
// 3. Node not reachable -> add annotation to kickoff VM migration directly
func (h *nodeDownHandler) OnNodeChanged(_ string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return node, nil
	}

	// we skip DiskPressure check here because kubelet already have the basic audit.
	// if you want to do more thing when the node is under disk pressure, you can implement it.
	if err := h.checkNodeReady(node); err != nil {
		return node, fmt.Errorf("failed to check node ready for node %s: %w", node.Name, err)
	}

	return node, nil
}

func getNodeCondition(conditions []corev1.NodeCondition, conditionType corev1.NodeConditionType) *corev1.NodeCondition {
	var cond *corev1.NodeCondition
	for i := range conditions {
		c := conditions[i]
		if c.Type == conditionType {
			cond = &c
			break
		}
	}
	return cond
}

// We treat the node not ready (False/Unknown) as node down
// In this situation, we will wait for the extra timeout and then add taints to trigger migration and cleanup
func (h *nodeDownHandler) checkNodeReady(node *corev1.Node) error {
	// get Ready condition
	cond := getNodeCondition(node.Status.Conditions, corev1.NodeReady)
	if cond == nil {
		return fmt.Errorf("can't find %s condition in node %s", corev1.NodeReady, node.Name)
	}

	if isOnUpgrade(h.upgrades) {
		logrus.Debugf("Node %s is on upgrade, skipping checking node readiness", node.Name)
		// skip processing during upgrade
		return nil
	}

	switch cond.Status {
	case corev1.ConditionFalse:
	case corev1.ConditionUnknown:
		// wait for extra timeout to
		vmForceResetPolicy, err := h.fetchVMForceResetPolicy()
		if err != nil {
			return fmt.Errorf("failed to fetch VMForceResetPolicy setting: %w", err)
		}

		if !vmForceResetPolicy.Enable {
			// skip if the setting is disabled
			return nil
		}

		if getNodeTaint(node.Spec.Taints, virtconfig.NodeDrainTaintDefaultKey) != nil && getNodeTaint(node.Spec.Taints, corev1.TaintNodeOutOfService) != nil {
			// already added both taints, no need to process again
			return nil
		}

		// try to add Kubevirt drain taint after waiting extra timeout
		if waitEnough, err := h.addKubevirtTaintAfterExtraTimeout(node, cond, vmForceResetPolicy.Period); err != nil {
			return err
		} else if !waitEnough {
			return nil
		}

		node, err = h.createOrUpdateNodeCondition(node, HarvesterNodeCondDrained, corev1.ConditionFalse, HarvesterNodeDrainedCondReasonDraining, HarvesterNodeDrainedCondMsg)
		if err != nil {
			return fmt.Errorf("failed to create or update HarvesterNodeDrained condition (draining) for node %s: %w", node.Name, err)
		}
		cond = getNodeCondition(node.Status.Conditions, HarvesterNodeCondDrained)
		// before adding out-of-service taint, wait for VM migration timeout
		if waitEnough, err := h.addOutOfServiceTaintAfterMigrationTimeout(node, cond, vmForceResetPolicy.VMMigrationTimeout); err != nil {
			return err
		} else if !waitEnough {
			return nil
		}

		// set HarvesterNodeDrained condition to True -> which means the node is drained
		node, err = h.createOrUpdateNodeCondition(node, HarvesterNodeCondDrained, corev1.ConditionTrue, HarvesterNodeDrainedCondReasonDrained, HarvesterNodeDrainedCondMsg)
		if err != nil {
			return fmt.Errorf("failed to create or update HarvesterNodeDrained condition (drained) for node %s: %w", node.Name, err)
		}

		return nil
	case corev1.ConditionTrue:
		// reset taint if node is healthy again
		if err := h.removeTaints(node, virtconfig.NodeDrainTaintDefaultKey, corev1.TaintNodeOutOfService); err != nil {
			return fmt.Errorf("failed to remove taints %v from node %s: %w", []string{virtconfig.NodeDrainTaintDefaultKey, corev1.TaintNodeOutOfService}, node.Name, err)
		}

		// reset HarvesterNodeFailure condition if needed
		cond := getNodeCondition(node.Status.Conditions, HarvesterNodeCondDrained)
		if cond != nil {
			// remove the condition if exists
			logrus.Infof("Removing HarvesterNodeDrained condition from node %s", node.Name)
			if err := h.removeHarvesterNodeDrainedCond(node); err != nil {
				return fmt.Errorf("failed to reset HarvesterNodeFailure condition for node %s: %w", node.Name, err)
			}
		}

		return nil
	default:
		return fmt.Errorf("unknown status %s for condition %s in node %s", cond.Status, corev1.NodeReady, node.Name)
	}
	return nil
}

func (h *nodeDownHandler) addKubevirtTaintAfterExtraTimeout(node *corev1.Node, cond *corev1.NodeCondition, extraTime int64) (bool, error) {
	if getNodeTaint(node.Spec.Taints, virtconfig.NodeDrainTaintDefaultKey) != nil {
		// already added taint
		return true, nil
	}
	waitEnough, err := h.tryWaitForCondTimeout(cond, extraTime)
	if err != nil {
		// error case, dont' care about waitEnough
		return false, err
	}
	if !waitEnough {
		return false, nil
	}

	// we wait enough or node is unreachable, add taints to trigger migration
	if err := h.triggerMigration(node); err != nil {
		return true, fmt.Errorf("failed to trigger migration for node %s: %w", node.Name, err)
	}
	logrus.Infof("triggered migration for node %s", node.Name)
	return true, nil
}

func (h *nodeDownHandler) addOutOfServiceTaintAfterMigrationTimeout(node *corev1.Node, cond *corev1.NodeCondition, vmMigrationTimeout int64) (bool, error) {
	if getNodeTaint(node.Spec.Taints, corev1.TaintNodeOutOfService) != nil {
		// already added taint
		return true, nil
	}
	waitEnough, err := h.tryWaitForCondTimeout(cond, vmMigrationTimeout)
	if err != nil {
		// error case, dont' care about waitEnough
		return false, err
	}
	if !waitEnough {
		return false, nil
	}

	// we wait enough, add out-of-service taint to force delete orphan resources
	if err := h.cleanupResources(node); err != nil {
		return true, fmt.Errorf("failed to cleanup stuck resources for node %s: %w", node.Name, err)
	}
	logrus.Infof("cleaned up stuck resources for node %s", node.Name)
	return true, nil
}

func (h *nodeDownHandler) removeHarvesterNodeDrainedCond(node *corev1.Node) error {
	nodeCpy := node.DeepCopy()
	conds := nodeCpy.Status.Conditions
	newConds := slices.DeleteFunc(conds, func(c corev1.NodeCondition) bool {
		return c.Type == HarvesterNodeCondDrained
	})
	nodeCpy.Status.Conditions = newConds
	if _, err := h.nodes.UpdateStatus(nodeCpy); err != nil {
		return err
	}
	return nil
}

func (h *nodeDownHandler) createOrUpdateNodeCondition(node *corev1.Node, condType corev1.NodeConditionType, status corev1.ConditionStatus, reason string, message string) (*corev1.Node, error) {
	logrus.Debugf("createOrUpdateNodeCondition: %v, %v, %v, %v", condType, status, reason, message)
	cond := getNodeCondition(node.Status.Conditions, condType)
	now := metav1.Now()
	notFound := false
	if cond == nil {
		cond = generateNewNodeCondition(condType, status, reason, message, now, now)
		notFound = true
	}
	if cond.Status != status {
		cond.Status = status
		cond.LastTransitionTime = now
	}
	cond.LastHeartbeatTime = now
	cond.Reason = reason
	cond.Message = message

	nodeCpy := node.DeepCopy()
	if notFound {
		nodeCpy.Status.Conditions = append(nodeCpy.Status.Conditions, *cond)
	} else {
		for id, c := range nodeCpy.Status.Conditions {
			if c.Type == condType {
				nodeCpy.Status.Conditions[id] = *cond
			}
		}
	}
	logrus.Debugf("toUpdated status: %v", nodeCpy.Status.Conditions)
	return h.nodes.UpdateStatus(nodeCpy)
}

func generateNewNodeCondition(condType corev1.NodeConditionType, status corev1.ConditionStatus, reason string, message string, lastHeartbeatTime metav1.Time, lastTransitionTime metav1.Time) *corev1.NodeCondition {
	return &corev1.NodeCondition{
		Type:               condType,
		Status:             status,
		LastHeartbeatTime:  lastHeartbeatTime,
		LastTransitionTime: lastTransitionTime,
		Reason:             reason,
		Message:            message,
	}
}

func (h *nodeDownHandler) fetchVMForceResetPolicy() (*settings.VMForceResetPolicy, error) {
	vmForceResetPolicy, err := settings.DecodeVMForceResetPolicy(settings.VMForceResetPolicySet.Get())
	if err != nil {
		return nil, fmt.Errorf("failed to decode VMForceResetPolicy setting: %w", err)
	}
	return vmForceResetPolicy, nil
}

// tryWaitForTimeout assume the condition is not nil, if the condition is nil, return error
func (h *nodeDownHandler) tryWaitForCondTimeout(cond *corev1.NodeCondition, timeout int64) (bool, error) {
	targetInterval := time.Duration(timeout) * time.Second
	if cond == nil {
		return false, fmt.Errorf("node condition is nil, cannot wait for timeout")
	}

	if time.Since(cond.LastTransitionTime.Time) < targetInterval {
		deadline := cond.LastTransitionTime.Add(targetInterval)
		logrus.Debugf("Enqueue because the corresponding cond (%v) was not timeout at %v", cond.Type, deadline)
		h.nodes.EnqueueAfter(cond.Reason, time.Until(deadline))
		return false, nil
	}

	return true, nil
}

// we will add kubevirtDrainTaint first to trigger migration then add nonGracefulTaint to force delete orphan resources
func (h *nodeDownHandler) triggerMigration(node *corev1.Node) error {
	if err := h.addTaints(node, kubevirtDrainTaint); err != nil {
		return fmt.Errorf("failed to add kubevirt drain taint to node %s: %w", node.Name, err)
	}
	return nil
}

func (h *nodeDownHandler) cleanupResources(node *corev1.Node) error {
	if err := h.addTaints(node, nonGracefulTaint); err != nil {
		return fmt.Errorf("failed to add non-graceful taint to node %s: %w", node.Name, err)
	}
	return nil
}

func (h *nodeDownHandler) addTaints(node *corev1.Node, taints ...corev1.Taint) error {
	nodeCpy := node.DeepCopy()
	nodeCpy.Spec.Taints = append(nodeCpy.Spec.Taints, taints...)
	var err error
	if !reflect.DeepEqual(node.Spec.Taints, nodeCpy.Spec.Taints) {
		_, err = h.nodes.Update(nodeCpy)
	}
	return err
}

func (h *nodeDownHandler) removeTaints(node *corev1.Node, taintKeys ...string) error {
	nodeCpy := node.DeepCopy()
	taints := nodeCpy.Spec.Taints
	newTaints := slices.DeleteFunc(taints, func(t corev1.Taint) bool {
		for _, remove := range taintKeys {
			if t.Key == remove {
				return true
			}
		}
		return false
	})
	nodeCpy.Spec.Taints = newTaints
	var err error
	if !reflect.DeepEqual(node.Spec.Taints, nodeCpy.Spec.Taints) {
		logrus.Infof("Removing taints %v from node %s", taintKeys, node.Name)
		_, err = h.nodes.Update(nodeCpy)
	}
	return err
}

func getNodeTaint(taints []corev1.Taint, taintKey string) *corev1.Taint {
	var taint *corev1.Taint
	for i := range taints {
		t := taints[i]
		if t.Key == taintKey {
			taint = &t
			break
		}
	}
	return taint
}

func isOnUpgrade(upgrades ctlharvesterv1.UpgradeClient) bool {
	req, err := labels.NewRequirement(util.LabelHarvesterUpgradeState, selection.NotIn, []string{upgrade.StateSucceeded, upgrade.StateFailed})
	if err != nil {
		logrus.Warnf("Failed to create label requirement for %s: %v", util.LabelHarvesterUpgradeState, err)
		return false
	}

	upgradesItems, err := upgrades.List(util.HarvesterSystemNamespaceName, metav1.ListOptions{
		LabelSelector: labels.NewSelector().Add(*req).String(),
	})
	if err != nil {
		logrus.Warnf("Failed to list upgrades with label %s: %v", util.LabelHarvesterUpgradeState, err)
		return false
	}
	if len(upgradesItems.Items) > 0 {
		logrus.Debugf("There are ongoing upgrades: %v.", upgradesItems.Items[0].Name)
		return true
	}
	return false
}
