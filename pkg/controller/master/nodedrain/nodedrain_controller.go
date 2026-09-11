package nodedrain

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/rest"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/config"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/drainhelper"
	"github.com/harvester/harvester/pkg/util/virtualmachineinstance"
)

const (
	nodeDrainController = "node-drain-controller"
	defaultWorkloadType = "VirtualMachineInstance"
)

// ControllerHandler to drain nodes.
// the controller checks if node has any replicas which may be the only working replica for a VM and attempts to shutdown the VM
// before the drain is initiated. This ensures that no data is lost when the instance-managers are terminated as
// part of the drain process
type ControllerHandler struct {
	nodes                        ctlcorev1.NodeClient
	nodeCache                    ctlcorev1.NodeCache
	virtualMachineInstanceCache  ctlkubevirtv1.VirtualMachineInstanceCache
	virtualMachineInstanceClient ctlkubevirtv1.VirtualMachineInstanceClient
	virtualMachineClient         ctlkubevirtv1.VirtualMachineClient
	virtualMachineCache          ctlkubevirtv1.VirtualMachineCache
	longhornVolumeCache          ctllhv1.VolumeCache
	longhornReplicaCache         ctllhv1.ReplicaCache
	restConfig                   *rest.Config
	context                      context.Context
	drainNode                    func(context.Context, *rest.Config, *corev1.Node, time.Duration) error
	enqueueAfter                 func(string, time.Duration)
}

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	nodes := management.CoreFactory.Core().V1().Node()
	vmis := management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	lhv := management.LonghornFactory.Longhorn().V1beta2().Volume()
	lhr := management.LonghornFactory.Longhorn().V1beta2().Replica()
	ndc := &ControllerHandler{
		nodes:                        nodes,
		nodeCache:                    nodes.Cache(),
		virtualMachineInstanceCache:  vmis.Cache(),
		virtualMachineInstanceClient: vmis,
		virtualMachineClient:         vms,
		virtualMachineCache:          vms.Cache(),
		longhornReplicaCache:         lhr.Cache(),
		longhornVolumeCache:          lhv.Cache(),
		restConfig:                   management.RestConfig,
		context:                      ctx,
		drainNode:                    drainhelper.DrainNodeWithTimeout,
		enqueueAfter:                 nodes.EnqueueAfter,
	}

	nodes.OnChange(ctx, nodeDrainController, ndc.OnNodeChange)
	return nil
}

// OnNodeChange handles reconcile logic for node drains
func (ndc *ControllerHandler) OnNodeChange(_ string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return node, nil
	}

	condition := util.GetMaintenanceModeCondition(node)
	requested := drainhelper.HasDrainRequest(node)

	if condition == nil {
		if !requested {
			// Ignore ordinary node updates that are not maintenance requests.
			return node, nil
		}

		// Persist the initial phase before pre-checks or drain side effects so
		// every maintenance attempt has an observable, resumable state.
		return ndc.updateMaintenanceCondition(node.Name, corev1.ConditionTrue, util.NodeConditionReasonValidating,
			util.MaintenanceModeMessageValidating)
	}

	switch condition.Reason {
	case util.NodeConditionReasonValidating:
		if !requested {
			return node, nil
		}
		return ndc.validate(node)
	case util.NodeConditionReasonDraining:
		return ndc.drain(node, condition)
	case util.NodeConditionReasonError:
		if requested || drainhelper.IsForcedDrainRequested(node) {
			return ndc.cleanupErrorIntent(node)
		}
	}

	return node, nil
}

func (ndc *ControllerHandler) validate(node *corev1.Node) (*corev1.Node, error) {
	forced := drainhelper.IsForcedDrainRequested(node)

	logrus.WithFields(logrus.Fields{
		"node_name": node.Name,
		"forced":    forced,
	}).Info("Attempting to place node in maintenance mode")

	// still running a check in the background to avoid maintenance issues when using object annotations
	// directly
	err := drainhelper.DrainPossible(ndc.nodeCache, node)
	if err != nil {
		if errors.Is(err, drainhelper.ErrNodeDrainNotPossible) {
			message := fmt.Sprintf("enabling maintenance mode is impossible: %v", err)
			return ndc.failPreCheck(node.Name, message)
		}

		return node, err
	}

	nonMigratableVMs, err := ndc.FindNonMigratableVMS(node)
	if err != nil {
		return node, fmt.Errorf("error getting non-migratable VMs: %w", err)
	}

	if !forced && len(nonMigratableVMs) > 0 {
		reasons := make([]string, 0, len(nonMigratableVMs))

		for condition, vms := range nonMigratableVMs {
			reasons = append(reasons, fmt.Sprintf("%s cannot be migrated due to %s", strings.Join(vms, ","), condition))
		}

		message := fmt.Sprintf("enabling maintenance mode is impossible. Non-migratable VMs found: %s. Use 'force drain' to perform a collective shutdown", strings.Join(reasons, "; "))
		return ndc.failPreCheck(node.Name, message)
	}

	// Entering Draining persists the absolute deadline start time. The next
	// reconcile performs VM shutdown and the synchronous DrainNode call.
	return ndc.updateMaintenanceCondition(node.Name, corev1.ConditionTrue, util.NodeConditionReasonDraining, util.MaintenanceModeMessageDraining)
}

func (ndc *ControllerHandler) drain(node *corev1.Node, condition *corev1.NodeCondition) (*corev1.Node, error) {
	if ndc.drainNode == nil {
		ndc.drainNode = drainhelper.DrainNodeWithTimeout
	}

	timeoutMinutes := settings.MaintenanceModeDrainTimeout.GetInt()
	deadline := time.Time{}

	if timeoutMinutes > 0 {
		deadline = condition.LastTransitionTime.Add(time.Duration(timeoutMinutes) * time.Minute)
		if !time.Now().Before(deadline) {
			return ndc.failDrainTimeout(node, timeoutMinutes, context.DeadlineExceeded)
		}
	}

	forced := drainhelper.IsForcedDrainRequested(node)
	nonMigratableVMs, err := ndc.FindNonMigratableVMS(node)
	if err != nil {
		return node, fmt.Errorf("error getting non-migratable VMs: %w", err)
	}

	shutdownVMs := make(map[string][]string)

	// Get the list of VMs that are labeled to forcibly shut down
	// before maintenance mode.
	maintainModeStrategyVMIs, err := ndc.listVMILabelMaintainModeStrategy(node)
	if err != nil {
		return node, fmt.Errorf("error in the listing of VMIs that are to be administratively stopped before migration: %w", err)
	}

	// Annotate these VMs so that they can be restarted immediately
	// when the node has finally switched into maintenance mode or
	// when the maintenance mode is disabled for the node.
	maintainModeStrategyLabelsToSkip := []string{
		util.MaintainModeStrategyShutdownAndRestartAfterEnable,
		util.MaintainModeStrategyShutdownAndRestartAfterDisable,
	}
	for _, vmi := range maintainModeStrategyVMIs {
		vmName, err := findVM(vmi)
		if err != nil {
			return node, err
		}

		// Append the VM to the list of VMs that need to be shut down.
		shutdownVMs[util.MaintainModeStrategyKey] = append(shutdownVMs[util.MaintainModeStrategyKey], fmt.Sprintf("%s/%s", vmi.Namespace, vmName))

		// Skip and do not annotate VMs that do not have to be restarted
		// at several stages of the maintenance mode. These are VMs with
		// the label values:
		// - Shutdown
		if !slices.Contains(maintainModeStrategyLabelsToSkip, vmi.Labels[util.LabelMaintainModeStrategy]) {
			continue
		}

		vm, err := ndc.virtualMachineCache.Get(vmi.Namespace, vmName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return node, fmt.Errorf("error looking up VM %s/%s: %w", vmi.Namespace, vmName, err)
		}

		if vm.Annotations == nil || vm.Annotations[util.AnnotationMaintainModeStrategyNodeName] != node.Name {
			vmCopy := vm.DeepCopy()
			if vmCopy.Annotations == nil {
				vmCopy.Annotations = make(map[string]string)
			}
			vmCopy.Annotations[util.AnnotationMaintainModeStrategyNodeName] = node.Name

			_, err = ndc.virtualMachineClient.Update(vmCopy)
			if err != nil {
				return node, err
			}
		}
	}

	// If forced is requested, also include all non-migratable VMs for shutdown.
	if forced {
		for k, v := range nonMigratableVMs {
			shutdownVMs[k] = v
		}
	}

	for _, v := range getUniqueVMSfromConditionMap(shutdownVMs) {
		// Fetch VMI again in case it has been modified.
		err := ndc.findAndStopVM(v)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return node, err
		}

		ns, name := splitNamespacedName(v)
		logrus.WithFields(logrus.Fields{
			"node_name":           node.Name,
			"namespace":           ns,
			"virtualmachine_name": name,
		}).Info("force stopping VM")
	}

	nodeCopy := node.DeepCopy()

	drainContext := ndc.context
	cancel := func() {}
	drainTimeout := time.Duration(0)
	if timeoutMinutes > 0 {
		drainContext, cancel = context.WithDeadline(drainContext, deadline)
		drainTimeout = time.Duration(timeoutMinutes) * time.Minute
	}
	defer cancel()

	err = ndc.drainNode(drainContext, ndc.restConfig, nodeCopy, drainTimeout)

	if timeoutMinutes > 0 && !time.Now().Before(deadline) {
		if err == nil {
			err = context.DeadlineExceeded
		}
		return ndc.failDrainTimeout(node, timeoutMinutes, err)
	}
	if err != nil {
		if timeoutMinutes > 0 && errors.Is(drainContext.Err(), context.DeadlineExceeded) {
			return ndc.failDrainTimeout(node, timeoutMinutes, err)
		}
		return node, err
	}

	// Persist the controller handoff before removing intent so a restart cannot
	// leave a successfully drained node without a visible maintenance phase.
	updated, err := ndc.updateMaintenanceCondition(node.Name, corev1.ConditionTrue, util.NodeConditionReasonEvacuating,
		util.MaintenanceModeMessageEvacuating)
	if err != nil {
		return node, err
	}

	return ndc.cleanupDrainRequest(updated)
}

func (ndc *ControllerHandler) failDrainTimeout(node *corev1.Node, timeoutMinutes int, drainErr error) (*corev1.Node, error) {
	message := fmt.Sprintf("Maintenance mode timed out while draining node %s after %d minutes: %v. The node remains cordoned and drain-related taints are unchanged. Disable maintenance mode before starting a new attempt.", node.Name, timeoutMinutes, drainErr)

	updated, err := ndc.updateMaintenanceCondition(node.Name, corev1.ConditionTrue, util.NodeConditionReasonError, message)
	if err != nil {
		return node, err
	}
	// Surface the terminal error before removing intent; status and metadata
	// cannot be updated atomically.
	return ndc.cleanupErrorIntent(updated)
}

func (ndc *ControllerHandler) failPreCheck(nodeName, message string) (*corev1.Node, error) {
	updated, err := ndc.updateMaintenanceCondition(nodeName, corev1.ConditionFalse, util.NodeConditionReasonError, message)

	if err != nil {
		return nil, err
	}
	return ndc.cleanupErrorIntent(updated)
}

func (ndc *ControllerHandler) updateMaintenanceCondition(nodeName string, status corev1.ConditionStatus, reason, message string) (*corev1.Node, error) {
	return util.UpdateMaintenanceModeCondition(ndc.nodes, nodeName, status, reason, message)
}

// findAndStopVM is a wrapper function to identify the owner VM for a VMI, and patch the run strategy
func (ndc *ControllerHandler) findAndStopVM(vmiName string) error {
	ns, name := splitNamespacedName(vmiName)
	vmObj, err := ndc.virtualMachineCache.Get(ns, name)
	if err != nil {
		return fmt.Errorf("error fetching vm during findAndStopVM: %v", err)
	}

	desiredRunStrategy := kubevirtv1.RunStrategyHalted

	// Exit immediately if the "RunStrategy" is already in the desired state.
	runStrategy, err := vmObj.RunStrategy()
	if err != nil {
		return err
	}
	if runStrategy == desiredRunStrategy {
		return nil
	}

	vmObjCopy := vmObj.DeepCopy()
	vmObjCopy.Spec.RunStrategy = new(desiredRunStrategy)
	_, err = ndc.virtualMachineClient.Update(vmObjCopy)
	if err != nil {
		return fmt.Errorf("error updating run strategy for vm %s in namespace %s: %v", vmObj.Name, vmObj.Namespace, err)
	}

	return nil
}

// findVM is a wrapper function to identify VM from VMI owner references
func findVM(vmi *kubevirtv1.VirtualMachineInstance) (string, error) {
	refs := vmi.GetOwnerReferences()
	for _, owner := range refs {
		if owner.Kind == "VirtualMachine" && owner.APIVersion == "kubevirt.io/v1" {
			return owner.Name, nil
		}
	}
	return "", fmt.Errorf("no valid VM owner found for VMI %s", vmi.Name)
}

// list VMI will list VM's which may have their last healthy replica on current node. The VM itself may be
// scheduled on a different VM
func (ndc *ControllerHandler) listVMI(node *corev1.Node) ([]*kubevirtv1.VirtualMachineInstance, error) {
	var vmiList []*kubevirtv1.VirtualMachineInstance
	volList, err := ndc.listVolumeNames(node)
	if err != nil {
		return nil, fmt.Errorf("error in listVolumeNames: %v", err)
	}

	for _, v := range volList {
		for _, workloads := range v.Status.KubernetesStatus.WorkloadsStatus {
			if workloads.WorkloadType == defaultWorkloadType {
				vmiObj, err := ndc.virtualMachineInstanceCache.Get(v.Status.KubernetesStatus.Namespace, workloads.WorkloadName)
				if err != nil {
					return nil, err
				}
				vmiList = append(vmiList, vmiObj)
			}
		}
	}
	return vmiList, nil
}

// listVolumeNames will filter on all loghorn volumes, and identify volumes with only 1 working replica
// which is currently on the node in scope for drain.
func (ndc *ControllerHandler) listVolumeNames(node *corev1.Node) ([]*lhv1beta2.Volume, error) {
	type internalVolumeDetails struct {
		healthy   int
		unhealthy int
		replicas  []*lhv1beta2.Replica
	}
	volumeMap := map[string]internalVolumeDetails{}

	replicaList, err := ndc.longhornReplicaCache.List(util.LonghornSystemNamespaceName, labels.NewSelector())
	if err != nil {
		return nil, err
	}

	// identify PVC's with just one working replica
	for _, r := range replicaList {
		v, ok := volumeMap[r.Spec.VolumeName]
		if !ok {
			v = internalVolumeDetails{}
		}
		if r.Status.Started {
			v.healthy++
		} else {
			v.unhealthy++
		}
		v.replicas = append(v.replicas, r)
		volumeMap[r.Spec.VolumeName] = v
	}

	var possibleVolumeNames []string

	for pvcName, replicaState := range volumeMap {
		if replicaState.healthy <= 1 {
			for _, v := range replicaState.replicas {
				// last started replica is on the current node
				if v.Status.Started && v.Spec.NodeID == node.Name {
					possibleVolumeNames = append(possibleVolumeNames, pvcName)
				}
			}
		}
	}

	volList := make([]*lhv1beta2.Volume, 0, len(possibleVolumeNames))
	for _, v := range possibleVolumeNames {
		vObj, err := ndc.longhornVolumeCache.Get(util.LonghornSystemNamespaceName, v)
		if err != nil {
			return nil, err
		}
		volList = append(volList, vObj)
	}

	return volList, nil
}

// FindNonMigratableVMS is called by action handler to leverage caches to find unhealthy VM's impacted by the migration
func (ndc *ControllerHandler) FindNonMigratableVMS(node *corev1.Node) (map[string][]string, error) {
	result := make(map[string][]string)
	shutdownVMs, err := ndc.listVMI(node)
	if err != nil {
		return nil, fmt.Errorf("error listing VMI: %v", err)
	}
	impactedVMDetails := make([]string, 0, len(shutdownVMs))
	for _, v := range shutdownVMs {
		vmName, err := findVM(v)
		if err != nil {
			return nil, err
		}
		impactedVMDetails = append(impactedVMDetails, fmt.Sprintf("%s/%s", v.Namespace, vmName))
	}

	if len(impactedVMDetails) > 0 {
		result[util.LastHealthyReplicaKey] = impactedVMDetails
	}

	// list all VMI's currently scheduled on this node
	labelsMap := map[string]string{
		kubevirtv1.NodeNameLabel: node.Name,
	}
	labelSelector := labels.SelectorFromSet(labelsMap)

	vmiList, err := ndc.virtualMachineInstanceCache.List(corev1.NamespaceAll, labelSelector)
	if err != nil {
		return nil, fmt.Errorf("error listing VMI: %v", err)
	}

	for k, v := range IdentifyNonMigratableVMS(vmiList) {
		result[k] = v
	}

	unschedulableVMs, err := ndc.CheckVMISchedulingRequirements(node, vmiList)
	if err != nil {
		return nil, fmt.Errorf("error while checking vmi scheduling requirements: %v", err)
	}

	if len(unschedulableVMs) > 0 {
		result[util.NodeSchedulingRequirementsNotMetKey] = unschedulableVMs
	}

	return result, nil
}

func ActionHelper(nodeCache ctlcorev1.NodeCache, virtualMachineInstanceCache ctlkubevirtv1.VirtualMachineInstanceCache,
	longhornVolumeCache ctllhv1.VolumeCache, longhornReplicaCache ctllhv1.ReplicaCache) *ControllerHandler {
	return &ControllerHandler{
		nodeCache:                   nodeCache,
		virtualMachineInstanceCache: virtualMachineInstanceCache,
		longhornVolumeCache:         longhornVolumeCache,
		longhornReplicaCache:        longhornReplicaCache,
	}
}

// IdentifyNonMigratableVMS finds VMI's with kubevirtv1.VirtualMachineInstanceIsMigratable condition
// set to false
func IdentifyNonMigratableVMS(vmiList []*kubevirtv1.VirtualMachineInstance) map[string][]string {
	nonMigratableVM := make(map[string][]string)
	for _, vmi := range vmiList {
		for _, condition := range vmi.Status.Conditions {
			if condition.Type == kubevirtv1.VirtualMachineInstanceIsMigratable && condition.Status == corev1.ConditionFalse {
				result := nonMigratableVM[condition.Reason]
				result = append(result, namespacedVMName(vmi))
				nonMigratableVM[condition.Reason] = result
			}
		}
	}
	return nonMigratableVM
}

func namespacedVMName(vmi *kubevirtv1.VirtualMachineInstance) string {
	return fmt.Sprintf("%s/%s", vmi.Namespace, vmi.Name)
}

func splitNamespacedName(namespacedName string) (string, string) {
	vmDetails := strings.Split(namespacedName, "/")
	return vmDetails[0], vmDetails[1]
}

// CheckVMISchedulingRequirements checks if the VMI can be scheduled on another node
// the function will check additional nodes that
// * are able to satisfy the NodeSelectors terms specified in RequiredDuringSchedulingIgnoredDuringExecution
// * and node is ready
func (ndc *ControllerHandler) CheckVMISchedulingRequirements(originalNode *corev1.Node, vmiList []*kubevirtv1.VirtualMachineInstance) ([]string, error) {
	var impactedVMS []string
	nodeList, err := ndc.nodeCache.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("error listing nodes from nodeCache: %v", err)
	}
	var validNodes []*corev1.Node
	for _, v := range nodeList {
		if v.Name != originalNode.Name && isNodeReady(v) {
			validNodes = append(validNodes, v)
		}
	}
	for _, vmi := range vmiList {
		var possibleNodes, matchingNodes []*corev1.Node
		if vmi.Spec.Affinity != nil && vmi.Spec.Affinity.NodeAffinity != nil && vmi.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
			nodeAffinitySelector, err := nodeaffinity.NewNodeSelector(vmi.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution)
			if err != nil {
				return nil, fmt.Errorf("error generating nodeAffinitySelector from node scheduling requirements: %v", err)
			}
			// identify if nodeAffinity can be met by other nodes and node is ready
			for _, v := range validNodes {
				if nodeAffinitySelector.Match(v) {
					possibleNodes = append(possibleNodes, v)
				}
			}
		} else {
			possibleNodes = validNodes
		}
		// for VM's using masquerade network no additional network specific affinity rules are added
		// as a result this check is skipped
		matchingNodes = filterNodesForNodeSelector(possibleNodes, vmi)
		// no valid node found that could meet the requirements
		if len(matchingNodes) == 0 {
			impactedVMS = append(impactedVMS, namespacedVMName(vmi))
		}
	}
	return impactedVMS, nil
}

// filterNodesForNodeSelector will filter nodes for vmi node selector requirement match
func filterNodesForNodeSelector(possibleNodes []*corev1.Node, vmi *kubevirtv1.VirtualMachineInstance) []*corev1.Node {
	var validNodes []*corev1.Node
	// VM's may also have node selector, which is used when defining specific hostnames
	if len(vmi.Spec.NodeSelector) > 0 {
		vmiNodeSelector := labels.SelectorFromSet(vmi.Spec.NodeSelector)
		for _, v := range possibleNodes {
			nodeLabels := labels.Set(v.GetLabels())
			if vmiNodeSelector.Matches(nodeLabels) {
				validNodes = append(validNodes, v)
			}
		}
	} else {
		return possibleNodes
	}
	return validNodes
}

func isNodeReady(node *corev1.Node) bool {
	if node.Spec.Unschedulable || util.IsMaintenanceModeEngaged(node) {
		return false
	}

	for _, v := range node.Status.Conditions {
		if v.Type == corev1.NodeReady && v.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func getUniqueVMSfromConditionMap(vms map[string][]string) []string {
	//nolint:prealloc // if we want to preallocate we need to calculate the length first, so skip this linter
	var vmList []string
	for _, v := range vms {
		vmList = append(vmList, v...)
	}
	slices.Sort(vmList) // ensure Compact works correctly
	return slices.Compact(vmList)
}

// listVMILabelMaintainModeStrategy gets a list of VMs that are labeled
// with 'harvesterhci.io/maintain-mode-strategy' to forcibly shut down
// before maintenance mode.
// The label must have one of the following values:
// - ShutdownAndRestartAfterEnable
// - ShutdownAndRestartAfterDisable
// - Shutdown
func (ndc *ControllerHandler) listVMILabelMaintainModeStrategy(node *corev1.Node) ([]*kubevirtv1.VirtualMachineInstance, error) {
	req, err := labels.NewRequirement(util.LabelMaintainModeStrategy, selection.In, util.MaintainModeStrategyShutdownValues)
	if err != nil {
		return nil, fmt.Errorf("failed to create selector to list VMIs that are to be administratively stopped before migration: %w", err)
	}
	return virtualmachineinstance.ListByNode(node, labels.NewSelector().Add(*req),
		ndc.virtualMachineInstanceCache)
}

func (ndc *ControllerHandler) cleanupDrainRequest(node *corev1.Node) (*corev1.Node, error) {
	nodeCopy, err := ndc.nodes.Get(node.Name, metav1.GetOptions{})
	if err != nil {
		return node, err
	}
	nodeCopy = nodeCopy.DeepCopy()
	drainhelper.ClearDrainRequest(nodeCopy)
	nodeUpdate, err := ndc.nodes.Update(nodeCopy)
	if err != nil {
		return node, fmt.Errorf("failed to clean up drain request: %w", err)
	}
	return nodeUpdate, nil
}

func (ndc *ControllerHandler) cleanupErrorIntent(node *corev1.Node) (*corev1.Node, error) {
	liveNode, err := ndc.nodes.Get(node.Name, metav1.GetOptions{})
	if err != nil {
		return node, err
	}
	condition := util.GetMaintenanceModeCondition(liveNode)
	if !util.IsMaintenanceModeError(condition) {
		return liveNode, nil
	}
	liveNode = liveNode.DeepCopy()
	drainhelper.ClearDrainRequest(liveNode)
	updated, err := ndc.nodes.Update(liveNode)
	if err != nil {
		return node, fmt.Errorf("failed to clean up drain request: %w", err)
	}
	return updated, nil
}
