package node

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-errors/errors"
	"github.com/rancher/wrangler/v3/pkg/condition"
	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/name"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/util"
	utilHelm "github.com/harvester/harvester/pkg/util/helm"
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

	CPUManagerJobPrefix string = "update-cpu-manager"
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
	nodeCache  ctlcorev1.NodeCache
	nodeClient ctlcorev1.NodeClient
	jobCache   ctlbatchv1.JobCache
	jobClient  ctlbatchv1.JobClient
	clientset  *kubernetes.Clientset
	namespace  string
}

// CPUManagerRegister registers the node controller
func CPUManagerRegister(ctx context.Context, management *config.Management, options config.Options) error {
	job := management.BatchFactory.Batch().V1().Job()
	node := management.CoreFactory.Core().V1().Node()
	pod := management.CoreFactory.Core().V1().Pod()

	cpuManagerNodeHandler := &cpuManagerNodeHandler{
		jobCache:   job.Cache(),
		jobClient:  job,
		nodeCache:  node.Cache(),
		nodeClient: node,
		clientset:  management.ClientSet,
		namespace:  options.Namespace,
	}

	node.OnChange(ctx, CPUManagerControllerName, cpuManagerNodeHandler.OnNodeChanged)
	job.OnChange(ctx, CPUManagerControllerName, cpuManagerNodeHandler.OnJobChanged)
	pod.OnChange(ctx, CPUManagerControllerName, cpuManagerNodeHandler.OnPodChanged)

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
	runningJobs, err := getCPUManagerRunningJobOnNode(h.jobCache, node.Name)
	if err != nil {
		logrus.WithField("node_name", node.Name).WithError(err).Error("Failed to get cpu manager running job on node")
		return node, err
	}
	numOfRunningJobs := len(runningJobs)
	if numOfRunningJobs > 0 {
		// This scenario should be prevented by the existing validator, which ensures no CPU manager jobs
		// run concurrently on the same node. However, if it does occur, log the event for further investigation.
		if numOfRunningJobs > 1 {
			jobNames := make([]string, len(runningJobs))
			for i, job := range runningJobs {
				jobNames[i] = job.Name
			}
			logrus.WithFields(logrus.Fields{
				"node_name":    node.Name,
				"running_jobs": strings.Join(jobNames, ", "),
			}).Warnf("Multiple CPU manager jobs are running simultaneously on the same node")
		}
		setUpdateStatus(newNode, toRunningStatus(cpuManagerStatus, runningJobs[0]))
	} else {
		job, err := h.submitJob(cpuManagerStatus, node)
		if err != nil {
			logrus.WithField("node_name", node.Name).WithError(err).Error("Failed to submit cpu manager job")
			setUpdateStatus(newNode, toFailedStatus(cpuManagerStatus))
		} else {
			logrus.WithFields(logrus.Fields{
				"node_name": node.Name,
				"job_name":  job.Name,
				"policy":    cpuManagerStatus.Policy,
			}).Info("Submit cpu manager job")
			setUpdateStatus(newNode, toRunningStatus(cpuManagerStatus, job))
		}
	}

	if reflect.DeepEqual(newNode, node) {
		return node, nil
	}
	return h.nodeClient.Update(newNode)
}

func (h *cpuManagerNodeHandler) OnJobChanged(_ string, job *batchv1.Job) (*batchv1.Job, error) {
	if job == nil || job.Labels == nil {
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

	// someone delete the cpu manager job
	if job.DeletionTimestamp != nil {
		logrus.WithField("job_name", job.Name).Warn("CPU Manager job was deleted before it finished")
		return nil, h.updateCPUManagerStatus(node, job, CPUManagerFailedStatus)
	}

	return job, nil
}

func (h *cpuManagerNodeHandler) OnPodChanged(_ string, pod *corev1.Pod) (*corev1.Pod, error) {
	if pod == nil || pod.Labels == nil || pod.DeletionTimestamp != nil {
		return pod, nil
	}
	jobName, found := pod.Labels[batchv1.JobNameLabel]
	exitCode := int32(0)
	if len(pod.Status.ContainerStatuses) > 0 && pod.Status.ContainerStatuses[0].State.Terminated != nil {
		exitCode = pod.Status.ContainerStatuses[0].State.Terminated.ExitCode
	}
	if found && strings.HasPrefix(jobName, CPUManagerJobPrefix) && exitCode > 0 {

		logFields := logrus.Fields{
			"job_name":            jobName,
			"pod_name":            pod.Name,
			"container_name":      pod.Status.ContainerStatuses[0].Name,
			"container_exit_code": exitCode,
		}

		job, err := h.jobCache.Get(h.namespace, jobName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrus.WithFields(logFields).Warn("CPU Manager job was not found")
				return pod, nil
			}
			logrus.WithFields(logFields).WithError(err).Error("Failed to get cpu manager job from cache")
			return nil, err
		}
		if job.DeletionTimestamp != nil {
			logrus.WithFields(logFields).Info("CPU Manager job is going to be deleted")
			return pod, nil
		}
		newJob := job.DeepCopy()
		newJob.Labels[util.LabelCPUManagerExitCode] = strconv.Itoa(int(exitCode))
		if reflect.DeepEqual(newJob, job) {
			return pod, nil
		}
		// Log container exit code that is not zero. This helps in troubleshooting,
		// especially in cases where a pod may have been evicted,
		// making it difficult to determine why an update to the CPU manager policy failed.
		logrus.WithFields(logFields).Error("container exit code is not 0, update exit code to cpu manager job")
		_, err = h.jobClient.Update(newJob)
		return nil, err
	}
	return pod, nil
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

	newNode := node.DeepCopy()
	setUpdateStatus(newNode, updateStatus)
	if reflect.DeepEqual(newNode, node) {
		return nil
	}
	if _, err := h.nodeClient.Update(newNode); err != nil {
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
			GenerateName: name.SafeConcatName(CPUManagerJobPrefix, node.Name) + "-",
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
	image, err := utilHelm.FetchImageFromHelmValues(h.clientset, h.namespace, releaseAppHarvesterName, []string{"generalJob", "image"})
	if err != nil {
		return nil, fmt.Errorf("failed to get harvester image (%s): %v", image.ImageName(), err)
	}

	job, err := h.jobClient.Create(h.getJob(updateStatus.Policy, node, image.ImageName()))
	if err != nil {
		return nil, err
	}

	return job, nil
}

func getCPUManagerRunningJobOnNode(jobCache ctlbatchv1.JobCache, nodeName string) ([]*batchv1.Job, error) {
	jobs, err := GetCPUManagerRunningJobsOnNodes(jobCache, []string{nodeName})
	if err != nil {
		return []*batchv1.Job{}, err
	}
	return jobs, nil
}

func GetCPUManagerRunningJobsOnNodes(jobCache ctlbatchv1.JobCache, nodeNames []string) ([]*batchv1.Job, error) {
	if len(nodeNames) == 0 {
		return []*batchv1.Job{}, fmt.Errorf("nodeNames size should > 0")
	}
	requirement, err := labels.NewRequirement(util.LabelCPUManagerUpdateNode, selection.In, nodeNames)
	if err != nil {
		return []*batchv1.Job{}, fmt.Errorf("failed to create requirement: %s", err.Error())
	}
	labelSelector := labels.NewSelector().Add(*requirement)
	jobs, err := jobCache.List("", labelSelector)
	if err != nil {
		return []*batchv1.Job{}, err
	}
	runningJobs := []*batchv1.Job{}
	for _, job := range jobs {
		if !condition.Cond(batchv1.JobComplete).IsTrue(job) && !condition.Cond(batchv1.JobFailed).IsTrue(job) {
			runningJobs = append(runningJobs, job)
		}
	}
	return runningJobs, nil
}
