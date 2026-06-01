package certrotation

import (
	"fmt"
	"sort"
	"time"

	"github.com/rancher/wrangler/v3/pkg/condition"
	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	"github.com/rancher/wrangler/v3/pkg/name"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/ptr"
)

const (
	SuccessfulJobsHistoryPerNodeLimit = 3
	FailedJobsHistoryPerNodeLimit     = 3
)

const (
	certCheckJobPrefix                   = "rke2-cert-check"
	certCheckScriptMountPath             = "/harvester-helpers"
	certCheckScriptPath                  = certCheckScriptMountPath + "/rke2-cert-check.sh"
	certCheckRootMountPath               = "/host"
	certCheckBackoffLimit          int32 = 5
	certCheckActiveDeadlineSeconds int64 = 300
)

type CheckJobSpec struct {
	Node                *corev1.Node
	Namespace           string
	Image               string
	HelperConfigMapName string
	Purpose             CertCheckPurpose
	Generation          *int64
}

func BuildCheckJob(spec CheckJobSpec) *batchv1.Job {
	jobLabels := checkJobLabelSet(
		spec.Node.Name,
		&spec.Purpose,
		spec.Generation,
	)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name.SafeConcatName(certCheckJobPrefix, string(spec.Purpose), spec.Node.Name) + "-",
			Namespace:    spec.Namespace,
			Labels:       jobLabels,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: spec.Node.APIVersion,
					Kind:       spec.Node.Kind,
					Name:       spec.Node.Name,
					UID:        spec.Node.UID,
				},
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:          ptr.To(certCheckBackoffLimit),
			ActiveDeadlineSeconds: ptr.To(certCheckActiveDeadlineSeconds),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: jobLabels},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					HostPID:       true,
					Containers: []corev1.Container{
						{
							Name:    "rke2-cert-check",
							Image:   spec.Image,
							Command: []string{"sh"},
							Args:    []string{"-e", certCheckScriptPath, spec.Node.Name},
							SecurityContext: &corev1.SecurityContext{
								Privileged: ptr.To(true),
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "host-root", MountPath: certCheckRootMountPath},
								{Name: "helpers", MountPath: certCheckScriptMountPath},
							},
							Env: []corev1.EnvVar{
								{Name: "HOST_DIR", Value: certCheckRootMountPath},
							},
						},
					},
					ServiceAccountName: "harvester",
					// Tolerate everything so cordoned / unschedulable nodes still get checks
					Tolerations: []corev1.Toleration{
						{Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
						{Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute},
					},
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      corev1.LabelHostname,
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{spec.Node.Name},
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
									Type: ptr.To(corev1.HostPathDirectory),
								},
							},
						},
						{
							Name: "helpers",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: spec.HelperConfigMapName,
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

func GenerationLabelValue(generation int64) string {
	return fmt.Sprintf("%d", generation)
}

const (
	rotationRootMountPath             = "/host"
	rotationScriptMountPath           = "/harvester-helpers"
	rotationJobPrefix                 = "rke2-cert-rotate"
	rotationScriptPath                = rotationScriptMountPath + "/rke2-cert-rotate.sh"
	rotationWaitReadyTimeoutSec int64 = 300
	rotationJobTimeoutInSec     int64 = rotationWaitReadyTimeoutSec * 2
)

type RotationJobSpec struct {
	Node       *corev1.Node
	Namespace  string
	Image      string
	Role       string
	Generation int64
}

func BuildRotateJob(spec RotationJobSpec) *batchv1.Job {
	jobLabels := rotateJobLabelSet(spec.Node.Name, &spec.Generation)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name.SafeConcatName(rotationJobPrefix, spec.Node.Name) + "-",
			Namespace:    spec.Namespace,
			Labels:       jobLabels,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: spec.Node.APIVersion,
					Kind:       spec.Node.Kind,
					Name:       spec.Node.Name,
					UID:        spec.Node.UID,
				},
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:          ptr.To(int32(0)),
			ActiveDeadlineSeconds: ptr.To(rotationJobTimeoutInSec),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: jobLabels},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					HostPID:       true,
					HostNetwork:   true,
					HostIPC:       true,
					DNSPolicy:     corev1.DNSClusterFirstWithHostNet,
					Containers: []corev1.Container{
						{
							Name:    "rke2-cert-rotate",
							Image:   spec.Image,
							Command: []string{"sh"},
							Args:    []string{"-e", rotationScriptPath, spec.Node.Name, spec.Role},
							SecurityContext: &corev1.SecurityContext{
								Privileged: ptr.To(true),
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "host-root", MountPath: rotationRootMountPath},
								{Name: "helpers", MountPath: rotationScriptMountPath},
							},
							Env: []corev1.EnvVar{
								{Name: "HOST_DIR", Value: rotationRootMountPath},
								{Name: "WAIT_READY_TIMEOUT", Value: fmt.Sprintf("%d", rotationWaitReadyTimeoutSec)},
							},
						},
					},
					ServiceAccountName: "harvester",
					Tolerations: []corev1.Toleration{
						{Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
						{Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute},
					},
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      corev1.LabelHostname,
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{spec.Node.Name},
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
									Type: ptr.To(corev1.HostPathDirectory),
								},
							},
						},
						{
							Name: "helpers",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "harvester-helpers",
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

func checkJobLabelSet(nodeName string, purpose *CertCheckPurpose, generation *int64) labels.Set {
	ls := labels.Set{
		LabelRKE2CertAction: CertActionCheck,
		LabelRKE2CertNode:   nodeName,
	}
	if purpose != nil {
		ls[LabelRKE2CertCheckPurpose] = string(*purpose)
	}
	if generation != nil {
		ls[LabelRKE2CertGeneration] = GenerationLabelValue(*generation)
	}
	return ls
}

func CheckJobSelector(nodeName string, purpose *CertCheckPurpose, generation *int64) labels.Selector {
	return labels.SelectorFromSet(checkJobLabelSet(nodeName, purpose, generation))
}

func rotateJobLabelSet(nodeName string, generation *int64) labels.Set {
	ls := labels.Set{
		LabelRKE2CertAction: CertActionRotate,
		LabelRKE2CertNode:   nodeName,
	}
	if generation != nil {
		ls[LabelRKE2CertGeneration] = GenerationLabelValue(*generation)
	}
	return ls
}

func RotateJobSelector(nodeName string, generation *int64) labels.Selector {
	return labels.SelectorFromSet(rotateJobLabelSet(nodeName, generation))
}

func PruneJobs(
	jobCache ctlbatchv1.JobCache,
	jobClient ctlbatchv1.JobClient,
	namespace string,
	selector labels.Selector,
	logger *logrus.Entry,
) error {
	all, err := jobCache.List(namespace, selector)
	if err != nil {
		return fmt.Errorf("list Jobs for pruning (selector=%s): %w", selector, err)
	}

	var successful, failed []*batchv1.Job
	for _, j := range all {
		if j.DeletionTimestamp != nil {
			continue
		}
		switch {
		case condition.Cond(batchv1.JobComplete).IsTrue(j):
			successful = append(successful, j)
		case condition.Cond(batchv1.JobFailed).IsTrue(j):
			failed = append(failed, j)
		}
	}

	// Sort newest-first so we keep the head and prune the tail.
	sort.Slice(successful, func(i, j int) bool {
		iTime := successfulJobTime(successful[i])
		jTime := successfulJobTime(successful[j])
		if iTime.Equal(jTime) {
			return failed[i].Name > failed[j].Name // Stable tie-breaker
		}
		return iTime.After(jTime)
	})
	sort.Slice(failed, func(i, j int) bool {
		iTime := failedJobTime(failed[i])
		jTime := failedJobTime(failed[j])
		if iTime.Equal(jTime) {
			return failed[i].Name > failed[j].Name // Stable tie-breaker
		}
		return iTime.After(jTime)
	})

	prune := func(category string, jobs []*batchv1.Job, limit int) {
		if len(jobs) <= limit {
			return
		}
		bg := metav1.DeletePropagationBackground
		for _, j := range jobs[limit:] {
			err := jobClient.Delete(j.Namespace, j.Name, &metav1.DeleteOptions{
				PropagationPolicy: &bg,
			})
			switch {
			case err == nil:
				logger.WithFields(logrus.Fields{
					"job":      j.Name,
					"category": category,
				}).Info("pruned terminal Job (history limit exceeded)")
			case apierrors.IsNotFound(err):
				// Already gone; harmless.
			default:
				logger.WithFields(logrus.Fields{
					"job":      j.Name,
					"category": category,
				}).WithError(err).Warn("failed to prune terminal Job; will retry next pass")
			}
		}
	}

	prune("successful", successful, SuccessfulJobsHistoryPerNodeLimit)
	prune("failed", failed, FailedJobsHistoryPerNodeLimit)
	return nil
}

func failedJobTime(job *batchv1.Job) time.Time {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return c.LastTransitionTime.Time
		}
	}
	// Fallback to job created time
	return job.CreationTimestamp.Time
}

func successfulJobTime(job *batchv1.Job) time.Time {
	if job.Status.CompletionTime != nil {
		return job.Status.CompletionTime.Time
	}
	// Fallback: If CompletionTime isn't populated yet, look for the condition timestamp
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return c.LastTransitionTime.Time
		}
	}
	// Fallback to job created time
	return job.CreationTimestamp.Time
}
