package node

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rancher/wrangler/pkg/condition"
	ctlbatchv1 "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/name"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"

	"github.com/harvester/harvester/pkg/config"
)

const (
	promoteControllerName = "promote-node-controller"

	KubeNodeRoleLabelPrefix      = "node-role.kubernetes.io/"
	KubeMasterNodeLabelKey       = KubeNodeRoleLabelPrefix + "master"
	KubeControlPlaneNodeLabelKey = KubeNodeRoleLabelPrefix + "control-plane"

	HarvesterLabelAnnotationPrefix      = "harvesterhci.io/"
	HarvesterManagedNodeLabelKey        = HarvesterLabelAnnotationPrefix + "managed"
	HarvesterPromoteNodeLabelKey        = HarvesterLabelAnnotationPrefix + "promote-node"
	HarvesterPromoteStatusAnnotationKey = HarvesterLabelAnnotationPrefix + "promote-status"

	PromoteStatusComplete = "complete"
	PromoteStatusRunning  = "running"
	PromoteStatusUnknown  = "unknown"
	PromoteStatusFailed   = "failed"

	defaultSpecManagementNumber = 3

	promoteImage         = "busybox:1.32.0"
	promoteRootMountPath = "/host"

	promoteScriptsMountPath = "/harvester-helpers"
	promoteScript           = "/harvester-helpers/promote.sh"
	helperConfigMapName     = "harvester-helpers"
)

var (
	promoteBackoffLimit = int32(2)

	ConditionJobComplete = condition.Cond(batchv1.JobComplete)
	ConditionJobFailed   = condition.Cond(batchv1.JobFailed)
)

// PromoteHandler
type PromoteHandler struct {
	nodes     ctlcorev1.NodeController
	nodeCache ctlcorev1.NodeCache
	jobs      ctlbatchv1.JobClient
	jobCache  ctlbatchv1.JobCache
	recorder  record.EventRecorder
	namespace string
}

// PromoteRegister registers the node controller
func PromoteRegister(ctx context.Context, management *config.Management, options config.Options) error {
	nodes := management.CoreFactory.Core().V1().Node()
	jobs := management.BatchFactory.Batch().V1().Job()

	promoteController := &PromoteHandler{
		nodes:     nodes,
		nodeCache: nodes.Cache(),
		jobs:      jobs,
		jobCache:  jobs.Cache(),
		recorder:  management.NewRecorder("harvester-"+promoteControllerName, "", ""),
		namespace: options.Namespace,
	}

	nodes.OnChange(ctx, promoteControllerName, promoteController.OnNodeChanged)
	jobs.OnChange(ctx, promoteControllerName, promoteController.OnJobChanged)
	jobs.OnRemove(ctx, promoteControllerName, promoteController.OnJobRemove)

	return nil
}

// OnNodeChanged automate the upgrade of node roles
// If the number of managements in the cluster is less than spec number,
// the harvester oldest node will be automatically promoted to be management.
func (h *PromoteHandler) OnNodeChanged(key string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return node, nil
	}

	nodeList, err := h.nodeCache.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	promoteNode := selectPromoteNode(nodeList)
	if promoteNode == nil {
		return node, nil
	}

	// wait until node metadata show up. Sometimes the metadata are empty
	// during the starting of nodes. If the metadata are empty, promotion
	// jobs creation call will fail.
	if promoteNode.Kind == "" || promoteNode.APIVersion == "" {
		h.nodes.EnqueueAfter(node.Name, time.Second*10)
		return node, nil
	}

	if _, err = h.promote(promoteNode); err != nil {
		return nil, err
	}

	return node, nil
}

// OnJobChanged
// If the node corresponding to the promote job has been removed, delete the job.
// If the promote job executes successfully, the node's promote status will be marked as complete and schedulable
// If the promote job fails, the node's promote status will be marked as failed.
func (h *PromoteHandler) OnJobChanged(key string, job *batchv1.Job) (*batchv1.Job, error) {
	if job == nil || job.DeletionTimestamp != nil {
		return job, nil
	}

	nodeName, ok := job.Labels[HarvesterPromoteNodeLabelKey]
	if !ok {
		return job, nil
	}

	node, err := h.nodeCache.Get(nodeName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return job, h.deleteJob(job, metav1.DeletePropagationBackground)
		}
		return job, err
	}

	if ConditionJobComplete.IsTrue(job) {
		return h.setPromoteResult(job, node, PromoteStatusComplete)
	}

	if ConditionJobFailed.IsTrue(job) {
		return h.setPromoteResult(job, node, PromoteStatusFailed)
	}

	return job, nil
}

// OnJobRemove
// If the running promote job is deleted, the node's promote status will be marked as unknown
func (h *PromoteHandler) OnJobRemove(key string, job *batchv1.Job) (*batchv1.Job, error) {
	if job == nil {
		return job, nil
	}

	nodeName, ok := job.Labels[HarvesterPromoteNodeLabelKey]
	if !ok {
		return job, nil
	}
	if ConditionJobFailed.IsTrue(job) || ConditionJobComplete.IsTrue(job) {
		return job, nil
	}

	node, err := h.nodeCache.Get(nodeName)
	switch {
	case apierrors.IsNotFound(err):
		return job, nil
	case err != nil:
		return job, err
	}

	if isPromoteStatusIn(node, PromoteStatusRunning) {
		return h.setPromoteResult(job, node, PromoteStatusUnknown)
	}

	return job, nil
}

func (h *PromoteHandler) promote(node *corev1.Node) (*corev1.Node, error) {
	// first, mark node into promote status
	startedNode, err := h.setPromoteStart(node)
	if err != nil {
		return nil, err
	}

	// then, create a promote job on the node
	if _, err := h.createPromoteJob(node); err != nil {
		return nil, err
	}

	return startedNode, nil
}

func (h *PromoteHandler) logPromoteEvent(node *corev1.Node, status string) {
	preStatus := node.Annotations[HarvesterPromoteStatusAnnotationKey]
	eventType := corev1.EventTypeNormal
	switch status {
	case PromoteStatusUnknown, PromoteStatusFailed:
		eventType = corev1.EventTypeWarning
	}
	nodeReference := &corev1.ObjectReference{
		Name: node.Name,
		UID:  types.UID(node.Name),
		Kind: "Node",
	}
	h.recorder.Event(nodeReference, eventType,
		fmt.Sprintf("NodePromote%s", strings.Title(status)),
		fmt.Sprintf("Node %s promote status change: %s => %s", node.Name, preStatus, status))
}

// setPromoteStart set node unschedulable and set promote status running.
func (h *PromoteHandler) setPromoteStart(node *corev1.Node) (*corev1.Node, error) {
	if node.Annotations[HarvesterPromoteStatusAnnotationKey] == PromoteStatusRunning {
		return node, nil
	}
	h.logPromoteEvent(node, PromoteStatusRunning)
	toUpdate := node.DeepCopy()
	toUpdate.Annotations[HarvesterPromoteStatusAnnotationKey] = PromoteStatusRunning
	toUpdate.Spec.Unschedulable = true
	return h.nodes.Update(toUpdate)
}

// setPromoteResult set node schedulable and update promote status if the promote is successful
func (h *PromoteHandler) setPromoteResult(job *batchv1.Job, node *corev1.Node, status string) (*batchv1.Job, error) {
	if node.Annotations[HarvesterPromoteStatusAnnotationKey] == status {
		return job, nil
	}
	h.logPromoteEvent(node, status)
	toUpdate := node.DeepCopy()
	toUpdate.Annotations[HarvesterPromoteStatusAnnotationKey] = status
	if status == PromoteStatusComplete {
		toUpdate.Spec.Unschedulable = false
	}
	_, err := h.nodes.Update(toUpdate)
	return job, err
}

// selectPromoteNode select the oldest ready worker node to promote
// If the cluster doesn't need to be promoted, return nil
func selectPromoteNode(nodeList []*corev1.Node) *corev1.Node {
	var (
		promoteNode                             *corev1.Node
		healthyHarvesterWorkers                 []*corev1.Node
		managementOrHealthyHarvesterWorkerZones = make(map[string]bool)
		managementZones                         = make(map[string]bool)
		managementNumber                        int
	)

	nodeNumber := len(nodeList)
	canBeManagementNodeCount := nodeNumber
	for _, node := range nodeList {
		isManagement := isManagementRole(node)

		if isManagement {
			managementNumber++
		}

		// return if there are already enough management nodes
		if managementNumber == defaultSpecManagementNumber {
			return nil
		}

		// return if the management node count is equal to the total amount of nodes (there are no more nodes left to promote)
		if managementNumber == nodeNumber {
			return nil
		}

		// worker promotion is complete but node is not yet labeled as a management node
		if !isManagement && isPromoteStatusIn(node, PromoteStatusComplete) {
			return nil
		}

		// wait until the node promotion is completed or the failed or unknown status is cleared
		if isPromoteStatusIn(node, PromoteStatusRunning, PromoteStatusFailed, PromoteStatusUnknown) {
			return nil
		}

		zone := node.Labels[corev1.LabelTopologyZone]
		if isManagement {
			if zone != "" {
				managementZones[zone] = true
				managementOrHealthyHarvesterWorkerZones[zone] = true
			}
		} else if isHealthyNode(node) && isHarvesterNode(node) {
			if zone != "" {
				managementOrHealthyHarvesterWorkerZones[zone] = true
			}
			healthyHarvesterWorkers = append(healthyHarvesterWorkers, node)
		} else {
			canBeManagementNodeCount--
		}

		// return if there are no enough nodes can be management node
		if canBeManagementNodeCount < defaultSpecManagementNumber {
			return nil
		}
	}

	// return if there are no enough zones
	hasZones := len(managementZones) > 0
	hasEnoughZones := len(managementOrHealthyHarvesterWorkerZones) >= defaultSpecManagementNumber
	if hasZones && !hasEnoughZones {
		return nil
	}

	promoteNode = nil
	for _, node := range healthyHarvesterWorkers {
		zone := node.Labels[corev1.LabelTopologyZone]
		hasNewZone := zone != "" && !managementZones[zone]
		if !hasZones || hasNewZone {
			if promoteNode == nil || node.CreationTimestamp.Before(&promoteNode.CreationTimestamp) {
				promoteNode = node
			}
		}
	}

	// promote the oldest node
	return promoteNode
}

// isHealthyNode determine whether it's an healthy node
func isHealthyNode(node *corev1.Node) bool {
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady && c.Status != corev1.ConditionTrue {
			// skip unready nodes
			return false
		}

		if c.Type != corev1.NodeReady && c.Status == corev1.ConditionTrue {
			// skip node with conditions like nodeMemoryPressure, nodeDiskPressure, nodePIDPressure
			// and nodeNetworkUnavailable equal to true
			return false
		}
	}
	return true
}

// isHarvesterNode determine whether it's an Harvester node based on the node's label
func isHarvesterNode(node *corev1.Node) bool {
	_, ok := node.Labels[HarvesterManagedNodeLabelKey]
	return ok
}

// isManagementRole determine whether it's an management node based on the node's label
func isManagementRole(node *corev1.Node) bool {
	if value, ok := node.Labels[KubeMasterNodeLabelKey]; ok {
		return value == "true"
	}

	// Related to https://github.com/kubernetes/kubernetes/pull/95382
	if value, ok := node.Labels[KubeControlPlaneNodeLabelKey]; ok {
		return value == "true"
	}

	return false
}

func isPromoteStatusIn(node *corev1.Node, statuses ...string) bool {
	status, ok := node.Annotations[HarvesterPromoteStatusAnnotationKey]
	if !ok {
		return false
	}

	for _, s := range statuses {
		if status == s {
			return true
		}
	}

	return false
}

func (h *PromoteHandler) createPromoteJob(node *corev1.Node) (*batchv1.Job, error) {
	job := buildPromoteJob(h.namespace, node)
	return h.jobs.Create(job)
}

func (h *PromoteHandler) deleteJob(job *batchv1.Job, deletionPropagation metav1.DeletionPropagation) error {
	return h.jobs.Delete(job.Namespace, job.Name, &metav1.DeleteOptions{PropagationPolicy: &deletionPropagation})
}

func buildPromoteJob(namespace string, node *corev1.Node) *batchv1.Job {
	nodeName := node.Name
	hostPathDirectory := corev1.HostPathDirectory
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildPromoteJobName(nodeName),
			Namespace: namespace,
			Labels: labels.Set{
				HarvesterPromoteNodeLabelKey: nodeName,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: node.APIVersion,
					Kind:       node.Kind,
					Name:       nodeName,
					UID:        node.UID,
				},
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &promoteBackoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels.Set{
						HarvesterPromoteNodeLabelKey: nodeName,
					},
				},
				Spec: corev1.PodSpec{
					HostIPC:     true,
					HostPID:     true,
					HostNetwork: true,
					DNSPolicy:   corev1.DNSClusterFirstWithHostNet,
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      corev1.LabelHostname,
										Operator: corev1.NodeSelectorOpIn,
										Values: []string{
											nodeName,
										},
									}},
								}},
							},
						},
						PodAntiAffinity: &corev1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      HarvesterPromoteNodeLabelKey,
												Operator: metav1.LabelSelectorOpIn,
												Values: []string{
													nodeName,
												},
											},
										},
									},
									TopologyKey: corev1.LabelHostname,
								},
							},
						},
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      corev1.TaintNodeUnschedulable,
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{{
						Name: `host-root`,
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/", Type: &hostPathDirectory,
							},
						},
					}, {
						Name: "helpers",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: helperConfigMapName,
								},
							},
						},
					}},
					ServiceAccountName: "harvester",
				},
			},
		},
	}
	podTemplate := &job.Spec.Template

	podTemplate.Spec.Containers = []corev1.Container{
		{
			Name:      "promote",
			Image:     promoteImage,
			Command:   []string{"sh"},
			Args:      []string{"-e", promoteScript},
			Resources: corev1.ResourceRequirements{},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "host-root", MountPath: promoteRootMountPath},
				{Name: "helpers", MountPath: promoteScriptsMountPath},
			},
			ImagePullPolicy: corev1.PullIfNotPresent,
			SecurityContext: &corev1.SecurityContext{
				Privileged: pointer.BoolPtr(true),
			},
			Env: []corev1.EnvVar{
				{
					Name:  "HARVESTER_PROMOTE_NODE_NAME",
					Value: node.Name,
				},
			},
		},
	}

	return job
}

func buildPromoteJobName(nodeName string) string {
	return name.SafeConcatName("harvester", "promote", nodeName)
}
