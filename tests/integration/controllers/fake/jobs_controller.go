package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	ctlhelmv1 "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/config"
)

const (
	OverrideToFail  = "fakejobcontroller/fail"
	LastAppliedHash = "fakejobcontroller/last-applied-hash"
)

type handler struct {
	job  ctlbatchv1.JobController
	helm ctlhelmv1.HelmChartController
}

func RegisterFakeControllers(ctx context.Context, management *config.Management, _ config.Options) error {
	jc := management.BatchFactory.Batch().V1().Job()
	hc := management.HelmFactory.Helm().V1().HelmChart()
	h := &handler{
		job:  jc,
		helm: hc,
	}

	hc.OnChange(ctx, "fake-helmchart-controller", h.OnHelmChartChange)
	hc.OnRemove(ctx, "fake-helmchart-controller-deletion", h.OnHelmChartDelete)
	return nil
}

func (h *handler) OnHelmChartChange(_ string, hc *helmv1.HelmChart) (*helmv1.HelmChart, error) {
	if hc == nil || hc.DeletionTimestamp != nil {
		return hc, nil
	}

	hcCopy := hc.DeepCopy()
	hcCopy, updated, err := h.checkAndGenerateHash(hcCopy)

	if err != nil {
		return hcCopy, err
	}

	if hcCopy.Status.JobName != "" && !updated {
		return hc, nil
	}

	var override bool
	if hcCopy.Annotations != nil {
		val, ok := hcCopy.Annotations[OverrideToFail]
		if ok && val == "true" {
			override = true
		}
	}

	jobName := fmt.Sprintf("helm-install-%s", hc.Name)
	if err := h.createJobIfNotFound(hcCopy.Namespace, jobName, override); err != nil {
		return hcCopy, err
	}

	hcCopy.Status.JobName = jobName
	return h.helm.Update(hcCopy)
}

func (h *handler) OnHelmChartDelete(_ string, hc *helmv1.HelmChart) (*helmv1.HelmChart, error) {
	if hc == nil || hc.DeletionTimestamp == nil {
		return nil, nil
	}

	var override bool
	if hc.Annotations != nil {
		val, ok := hc.Annotations[OverrideToFail]
		if ok && val == "true" {
			override = true
		}
	}
	if err := h.job.Delete(hc.Namespace, fmt.Sprintf("helm-install-%s", hc.Name), &metav1.DeleteOptions{}); err != nil {
		return hc, err
	}

	jobName := fmt.Sprintf("helm-delete-%s", hc.Name)
	if err := h.createJobIfNotFound(hc.Namespace, jobName, override); err != nil {
		return hc, err
	}

	if hc.Status.JobName == jobName {
		_, err := h.helm.Update(hc)
		if err != nil {
			return hc, err
		}
	}

	// wait for job to complete
	for h.waitForJobCompletion(hc.Namespace, jobName) == nil {
		time.Sleep(5 * time.Second)
	}

	return hc, nil
}

func (h *handler) createJobIfNotFound(namespace string, name string, override bool) error {
	foundJob, err := h.job.Cache().Get(namespace, name)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	if foundJob != nil {
		err = h.job.Delete(namespace, name, &metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	args := []string{}
	if strings.Contains(name, "fail") || override {
		args = append(args, "exit 1")
	} else {
		args = append(args, "exit 0")
	}
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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

	_, err = h.job.Create(j)
	return err
}

func (h *handler) waitForJobCompletion(namespace string, name string) error {
	j, err := h.job.Cache().Get(namespace, name)
	if err != nil {
		return err
	}

	if j.Status.CompletionTime == nil {
		return fmt.Errorf("waiting for job to complete")
	}

	return nil
}

func (h *handler) checkAndGenerateHash(hc *helmv1.HelmChart) (*helmv1.HelmChart, bool, error) {
	hcByte, err := json.Marshal(hc.Spec)
	if err != nil {
		return hc, false, err
	}

	if hc.Annotations == nil {
		hc.Annotations = make(map[string]string)
	}

	lastAppliedHashVal := hc.Annotations[LastAppliedHash]
	if string(hcByte) != lastAppliedHashVal {
		hc.Annotations[LastAppliedHash] = string(hcByte)
		return hc, true, nil
	}

	return hc, false, nil
}
