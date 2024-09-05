package node

import (
	"fmt"
	"strconv"
	"strings"

	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/virtualmachineinstance"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(nodeCache v1.NodeCache, jobCache ctlbatchv1.JobCache, vmiCache ctlkubevirtv1.VirtualMachineInstanceCache) types.Validator {
	return &nodeValidator{
		nodeCache: nodeCache,
		jobCache:  jobCache,
		vmiCache:  vmiCache,
	}
}

type nodeValidator struct {
	types.DefaultValidator
	nodeCache v1.NodeCache
	jobCache  ctlbatchv1.JobCache
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
	// check if there is any vm that enable cpu pinning while cpu manager is going to be disabled
	if err := checkCPUPinningVMIs(node, policy, v.vmiCache); err != nil {
		return err
	}

	return nil
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
	if policy != ctlnode.CPUManagerNonePolicy {
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
