package node

import (
	"context"

	ctlbatchv1 "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/name"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/pointer"

	"github.com/harvester/harvester/pkg/config"
	v1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

const (
	HarvesterDeleteNodeLabelKey = "harvesterhci.io/delete-node"
	deleteImage                 = "busybox:1.32.0"
	deleteNodeControllerName    = "delete-node-controller"
	deleteNodeFinalizer         = "harvesterhci.io/delete-node-controller"
	deleteRootMountPath         = "/host"
	rke2ConfigFilePath          = "/etc/rancher/rke2/config.yaml.d/50-rancher.yaml"
)

// deleteNodeHandler handles the finalization of `harvesterhci.io/delete-node-controller` finalizer.
// At this moment, it does the following tasks:
//
// - Add finalizer `harvesterhci.io/delete-node-controller` onto nodes.
// - Spawn job `harvester-delete-node-<node-name>` to remove RKE2 config file.
// - Delete the complete job `harvester-delete-node-<node-name>` and remove the finalizer.
type deleteNodeHandler struct {
	nodes                       ctlcorev1.NodeClient
	nodeCache                   ctlcorev1.NodeCache
	virtualMachineInstanceCache v1.VirtualMachineInstanceCache
	jobs                        ctlbatchv1.JobClient
	jobCache                    ctlbatchv1.JobCache
	namespace                   string
}

// DeleteRegister registers the node controller
func DeleteRegister(ctx context.Context, management *config.Management, options config.Options) error {
	nodes := management.CoreFactory.Core().V1().Node()
	jobs := management.BatchFactory.Batch().V1().Job()
	vmis := management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	deleteNodeHandler := &deleteNodeHandler{
		nodes:                       nodes,
		nodeCache:                   nodes.Cache(),
		jobs:                        jobs,
		jobCache:                    jobs.Cache(),
		virtualMachineInstanceCache: vmis.Cache(),
		namespace:                   options.Namespace,
	}

	nodes.OnChange(ctx, deleteNodeControllerName, deleteNodeHandler.OnNodeChanged)
	nodes.OnRemove(ctx, deleteNodeControllerName, deleteNodeHandler.OnNodeRemove)

	jobs.OnChange(ctx, deleteNodeControllerName, deleteNodeHandler.OnJobChanged)

	return nil
}

func (h *deleteNodeHandler) OnNodeChanged(key string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return node, nil
	}

	if !containsFinalizer(node, deleteNodeFinalizer) {
		toUpdate := node.DeepCopy()
		toUpdate.Finalizers = append(toUpdate.GetFinalizers(), deleteNodeFinalizer)
		return h.nodes.Update(toUpdate)
	}

	return node, nil
}

func (h *deleteNodeHandler) OnNodeRemove(key string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil {
		return node, nil
	}

	if node.DeletionTimestamp != nil {
		if _, err := h.createDeleteNodeJob(node); err != nil {
			return node, err
		}
	}

	return node, nil
}

func (h *deleteNodeHandler) OnJobChanged(key string, job *batchv1.Job) (*batchv1.Job, error) {
	if job == nil || job.DeletionTimestamp != nil {
		return job, nil
	}

	nodeName, ok := job.Labels[HarvesterDeleteNodeLabelKey]
	if !ok {
		return job, nil
	}

	if ConditionJobComplete.IsTrue(job) {
		node, err := h.nodeCache.Get(nodeName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				policy := metav1.DeletePropagationBackground
				return job, h.jobs.Delete(job.Namespace, job.Name, &metav1.DeleteOptions{PropagationPolicy: &policy})
			}
			return job, err
		}
		if _, err := h.removeFinalizer(node); err != nil {
			return job, err
		}
	}

	return job, nil
}

func (h *deleteNodeHandler) createDeleteNodeJob(node *corev1.Node) (*batchv1.Job, error) {
	job := createDeleteNodeJob(h.namespace, node)
	return h.jobs.Create(job)
}

func (h *deleteNodeHandler) removeFinalizer(node *corev1.Node) (*corev1.Node, error) {
	nodeCpy := node.DeepCopy()
	finalizers := []string{}
	for _, f := range nodeCpy.GetFinalizers() {
		if f != deleteNodeFinalizer {
			finalizers = append(finalizers, f)
		}
	}
	nodeCpy.SetFinalizers(finalizers)
	return h.nodes.Update(nodeCpy)
}

func containsFinalizer(node *corev1.Node, finalizer string) bool {
	for _, f := range node.GetFinalizers() {
		if f == finalizer {
			return true
		}
	}
	return false
}

func buildDeleteNodeJobName(nodeName string) string {
	return name.SafeConcatName("harvester", "delete-node", nodeName)
}

func createDeleteNodeJob(namespace string, node *corev1.Node) *batchv1.Job {
	nodeName := node.Name
	hostPathDirectory := corev1.HostPathDirectory
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildDeleteNodeJobName(nodeName),
			Namespace: namespace,
			Labels: labels.Set{
				HarvesterDeleteNodeLabelKey: nodeName,
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
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels.Set{
						HarvesterDeleteNodeLabelKey: nodeName,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:      "delete",
							Image:     deleteImage,
							Command:   []string{"rm"},
							Args:      []string{"-f", deleteRootMountPath + rke2ConfigFilePath},
							Resources: corev1.ResourceRequirements{},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "host-root", MountPath: deleteRootMountPath},
							},
							ImagePullPolicy: corev1.PullIfNotPresent,
							SecurityContext: &corev1.SecurityContext{
								Privileged: pointer.BoolPtr(true),
							},
						},
					},
					DNSPolicy: corev1.DNSClusterFirstWithHostNet,
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      corev1.LabelHostname,
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{nodeName},
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
												Key:      HarvesterDeleteNodeLabelKey,
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{nodeName},
											},
										},
									},
									TopologyKey: corev1.LabelHostname,
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{{
						Name: `host-root`,
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/",
								Type: &hostPathDirectory,
							},
						},
					}},
					ServiceAccountName: "harvester",
				},
			},
		},
	}

	return job
}
