package fake

import (
	"context"
	"fmt"
	"strings"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	ctlhelmv1 "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	ctlbatchv1 "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/config"
)

type handler struct {
	job  ctlbatchv1.JobController
	helm ctlhelmv1.HelmChartController
}

func RegisterFakeControllers(ctx context.Context, management *config.Management, opts config.Options) error {
	jc := management.BatchFactory.Batch().V1().Job()
	hc := management.HelmFactory.Helm().V1().HelmChart()
	h := &handler{
		job:  jc,
		helm: hc,
	}

	hc.OnChange(ctx, "fake-helmchart-controller", h.OnHelmChartChange)
	hc.OnRemove(ctx, "fake-helmchart-controller-remove", h.OnHelmChartDelete)
	return nil
}

func (h *handler) OnHelmChartChange(key string, hc *helmv1.HelmChart) (*helmv1.HelmChart, error) {
	if hc == nil || hc.DeletionTimestamp != nil {
		return hc, nil
	}

	jobName, err := h.createJob(hc, true)
	if err != nil {
		return hc, err
	}

	hc.Status.JobName = jobName
	return h.helm.Update(hc)
}

func (h *handler) OnHelmChartDelete(key string, hc *helmv1.HelmChart) (*helmv1.HelmChart, error) {
	if hc.DeletionTimestamp == nil {
		return nil, nil
	}

	jobName, err := h.createJob(hc, false)
	if err != nil {
		return hc, err
	}

	hc.Status.JobName = jobName
	return h.helm.Update(hc)
}

// createJob will submit fake jobs on creation/deletion events for helm charts
// this allows us to mock job failures to ensure addon gets to correct failed state
func (h *handler) createJob(hc *helmv1.HelmChart, install bool) (string, error) {
	if hc.Status.JobName != "" {
		return hc.Status.JobName, nil
	}

	var pattern string
	if install {
		pattern = "install"
	} else {
		pattern = "delete"
	}

	jobObj, err := h.job.Get(hc.Namespace, fmt.Sprintf("helm-%s-%s", pattern, hc.Name), metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return "", err
	}

	// job exists nothing to be done
	if err == nil {
		return jobObj.Name, nil
	}

	args := []string{}
	if strings.Contains(hc.Name, "fail") {
		args = append(args, "exit 1")
	} else {
		args = append(args, "exit 0")
	}
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("helm-%s-%s", pattern, hc.Name),
			Namespace: hc.Namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &[]int32{1}[0],
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "fake",
							Image:   "alpine",
							Command: []string{"/bin/sh", "-c"},
							Args:    args,
						},
						{
							Name:    "fake2",
							Image:   "alpine",
							Command: []string{"/bin/sh", "-c"},
							Args:    args,
						},
					},
				},
			},
		},
	}

	jObj, err := h.job.Create(j)
	return jObj.Name, err
}
