package node

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"github.com/go-errors/errors"
	catalogv1 "github.com/rancher/rancher/pkg/generated/controllers/catalog.cattle.io/v1"
	"github.com/rancher/wrangler/v3/pkg/condition"
	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/name"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/catalog"
)

const (
	CPUManagerControllerName = "cpu-manager-controller"
	// policy
	CPUManagerStaticPolicy CPUManagerPolicy = "static"
	CPUManagerNonePolicy   CPUManagerPolicy = "none"
	// status
	CPUManagerRequestedStatus CPUManagerStatus = "requested"
	CPUManagerRunningStatus   CPUManagerStatus = "running"
	CPUManagerSuccessStatus   CPUManagerStatus = "success"
	CPUManagerFailedStatus    CPUManagerStatus = "failed"

	CPUManagerRootMountPath         string = "/host"
	CPUManagerScriptMountPath       string = "/harvester-helpers"
	CPUManagerScriptPath            string = CPUManagerScriptMountPath + "/cpu-manager.sh"
	CPUManagerWaitLabelTimeoutInSec int64  = 300 // 5 min
	CPUManagerJobTimeoutInSec       int64  = CPUManagerWaitLabelTimeoutInSec * 2
	CPUManagerJobTTLInSec           int32  = 86400 * 7 // 7 day
)

type CPUManagerPolicy string
type CPUManagerStatus string

type CPUManagerUpdateStatus struct {
	Policy  CPUManagerPolicy `json:"policy"`
	Status  CPUManagerStatus `json:"status"`
	JobName string           `json:"jobName,omitempty"`
}

// cpuManagerNodeHandler updates cpu manager status of a node in its annotations, so that
// we can tell whether the node is under modifing cpu manager policy or not and its current policy.
type cpuManagerNodeHandler struct {
	appCache   catalogv1.AppCache
	nodeCache  ctlcorev1.NodeCache
	nodeClient ctlcorev1.NodeClient
	jobClient  ctlbatchv1.JobClient
	namespace  string
}

// CPUManagerRegister registers the node controller
func CPUManagerRegister(ctx context.Context, management *config.Management, options config.Options) error {
	app := management.CatalogFactory.Catalog().V1().App()
	job := management.BatchFactory.Batch().V1().Job()
	node := management.CoreFactory.Core().V1().Node()

	cpuManagerNodeHandler := &cpuManagerNodeHandler{
		appCache:   app.Cache(),
		jobClient:  job,
		nodeCache:  node.Cache(),
		nodeClient: node,
		namespace:  options.Namespace,
	}

	node.OnChange(ctx, CPUManagerControllerName, cpuManagerNodeHandler.OnNodeChanged)
	job.OnChange(ctx, CPUManagerControllerName, cpuManagerNodeHandler.OnJobChanged)

	return nil
}

func (h *cpuManagerNodeHandler) OnNodeChanged(_ string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.Annotations == nil || node.DeletionTimestamp != nil {
		return node, nil
	}

	annot, found := node.Annotations[util.AnnotationCPUManagerUpdateStatus]
	if !found {
		return node, nil
	}

	// do nothing if the node is in witness role
	if _, found := node.Labels[HarvesterWitnessNodeLabelKey]; found {
		return node, nil
	}

	cpuManagerStatus, err := GetCPUManagerUpdateStatus(annot)
	if err != nil {
		logrus.WithField("node_name", node.Name).WithError(err).Error("Skip update cpu manager policy, failed to retrieve cpu-manager-update-status from annotation")
		return node, nil
	}
	// do nothing if status is not requested
	if cpuManagerStatus.Status != CPUManagerRequestedStatus {
		return node, nil
	}

	newNode := node.DeepCopy()
	job, err := h.submitJob(cpuManagerStatus, node)
	logrus.WithFields(logrus.Fields{
		"node_name": node.Name,
		"job_name":  job.Name,
		"policy":    cpuManagerStatus.Policy,
	}).Info("Submit cpu manager job")
	if err != nil {
		logrus.WithField("node_name", node.Name).WithError(err).Error("Submit cpu manager job failed")
		setUpdateStatus(newNode, toFailedStatus(cpuManagerStatus))
	} else {
		setUpdateStatus(newNode, toRunningStatus(cpuManagerStatus, job))
	}

	if reflect.DeepEqual(newNode, node) {
		return node, nil
	}
	return h.nodeClient.Update(newNode)
}

func (h *cpuManagerNodeHandler) OnJobChanged(_ string, job *batchv1.Job) (*batchv1.Job, error) {
	if job == nil || job.Labels == nil || job.DeletionTimestamp != nil {
		return job, nil
	}

	nodeName, ok := job.Labels[util.LabelCPUManagerUpdateNode]
	if !ok {
		return job, nil
	}

	node, err := h.nodeCache.Get(nodeName)
	if err != nil {
		logrus.WithField("node_name", nodeName).WithError(err).Error("failed to get node from cache")
		return job, err
	}

	if condition.Cond(batchv1.JobComplete).IsTrue(job) {
		return nil, h.updateCPUManagerStatus(node, job, CPUManagerSuccessStatus)
	}

	if condition.Cond(batchv1.JobFailed).IsTrue(job) {
		return nil, h.updateCPUManagerStatus(node, job, CPUManagerFailedStatus)
	}

	return job, nil
}

func (h *cpuManagerNodeHandler) updateCPUManagerStatus(node *corev1.Node, job *batchv1.Job, status CPUManagerStatus) error {
	updateStatus := &CPUManagerUpdateStatus{
		Status:  status,
		Policy:  CPUManagerPolicy(job.Labels[util.LabelCPUManagerUpdatePolicy]),
		JobName: job.Name,
	}

	logrus.WithFields(logrus.Fields{
		"node_name": node.Name,
		"job_name":  job.Name,
		"policy":    updateStatus.Policy,
	}).Infof("cpu manager job %s", status)

	if _, err := h.nodeClient.Update(setUpdateStatus(node, updateStatus)); err != nil {
		logrus.WithField("node_name", node.Name).WithError(err).Errorf("failed to update node cpu manager %s status", status)
		return err
	}

	return nil
}

func GetCPUManagerUpdateStatus(jsonString string) (*CPUManagerUpdateStatus, error) {
	cpuManagerStatus := &CPUManagerUpdateStatus{}

	if err := json.Unmarshal([]byte(jsonString), cpuManagerStatus); err != nil {
		return nil, err
	}
	if cpuManagerStatus.Status != CPUManagerRequestedStatus && cpuManagerStatus.Status != CPUManagerRunningStatus && cpuManagerStatus.Status != CPUManagerSuccessStatus && cpuManagerStatus.Status != CPUManagerFailedStatus {
		return nil, errors.New("invalid status")
	}
	if cpuManagerStatus.Policy != CPUManagerNonePolicy && cpuManagerStatus.Policy != CPUManagerStaticPolicy {
		return nil, errors.New("invalid policy")
	}
	return cpuManagerStatus, nil
}

func toFailedStatus(updateStatus *CPUManagerUpdateStatus) *CPUManagerUpdateStatus {
	newUpdateStatus := CPUManagerUpdateStatus(*updateStatus)
	newUpdateStatus.Status = CPUManagerFailedStatus
	return &newUpdateStatus
}

func toRunningStatus(updateStatus *CPUManagerUpdateStatus, job *batchv1.Job) *CPUManagerUpdateStatus {
	newUpdateStatus := CPUManagerUpdateStatus(*updateStatus)
	newUpdateStatus.Status = CPUManagerRunningStatus
	newUpdateStatus.JobName = job.Name
	return &newUpdateStatus
}

func setUpdateStatus(node *corev1.Node, updateStatus *CPUManagerUpdateStatus) *corev1.Node {
	jsonStr, _ := json.Marshal(updateStatus)
	if node.Annotations == nil {
		node.Annotations = map[string]string{}
	}
	node.Annotations[util.AnnotationCPUManagerUpdateStatus] = string(jsonStr)
	return node
}

func (h *cpuManagerNodeHandler) getJob(policy CPUManagerPolicy, node *corev1.Node, image string) *batchv1.Job {
	hostPathDirectory := corev1.HostPathDirectory
	labels := map[string]string{
		util.LabelCPUManagerUpdateNode:   node.Name,
		util.LabelCPUManagerUpdatePolicy: string(policy),
	}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name.SafeConcatName(node.Name, "update-cpu-manager-"),
			Namespace:    h.namespace,
			Labels:       labels,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: node.APIVersion,
					Kind:       node.Kind,
					Name:       node.Name,
					UID:        node.UID,
				},
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To(int32(0)), // do not retry
			TTLSecondsAfterFinished: ptr.To(CPUManagerJobTTLInSec),
			ActiveDeadlineSeconds:   ptr.To(CPUManagerJobTimeoutInSec),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					HostPID:       true,
					Containers: []corev1.Container{
						{
							Name:    "update-cpu-manager",
							Image:   image,
							Command: []string{"sh"},
							Args:    []string{"-e", CPUManagerScriptPath, node.Name, string(policy)},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "host-root", MountPath: CPUManagerRootMountPath},
								{Name: "helpers", MountPath: CPUManagerScriptMountPath},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "HOST_DIR",
									Value: CPUManagerRootMountPath,
								},
								{
									Name:  "WAIT_LABEL_TIMEOUT",
									Value: strconv.FormatInt(CPUManagerWaitLabelTimeoutInSec, 10),
								},
							},
						},
					},
					ServiceAccountName: "harvester",
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      corev1.LabelHostname,
										Operator: corev1.NodeSelectorOpIn,
										Values: []string{
											node.Name,
										},
									}},
								}},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "host-root",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/",
									Type: &hostPathDirectory,
								},
							},
						},
						{
							Name: "helpers",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: helperConfigMapName,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (h *cpuManagerNodeHandler) submitJob(updateStatus *CPUManagerUpdateStatus, node *corev1.Node) (*batchv1.Job, error) {
	image, err := catalog.FetchAppChartImage(h.appCache, h.namespace, releaseAppHarvesterName, []string{"generalJob", "image"})
	if err != nil {
		return nil, fmt.Errorf("failed to get harvester image (%s): %v", image.ImageName(), err)
	}

	job, err := h.jobClient.Create(h.getJob(updateStatus.Policy, node, image.ImageName()))
	if err != nil {
		return nil, err
	}

	return job, nil
}
