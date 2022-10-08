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
	return nil
}

func (h *handler) OnHelmChartChange(key string, hc *helmv1.HelmChart) (*helmv1.HelmChart, error) {
	if hc == nil || hc.DeletionTimestamp != nil {
		return hc, nil
	}

	if hc.Status.JobName != "" {
		return hc, nil
	}

	_, err := h.job.Get(hc.Namespace, fmt.Sprintf("helm-install-%s", hc.Name), metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return hc, err
	}

	// job exists nothing to be done
	if err == nil {
		return hc, nil
	}

	args := []string{}
	if strings.Contains(hc.Name, "fail") {
		args = append(args, "exit 1")
	} else {
		args = append(args, "exit 0")
	}
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("helm-install-%s", hc.Name),
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
	if err != nil {
		return hc, fmt.Errorf("error creating fake job: %v", err)
	}

	hc.Status.JobName = jObj.Name
	return h.helm.Update(hc)
}

func (h *handler) OnHelmChartDelete(key string, hc *helmv1.HelmChart) (*helmv1.HelmChart, error) {
	if hc.DeletionTimestamp == nil {
		return nil, nil
	}

	if err := h.job.Delete(hc.Namespace, fmt.Sprintf("helm-install-%s", hc.Name), &metav1.DeleteOptions{}); err != nil {
		return hc, err
	}

	return nil, h.helm.Delete(hc.Namespace, hc.Name, &metav1.DeleteOptions{})
}
