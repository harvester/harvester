package nodedrain

import (
	"context"
	"fmt"
	"slices"
	"strings"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/rest"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/config"
	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/drainhelper"
	"github.com/harvester/harvester/pkg/util/virtualmachineinstance"
)

const (
	nodeDrainController  = "node-drain-controller"
	defaultWorkloadType  = "VirtualMachineInstance"
	defaultSingleCPCount = 1
	defaultHACPCount     = 3
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
	}

	nodes.OnChange(ctx, nodeDrainController, ndc.OnNodeChange)
	return nil
}

// OnNodeChange handles reconcile logic for node drains
func (ndc *ControllerHandler) OnNodeChange(_ string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return node, nil
	}
	_, ok := node.Annotations[drainhelper.DrainAnnotation]

	if !ok {
		return node, nil
	}

	_, forced := node.Annotations[drainhelper.ForcedDrain]

	if ok {
		// still running a check in the background to avoid maintenance issues when using object annotations
		// directly
		err := drainhelper.DrainPossible(ndc.nodeCache, node)
		if err != nil {
			return node, err
		}

		logrus.WithFields(logrus.Fields{
			"node_name": node.Name,
		}).Info("attempting to place node in maintenance mode")

		// List of VMs that need to be forcibly shutdown before maintenance
		// mode.
		shutdownVMs := make(map[string][]string)

		if !forced {
			var maintainModeStrategyVMs []string

			// Get the list of VMs that are labeled to forcibly shut down
			// before maintenance mode.
			maintainModeStrategyVMIs, err := ndc.listVMILabelMaintainModeStrategy(node)
			if err != nil {
				return node, fmt.Errorf("error in the listing of VMIs that are to be administratively stopped before migration: %w", err)
			}

			// Annotate these VMs so that they can be restarted immediately
			// when the node has finally switched into maintenance mode or
			// when the maintenance mode is disabled for the node.
			//
			// Note, forcing a shutdown of all VMs via the UI setting will
			// override the individual settings of VMs that are labelled
			// with 'harvesterhci.io/maintain-mode-strategy'. These VMs are
			// NOT restarted in this case.
			for _, vmi := range maintainModeStrategyVMIs {
				vmName, err := findVM(vmi)
				if err != nil {
					return node, err
				}

				// Append the VM to the list of VMs that need to be shut down.
				maintainModeStrategyVMs = append(maintainModeStrategyVMs, fmt.Sprintf("%s/%s", vmi.Namespace, vmName))

				// Skip and do not annotate VMs that do not have to be restarted
				// at several stages of the maintenance mode. These are VMs with
				// the label values:
				// - Shutdown
				if !slices.Contains([]string{
					util.MaintainModeStrategyShutdownAndRestartAfterEnable,
					util.MaintainModeStrategyShutdownAndRestartAfterDisable},
					vmi.Labels[util.LabelMaintainModeStrategy]) {
					continue
				}

				vm, err := ndc.virtualMachineCache.Get(vmi.Namespace, vmName)
				if err != nil {
					return node, fmt.Errorf("error looking up VM %s/%s: %w", vmi.Namespace, vmName, err)
				}

				vmCopy := vm.DeepCopy()
				vmCopy.Annotations[util.AnnotationMaintainModeStrategyNodeName] = node.Name
				_, err = ndc.virtualMachineClient.Update(vmCopy)
				if err != nil {
					return node, err
				}
			}

			shutdownVMs[util.MaintainModeStrategyKey] = maintainModeStrategyVMs
		}

		// Shutdown ALL VMs on that node forcibly? This is activated by a
		// checkbox in the maintenance mode dialog in the UI.
		if forced {
			shutdownVMs, err = ndc.FindNonMigratableVMS(node)
			if err != nil {
				return node, fmt.Errorf("error listing VMIs in scope for shutdown: %v", err)
			}
		}

		for _, v := range getUniqueVMSfromConditionMap(shutdownVMs) {
			// Fetch VMI again in case it has been modified.
			err := ndc.findAndStopVM(v)
			if err != nil {
				return node, err
			}

			ns, name := splitNamespacedName(v)
			logrus.WithFields(logrus.Fields{
				"node_name":           node.Name,
				"namespace":           ns,
				"virtualmachine_name": name,
			}).Info("force stopping VM")
		}

		// run node drain
		nodeCopy := node.DeepCopy()
		err = drainhelper.DrainNode(ndc.context, ndc.restConfig, nodeCopy)
		if err != nil {
			return node, err
		}

		nodeCopy.Annotations[ctlnode.MaintainStatusAnnotationKey] = ctlnode.MaintainStatusRunning
		delete(nodeCopy.Annotations, drainhelper.DrainAnnotation)
		delete(nodeCopy.Annotations, drainhelper.ForcedDrain)
		return ndc.nodes.Update(nodeCopy)
	}
	return node, nil
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
	vmObjCopy.Spec.RunStrategy = &[]kubevirtv1.VirtualMachineRunStrategy{desiredRunStrategy}[0]
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

	cdromOrContainerDiskVMs, err := findVMSwithCDROMOrContainerDisk(vmiList)
	if err != nil {
		return nil, fmt.Errorf("error finding VMs with CDROM or container disk: %v", err)
	}
	if len(cdromOrContainerDiskVMs) > 0 {
		result[util.ContainerDiskOrCDRomKey] = cdromOrContainerDiskVMs
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

// findVMSwithCDROMOrContainerDisk is called by action handler to leverage caches to find VM's which may have a cdrom or container disk
// attached to vmi
func findVMSwithCDROMOrContainerDisk(vmiList []*kubevirtv1.VirtualMachineInstance) ([]string, error) {
	var impactedVMI []string
	for _, vmi := range vmiList {
		if virtualmachineinstance.VMContainsCDRomOrContainerDisk(vmi) {
			impactedVMI = append(impactedVMI, namespacedVMName(vmi))
		}
	}
	return impactedVMI, nil
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
	if node.Spec.Unschedulable {
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
	var vmList []string
	for _, v := range vms {
		vmList = append(vmList, v...)
	}
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
	req, err := labels.NewRequirement(util.LabelMaintainModeStrategy, selection.In, []string{
		util.MaintainModeStrategyShutdownAndRestartAfterEnable,
		util.MaintainModeStrategyShutdownAndRestartAfterDisable,
		util.MaintainModeStrategyShutdown,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create selector to list VMIs that are to be administratively stopped before migration: %w", err)
	}
	return virtualmachineinstance.ListByNode(node, labels.NewSelector().Add(*req),
		ndc.virtualMachineInstanceCache)
}
