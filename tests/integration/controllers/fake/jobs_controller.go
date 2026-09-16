package fake

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

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

	logrus.Infof("fake helmchart-controller is registered")

	hc.OnChange(ctx, "fake-helmchart-controller", h.OnHelmChartChange)
	hc.OnRemove(ctx, "fake-helmchart-controller-deletion", h.OnHelmChartDelete)
	return nil
}

func (h *handler) OnHelmChartChange(_ string, hc *helmv1.HelmChart) (*helmv1.HelmChart, error) {
	if hc == nil || hc.DeletionTimestamp != nil {
		return hc, nil
	}

	// 1. Generate the expected hash for this specific state
	hcCopy := hc.DeepCopy()
	hcCopy, currentHash, changed, err := h.checkAndGenerateHash(hcCopy)
	if err != nil {
		return hc, err
	}

	// 2. Evaluate annotations for failure testing overrides
	var override bool
	if val, ok := hcCopy.Annotations[OverrideToFail]; ok && val == "true" {
		override = true
	}

	// 3. Reconcile the cluster state (Ensures matching Job exists)
	jobName := fmt.Sprintf("helm-install-%s", hc.Name)
	if err := h.createJobIfNotFound(hcCopy.Namespace, jobName, currentHash, override); err != nil {
		return hc, err
	}

	// 4. Skip to update the HelmChart Status, if job name && hash are both matched
	// for a running addon, the spec could change, but the job name is still `helm-install-...`
	if hcCopy.Status.JobName == jobName && !changed {
		return hc, nil
	}

	logrus.Infof("Linking job %s to helmChart %s/%s with hash %s, hash changed: %v", jobName, hcCopy.Namespace, hcCopy.Name, currentHash, changed)
	hcCopy.Status.JobName = jobName
	return h.helm.Update(hcCopy)
}

func (h *handler) OnHelmChartDelete(_ string, hc *helmv1.HelmChart) (*helmv1.HelmChart, error) {
	if hc == nil || hc.DeletionTimestamp == nil {
		return nil, nil
	}

	deleteJobName := fmt.Sprintf("helm-delete-%s", hc.Name)

	// 1. CONFIDENCE GUARD: If status is already locked onto this deletion task, hand-off is complete.
	if hc.Status.JobName == deleteJobName {
		return hc, nil
	}

	// 2. Calculate the target hash of the spec being torn down
	currentHash, err := calculateSpecHash(hc.Spec)
	if err != nil {
		return hc, err
	}

	// 3. Evaluate failure overrides from metadata
	var override bool
	if val, ok := hc.Annotations[OverrideToFail]; ok && val == "true" {
		override = true
	}

	// 4. Delegate everything to our smart helper!
	// It will fetch, inspect, purge if stale, or create a brand new job if missing.
	if err := h.createJobIfNotFound(hc.Namespace, deleteJobName, currentHash, override); err != nil {
		return hc, err // Safely bubbles up re-queues if a stale job was just deleted
	}

	// 5. Seal the status link to transfer ownership to the monitoring controller
	hcCopy := hc.DeepCopy()
	hcCopy.Status.JobName = deleteJobName

	logrus.Infof("Uninstallation job %s established for helmChart %s/%s with hash %s", deleteJobName, hc.Namespace, hc.Name, currentHash)
	return h.helm.Update(hcCopy)
}

func (h *handler) createJobIfNotFound(namespace, name, hash string, override bool) error {
	// 1. Try to fetch the existing job
	existingJob, err := h.job.Cache().Get(namespace, name)
	if err == nil {
		// Check if the old job is currently in the middle of being deleted
		if existingJob.DeletionTimestamp != nil {
			return fmt.Errorf("waiting for outdated job %s to finish deleting before recreating", name)
		}

		// Job exists and is active! Check if the configuration matches
		existingHash := existingJob.Annotations[LastAppliedHash]
		if existingHash == hash {
			// Perfect match. No action needed.
			return nil
		}

		// Hash mismatch! The HelmChart changed while this job was tracking old config.
		// We must delete the outdated job so a fresh one can be spun up.
		propPolicy := metav1.DeletePropagationBackground
		deleteOpts := metav1.DeleteOptions{PropagationPolicy: &propPolicy}
		if err := h.job.Delete(namespace, name, &deleteOpts); err != nil {
			return fmt.Errorf("failed to delete outdated job %s: %w", name, err)
		}

		// CRITICAL: Return an error to stop execution and trigger a quick retry.
		// This gives Kubernetes time to gracefully delete the old job and pods.
		return fmt.Errorf("outdated job %s deleted; retrying on next sync to allow cleanup", name)

	} else if !apierrors.IsNotFound(err) {
		return err
	}

	args := []string{}
	// The loop prints progress 5 times, sleeping 1 second between each step
	// Avoid the job to be done too fast, the real helm-job takes time to finish all the tasks
	loopScript := "for i in 1 2 3 4 5; do echo \"Current iteration: $i/5\"; sleep 1; done; "

	if strings.Contains(name, "fail") || override {
		// Appends the loop first, then terminates with a failure exit code
		args = append(args, loopScript+"echo \"Failure condition met, exiting with 1\"; exit 1")
	} else {
		// Appends the loop first, then terminates with a success exit code
		args = append(args, loopScript+"echo \"Success condition met, exiting with 0\"; exit 0")
	}

	// 2. Construct the new Job object if not found (or just deleted above)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				LastAppliedHash: hash, // Pair with helmchart
			},
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
	_, err = h.job.Create(job)
	logrus.Infof("new job %s is created, error result: %+v", job.Name, err)
	return err
}

// the `bool` return param is to indicate if the hash has changed
func (h *handler) checkAndGenerateHash(hc *helmv1.HelmChart) (*helmv1.HelmChart, string, bool, error) {
	// 1. Calculate the real hash using the pure utility function
	currentHashVal, err := calculateSpecHash(hc.Spec)
	if err != nil {
		return hc, "", false, err
	}

	// 2. Safely initialize metadata maps if needed
	if hc.Annotations == nil {
		hc.Annotations = make(map[string]string)
	}

	lastAppliedHashVal := hc.Annotations[LastAppliedHash]

	// 3. Track if adjustments were introduced
	if currentHashVal != lastAppliedHashVal {
		hc.Annotations[LastAppliedHash] = currentHashVal
		return hc, currentHashVal, true, nil
	}

	return hc, currentHashVal, false, nil
}

// calculateSpecHash generates a stable, deterministic SHA-256 hex string
// from a given specification structure.
func calculateSpecHash(spec interface{}) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)

	// Prevent character escaping and ensure uniform formatting
	enc.SetEscapeHTML(false)

	if err := enc.Encode(spec); err != nil {
		return "", err
	}

	// Compute the cryptographic checksum directly from the buffer bytes
	hashBytes := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(hashBytes[:]), nil
}
