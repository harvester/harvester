package node

import (
	"fmt"
	"strconv"
	"strings"

	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	schedulinghelper "k8s.io/component-helpers/scheduling/corev1/nodeaffinity"

	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
	indexeresutil "github.com/harvester/harvester/pkg/util/indexeres"
	"github.com/harvester/harvester/pkg/util/virtualmachineinstance"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(nodeCache v1.NodeCache, jobCache ctlbatchv1.JobCache, vmCache ctlkubevirtv1.VirtualMachineCache, vmiCache ctlkubevirtv1.VirtualMachineInstanceCache) types.Validator {
	return &nodeValidator{
		nodeCache: nodeCache,
		jobCache:  jobCache,
		vmCache:   vmCache,
		vmiCache:  vmiCache,
	}
}

type nodeValidator struct {
	types.DefaultValidator
	nodeCache v1.NodeCache
	jobCache  ctlbatchv1.JobCache
	vmCache   ctlkubevirtv1.VirtualMachineCache
	vmiCache  ctlkubevirtv1.VirtualMachineInstanceCache
}

func (v *nodeValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"nodes"},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   corev1.SchemeGroupVersion.Group,
		APIVersion: corev1.SchemeGroupVersion.Version,
		ObjectType: &corev1.Node{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Update,
		},
	}
}

func (v *nodeValidator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	oldNode := oldObj.(*corev1.Node)
	newNode := newObj.(*corev1.Node)

	nodeList, err := v.nodeCache.List(labels.Everything())
	if err != nil {
		return err
	}

	if err := validateCordonAndMaintenanceMode(oldNode, newNode, nodeList); err != nil {
		return err
	}
	if err := v.validateCPUManagerOperation(newNode); err != nil {
		return err
	}
	if err := validateWitnessRoleChange(oldNode, newNode); err != nil {
		return err
	}
	return nil
}

func validateCordonAndMaintenanceMode(oldNode, newNode *corev1.Node, nodeList []*corev1.Node) error {
	// if old node already have "maintain-status" annotation or Unscheduleable=true,
	// it has already been enabled, so we skip it
	if _, ok := oldNode.Annotations[ctlnode.MaintainStatusAnnotationKey]; ok || oldNode.Spec.Unschedulable {
		return nil
	}
	// if new node doesn't have "maintain-status" annotation and Unscheduleable=false, we skip it
	if _, ok := newNode.Annotations[ctlnode.MaintainStatusAnnotationKey]; !ok && !newNode.Spec.Unschedulable {
		return nil
	}

	for _, node := range nodeList {
		if node.Name == oldNode.Name {
			continue
		}

		// Return when we find another available node
		if _, ok := node.Annotations[ctlnode.MaintainStatusAnnotationKey]; !ok && !node.Spec.Unschedulable {
			return nil
		}
	}
	return werror.NewBadRequest("can't enable maintenance mode or cordon on the last available node")
}

func (v *nodeValidator) validateCPUManagerOperation(node *corev1.Node) error {
	annot, ok := node.Annotations[util.AnnotationCPUManagerUpdateStatus]
	if !ok {
		return nil
	}

	// check if the node is in witness role
	if _, found := node.Labels[ctlnode.HarvesterWitnessNodeLabelKey]; found {
		return werror.NewBadRequest("The witness node is unable to update the CPU manager policy.")
	}

	updateStatus, err := ctlnode.GetCPUManagerUpdateStatus(annot)
	if err != nil {
		return werror.NewBadRequest(fmt.Sprintf("Failed to retrieve cpu-manager-update-status from annotation: %v", err))
	}
	// only validate when update status is requested
	if updateStatus.Status != ctlnode.CPUManagerRequestedStatus {
		return nil
	}
	policy := updateStatus.Policy

	// check if cpu manager policy is the same
	if err := checkCPUManagerLabel(node, policy); err != nil {
		return err
	}
	// check if there is other job that still updating cpu manager policy to the same node
	if err := checkCurrentNodeCPUManagerJobs(node, v.jobCache); err != nil {
		return err
	}
	// check if this node is master and there are other master nodes undating cpu manager policy
	// since the policy update on master node need to restart rke2-server, we only allow one master node update policy
	if err := checkMasterNodeJobs(node, v.nodeCache, v.jobCache); err != nil {
		return err
	}
	// check if there is any running vmis that enable cpu pinning while cpu manager is going to be disabled
	if err := checkCPUPinningVMIs(node, policy, v.vmiCache); err != nil {
		return err
	}
	// check if there is at least one node has cpu manager enabled if there is any vm with cpu pinning
	if err := checkCPUManagerEnabledForPinnedVMs(node, policy, v.nodeCache, v.vmCache); err != nil {
		return err
	}

	return nil
}

// validateWitnessRoleChange validates if the user is trying to make any change to the Witness node. Specifically,
// they should not be able to manually remove the witness label and taint from a witness node
func validateWitnessRoleChange(oldNode, newNode *corev1.Node) error {
	var isWitnessNodeTaint, isWitnessNodeLabel bool
	isWitnessNodeTaint = isWitnessNode(oldNode.Spec.Taints)
	if _, ok := oldNode.ObjectMeta.Labels[util.HarvesterWitnessNodeLabelKey]; ok {
		isWitnessNodeLabel = true
	}

	if !isWitnessNodeTaint && !isWitnessNodeLabel {
		// oldNode is not a witness node; no need for further checks
		return nil
	}

	var witnessTaintPersist, witnessLabelPersist bool
	witnessTaintPersist = isWitnessNode(newNode.Spec.Taints)
	if _, ok := newNode.ObjectMeta.Labels[util.HarvesterWitnessNodeLabelKey]; ok {
		witnessLabelPersist = true
	}

	if !witnessTaintPersist || !witnessLabelPersist {
		return werror.NewBadRequest("Cannot remove witness node taint or label from the witness node")
	}

	return nil
}

func isWitnessNode(taints []corev1.Taint) bool {
	for _, taint := range taints {
		if taint.Key == "node-role.kubernetes.io/etcd" && taint.Value == "true" && taint.Effect == corev1.TaintEffectNoExecute {
			return true
		}
	}
	return false
}

func checkCPUManagerLabel(node *corev1.Node, policy ctlnode.CPUManagerPolicy) error {
	cpuManagerLabel, err := strconv.ParseBool(node.Labels[kubevirtv1.CPUManager])
	// means CPUManager feature gate not enabled
	if err != nil {
		return werror.NewBadRequest("label cpumanager not found")
	}
	// check if the cpu manager policy is the same
	if ((policy == ctlnode.CPUManagerStaticPolicy) && cpuManagerLabel) || ((policy == ctlnode.CPUManagerNonePolicy) && !cpuManagerLabel) {
		return werror.NewBadRequest(fmt.Sprintf("current cpu manager policy is already the same as requested value: %s", policy))
	}
	return nil
}

func checkCurrentNodeCPUManagerJobs(node *corev1.Node, jobCache ctlbatchv1.JobCache) error {
	jobNames, err := getCPUManagerRunningJobNamesOnNodes(jobCache, []string{node.Name})
	if err != nil {
		return werror.NewInternalError(err.Error())
	}
	if len(jobNames) > 0 {
		return werror.NewBadRequest(fmt.Sprintf("there is other job %s updating the cpu manager policy for this node %s", strings.Join(jobNames, ", "), node.Name))
	}
	return nil
}

func checkMasterNodeJobs(node *corev1.Node, nodeCache v1.NodeCache, jobCache ctlbatchv1.JobCache) error {
	// the node is worker, no need to do validation
	if !ctlnode.IsManagementRole(node) {
		return nil
	}

	nodes, err := nodeCache.List(labels.Everything())
	if err != nil {
		return werror.NewInternalError(err.Error())
	}

	// collect master node names except the node itself
	masterNodeNames := []string{}
	for _, n := range nodes {
		if n.Name != node.Name && ctlnode.IsManagementRole(n) {
			masterNodeNames = append(masterNodeNames, n.Name)
		}
	}

	if len(masterNodeNames) == 0 {
		return nil
	}

	jobNames, err := getCPUManagerRunningJobNamesOnNodes(jobCache, masterNodeNames)
	if err != nil {
		return werror.NewInternalError(err.Error())
	}
	if len(jobNames) > 0 {
		return werror.NewBadRequest(fmt.Sprintf("the node you are trying to update the cpu manager policy is a master node, and only one master node can be updated at a time, while job %s is updating the policy for other master nodes",
			strings.Join(jobNames, ", ")))
	}

	return nil
}

func checkCPUPinningVMIs(node *corev1.Node, policy ctlnode.CPUManagerPolicy, vmiCache ctlkubevirtv1.VirtualMachineInstanceCache) error {
	// Skip check if the policy is set to 'static'
	if policy == ctlnode.CPUManagerStaticPolicy {
		return nil
	}

	vmis, err := virtualmachineinstance.ListByNode(node, labels.NewSelector(), vmiCache)
	if err != nil {
		return werror.NewInternalError(err.Error())
	}

	for _, vmi := range vmis {
		if vmi.Spec.Domain.CPU != nil && vmi.Spec.Domain.CPU.DedicatedCPUPlacement {
			return werror.NewBadRequest("there should not be any running VMs with CPU pinning when disabling the CPU manager")
		}
	}
	return nil
}

func getCPUManagerRunningJobNamesOnNodes(jobCache ctlbatchv1.JobCache, nodeNames []string) ([]string, error) {
	jobs, err := ctlnode.GetCPUManagerRunningJobsOnNodes(jobCache, nodeNames)
	if err != nil {
		return []string{}, err
	}
	jobNames := make([]string, len(jobs))
	for i, job := range jobs {
		jobNames[i] = job.Name
	}
	return jobNames, nil
}

// checkCPUManagerEnabledForPinnedVMs ensures cluster safety when disabling CPU Manager by preventing
// scenarios that would leave CPU-pinned VMs without a suitable host node. This validation performs
// two critical checks:
//
//  1. VM Binding Check: Identifies VMs with CPU pinning that are bound to the current node via
//     NodeSelector constraints. These VMs cannot be rescheduled to other nodes, so disabling
//     CPU Manager would make them unschedulable.
//
//  2. Last Node Protection: Prevents disabling CPU Manager on the last enabled node when any
//     CPU-pinned VMs exist in the cluster, ensuring at least one node remains available for
//     scheduling these VMs.
func checkCPUManagerEnabledForPinnedVMs(
	node *corev1.Node,
	policy ctlnode.CPUManagerPolicy,
	nodeCache v1.NodeCache,
	vmCache ctlkubevirtv1.VirtualMachineCache,
) error {
	// Skip check if the policy is set to 'static'
	if policy == ctlnode.CPUManagerStaticPolicy {
		return nil
	}

	// Get all nodes with CPU Manager enabled
	selector := labels.Set{kubevirtv1.CPUManager: "true"}.AsSelector()
	nodesWithCPUManager, err := nodeCache.List(selector)
	if err != nil {
		return werror.NewInternalError(err.Error())
	}

	// Get all VMs with CPU pinning
	cpuPinnedVMs, err := vmCache.GetByIndex(indexeresutil.VMByCPUPinningIndex, indexeresutil.CPUPinningEnabled)
	if err != nil {
		return werror.NewInternalError(err.Error())
	}

	if len(cpuPinnedVMs) == 0 {
		return nil
	}

	// Check if VMs are bound to this specific node via NodeSelector
	vmsBoundToCurrentNode := getVMsBoundToNode(cpuPinnedVMs, node, nodesWithCPUManager)
	if len(vmsBoundToCurrentNode) > 0 {
		vmList := formatVMList(vmsBoundToCurrentNode)
		return werror.NewBadRequest(
			"Cannot disable CPU Manager on this node because the following VM(s) with CPU pinning " +
				"are bound to this node via Node Scheduling rules and cannot be scheduled on other CPU Manager enabled nodes. " +
				"Please remove CPU pinning or update Node Scheduling rules for these VM(s): " + vmList)
	}

	// Check if this is the last node with CPU Manager enabled
	if len(nodesWithCPUManager) == 1 && nodesWithCPUManager[0].UID == node.UID {
		vmList := formatVMList(cpuPinnedVMs)
		return werror.NewBadRequest(
			"Cannot disable CPU Manager on the last enabled node when VM(s) with CPU pinning exist. " +
				"Please remove CPU pinning from the following VM(s) to proceed: " + vmList)
	}

	return nil
}

func formatVMList(vms []*kubevirtv1.VirtualMachine) string {
	vmStrs := make([]string, len(vms))
	for i, vm := range vms {
		vmStrs[i] = fmt.Sprintf("%s/%s", vm.Namespace, vm.Name)
	}
	return strings.Join(vmStrs, ", ")
}

// getVMsBoundToNode returns VMs that are bound to a specific node via NodeSelector or NodeAffinity
// A VM is considered "bound" to a node if its scheduling constraints can only be satisfied by this node among CPU Manager enabled nodes.
func getVMsBoundToNode(vms []*kubevirtv1.VirtualMachine, targetNode *corev1.Node, cpuManagerNodes []*corev1.Node) []*kubevirtv1.VirtualMachine {
	boundVMs := make([]*kubevirtv1.VirtualMachine, 0)

	for _, vm := range vms {
		templateSpec := &vm.Spec.Template.Spec

		// Check if VM has any node constraints (NodeSelector or NodeAffinity)
		hasNodeSelector := len(templateSpec.NodeSelector) > 0
		hasNodeAffinity := templateSpec.Affinity != nil &&
			templateSpec.Affinity.NodeAffinity != nil &&
			templateSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil

		if !hasNodeSelector && !hasNodeAffinity {
			// No constraints, VM is not bound to any specific node
			continue
		}

		// Check if target node matches the VM's constraints
		if !vmMatchesNode(templateSpec, targetNode) {
			continue
		}

		// Count how many CPU Manager enabled nodes can satisfy this VM's constraints
		matchingNodeCount := 0
		for _, node := range cpuManagerNodes {
			if vmMatchesNode(templateSpec, node) {
				matchingNodeCount++
			}
		}

		// VM is bound to this node if it's the only CPU Manager enabled node that matches
		if matchingNodeCount == 1 {
			boundVMs = append(boundVMs, vm)
		}
	}

	return boundVMs
}

// vmMatchesNode checks if a VM's scheduling constraints (NodeSelector and NodeAffinity) match a node
func vmMatchesNode(vmiSpec *kubevirtv1.VirtualMachineInstanceSpec, node *corev1.Node) bool {
	// Check NodeSelector
	if len(vmiSpec.NodeSelector) > 0 {
		selector := labels.SelectorFromSet(vmiSpec.NodeSelector)
		if !selector.Matches(labels.Set(node.Labels)) {
			return false
		}
	}

	// Check NodeAffinity
	if vmiSpec.Affinity != nil &&
		vmiSpec.Affinity.NodeAffinity != nil &&
		vmiSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {

		nodeSelector := vmiSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
		lazySelector := schedulinghelper.NewLazyErrorNodeSelector(nodeSelector)

		matches, err := lazySelector.Match(node)
		if err != nil {
			logrus.WithField("node", node.Name).Warnf("Error matching node affinity: %v", err)
			return false
		}
		if !matches {
			return false
		}
	}

	return true
}
