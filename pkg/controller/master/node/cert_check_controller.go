package node

import (
	"context"
	"fmt"
	"time"

	"github.com/rancher/wrangler/v3/pkg/condition"
	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/certrotation"
	utilHelm "github.com/harvester/harvester/pkg/util/helm"
)

const (
	certCheckControllerName = "rke2-cert-check-controller"

	certCheckInterval      = 24 * time.Hour
	certCheckRetryDelay    = 1 * time.Hour
	certCheckExistingDelay = 2 * time.Minute
	certCheckJitterMax     = 5
)

type certCheckNodeHandler struct {
	nodeController ctlcorev1.NodeController
	nodeCache      ctlcorev1.NodeCache
	jobCache       ctlbatchv1.JobCache
	jobClient      ctlbatchv1.JobClient
	clientset      kubernetes.Interface
	namespace      string
}

func CertCheckRegister(ctx context.Context, management *config.Management, options config.Options) error {
	jobs := management.BatchFactory.Batch().V1().Job()
	nodes := management.CoreFactory.Core().V1().Node()

	h := &certCheckNodeHandler{
		nodeController: nodes,
		nodeCache:      nodes.Cache(),
		jobCache:       jobs.Cache(),
		jobClient:      jobs,
		clientset:      management.ClientSet,
		namespace:      options.Namespace,
	}

	nodes.OnChange(ctx, certCheckControllerName, h.OnNodeChanged)
	jobs.OnChange(ctx, certCheckControllerName, h.OnJobChanged)

	return nil
}

func (h *certCheckNodeHandler) OnNodeChanged(_ string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return node, nil
	}

	logger := logrus.WithFields(logrus.Fields{
		"controller": certCheckControllerName,
		"node":       node.Name,
	})

	delay, err := h.reconcileNode(logger, node)
	if err != nil {
		// Re-enqueue with a backoff so transient errors recover without a
		// tight loop. We do NOT return the error to wrangler because that
		// would also re-queue
		logger.WithError(err).Warn("cert-check reconcile failed; will retry")
		h.nodeController.EnqueueAfter(node.Name, certCheckRetryDelay)
		return node, nil
	}

	if delay > 0 {
		h.nodeController.EnqueueAfter(node.Name, delay)
	}
	return node, nil
}

func (h *certCheckNodeHandler) reconcileNode(logger *logrus.Entry, node *corev1.Node) (time.Duration, error) {
	running, err := h.runningCheckJobsForNode(node.Name)
	if err != nil {
		return 0, fmt.Errorf("list running cert-check jobs: %w", err)
	}
	if len(running) > 0 {
		jobNames := make([]string, 0, len(running))
		for _, j := range running {
			jobNames = append(jobNames, j.Name)
		}
		logger.WithField("jobs", jobNames).Debug("cert-check Job already in flight; waiting")
		return certCheckExistingDelay, nil
	}

	until := h.untilNextCheck(logger, node, time.Now())
	if until > 0 {
		logger.WithField("until", until).Debug("cert-check not yet due; rescheduling")
		return until, nil
	}

	h.pruneJobs(node.Name)

	job, err := h.dispatchCheckJob(node)
	if err != nil {
		return 0, fmt.Errorf("dispatch cert-check job: %w", err)
	}
	logger.WithField("job", job.Name).Info("dispatched RKE2 cert-check Job")

	jitter := time.Duration(rand.Intn(certCheckJitterMax)) * time.Second
	return certCheckInterval + jitter, nil
}

func (h *certCheckNodeHandler) untilNextCheck(logger *logrus.Entry, node *corev1.Node, now time.Time) time.Duration {
	info, exists, err := certrotation.LoadCertInfoFromAnnotations(node.Annotations)
	if err != nil {
		logger.WithError(err).Warn("malformed rke2-certs annotation; treating as due")
		return 0
	}
	if !exists {
		return 0
	}

	jitter := time.Duration(rand.Intn(certCheckJitterMax)) * time.Second
	last := info.UpdatedAt.Time
	next := last.Add(certCheckInterval + jitter)
	if !now.Before(next) {
		return 0
	}
	return next.Sub(now)
}

func (h *certCheckNodeHandler) runningCheckJobsForNode(nodeName string) ([]*batchv1.Job, error) {
	all, err := h.jobCache.List(h.namespace, certrotation.CheckJobSelector(nodeName, nil, nil))
	if err != nil {
		return nil, err
	}
	out := make([]*batchv1.Job, 0, len(all))
	for _, j := range all {
		if j.DeletionTimestamp != nil {
			continue
		}
		if condition.Cond(batchv1.JobComplete).IsTrue(j) {
			continue
		}
		if condition.Cond(batchv1.JobFailed).IsTrue(j) {
			continue
		}
		out = append(out, j)
	}
	return out, nil
}

func (h *certCheckNodeHandler) OnJobChanged(_ string, job *batchv1.Job) (*batchv1.Job, error) {
	if job == nil || job.Labels == nil || job.DeletionTimestamp != nil {
		return job, nil
	}
	if action, ok := job.Labels[certrotation.LabelRKE2CertAction]; !ok || action != certrotation.CertActionCheck {
		return job, nil
	}
	if job.Labels[certrotation.LabelRKE2CertCheckPurpose] != string(certrotation.CertCheckPurposeSchedule) {
		return job, nil
	}

	nodeName := job.Labels[certrotation.LabelRKE2CertNode]
	logger := logrus.WithFields(logrus.Fields{
		"controller": certCheckControllerName,
		"node":       nodeName,
		"job":        job.Name,
	})

	switch {
	case condition.Cond(batchv1.JobComplete).IsTrue(job):
		logger.Info("RKE2 cert-check Job completed successfully")
	case condition.Cond(batchv1.JobFailed).IsTrue(job):
		logger.Warn("RKE2 cert-check Job failed; will retry on next reconcile (see Job's pod logs)")
	default:
		// Not yet terminal.
		return job, nil
	}

	h.pruneJobs(nodeName)

	// Wake the node so a new Job is scheduled (or the next interval is set).
	h.nodeController.Enqueue(nodeName)
	return job, nil
}

func (h *certCheckNodeHandler) dispatchCheckJob(node *corev1.Node) (*batchv1.Job, error) {
	image, err := utilHelm.FetchImageFromHelmValues(h.clientset, h.namespace, util.HarvesterChartReleaseName, []string{"generalJob", "image"})
	if err != nil {
		return nil, fmt.Errorf("get harvester generalJob image: %w", err)
	}

	job := certrotation.BuildCheckJob(certrotation.CheckJobSpec{
		Node:                node,
		Namespace:           h.namespace,
		Image:               image.ImageName(),
		HelperConfigMapName: helperConfigMapName,
		Purpose:             certrotation.CertCheckPurposeSchedule,
		Generation:          nil,
	})
	return h.jobClient.Create(job)
}

func (h *certCheckNodeHandler) pruneJobs(nodeName string) {
	logger := logrus.WithFields(logrus.Fields{
		"controller": certCheckControllerName,
		"node":       nodeName,
	})
	if err := certrotation.PruneJobs(
		h.jobCache, h.jobClient, h.namespace,
		certrotation.CheckJobSelector(nodeName, ptr.To(certrotation.CertCheckPurposeSchedule), nil),
		logger,
	); err != nil {
		logger.WithError(err).Warn("prune cert-check jobs failed; will retry on next reconcile")
	}
}
