package setting

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/rancher/wrangler/v3/pkg/condition"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/certrotation"
	utilHelm "github.com/harvester/harvester/pkg/util/helm"
)

const (
	idleReconcileInterval           = 1 * time.Hour
	activeReconcileInterval         = 30 * time.Second
	nodeRotateTimeout               = 15 * time.Minute
	nodeVerifyTimeout               = 10 * time.Minute
	rotationStaleThreshold          = nodeRotateTimeout + nodeVerifyTimeout
	verifyJobMissingRedispatchAfter = activeReconcileInterval
)

type nodeRole int

const (
	roleManagement nodeRole = iota // control-plane / etcd / witness
	roleWorker
)

func (h *Handler) syncAutoRotateRKE2Certs(setting *harvesterv1.Setting) error {
	logger := logrus.WithField("setting", setting.Name)

	cfg := &settings.AutoRotateRKE2Certs{}
	if setting.Value != "" {
		if err := json.Unmarshal([]byte(setting.Value), cfg); err != nil {
			logger.WithError(err).WithField("value", setting.Value).
				Error("failed to unmarshal auto-rotate-rke2-certs value")
			return err
		}
	}

	state, err := certrotation.LoadClusterStateFromAnnotations(setting.Annotations)
	if err != nil {
		now := metav1.Now()
		logger.WithError(err).Warn("malformed rke2-cert-rotation-state annotation; resetting to idle")
		state = certrotation.ClusterRotationState{Phase: certrotation.PhaseIdle, StartedAt: now, UpdatedAt: now}
		if updErr := h.persistClusterState(setting, state); updErr != nil {
			return updErr
		}
	}
	if state.Phase == "" {
		state.Phase = certrotation.PhaseIdle
	}

	logger = logger.WithFields(logrus.Fields{
		"phase":      state.Phase,
		"generation": state.Generation,
	})

	if !cfg.Enable {
		if !certrotation.IsRotationInProgress(state.Phase) {
			logger.Debug("auto-rotation disabled and no rotation in progress; nothing to do")
			return nil
		}
		logger.Info("auto-rotation disabled but a rotation is still in progress; continuing to rotate")
	}

	newState, requeueAfter, err := h.advanceRotation(logger, setting, cfg, state)
	if err != nil {
		return err
	}

	if !certrotation.EqualState(state, newState) {
		if err := h.persistClusterState(setting, newState); err != nil {
			return err
		}
	}

	if requeueAfter > 0 {
		h.settingController.EnqueueAfter(setting.Name, requeueAfter)
	}
	return nil
}

func (h *Handler) advanceRotation(
	logger *logrus.Entry,
	setting *harvesterv1.Setting,
	cfg *settings.AutoRotateRKE2Certs,
	state certrotation.ClusterRotationState,
) (certrotation.ClusterRotationState, time.Duration, error) {
	switch state.Phase {
	case certrotation.PhaseIdle, certrotation.PhaseCompleted:
		return h.maybeStartRotation(logger, cfg, state)

	case certrotation.PhaseFailed:
		logger.WithField("lastError", state.LastError).
			Debug("rotation in failed phase; awaiting operator intervention")
		return state, 0, nil

	case certrotation.PhaseCPRotation:
		return h.advancePhase(logger, state, roleManagement, certrotation.PhaseWorkerRotation)

	case certrotation.PhaseWorkerRotation:
		return h.advancePhase(logger, state, roleWorker, certrotation.PhaseCompleted)

	default:
		// Unknown phase string: treat as a corruption of the annotation
		// and return to idle. The next reconcile will re-evaluate.
		logger.WithField("phase", state.Phase).Warn("unknown rotation phase; resetting to idle")
		return certrotation.ClusterRotationState{Phase: certrotation.PhaseIdle}, idleReconcileInterval, nil
	}
}

func (h *Handler) maybeStartRotation(
	logger *logrus.Entry,
	cfg *settings.AutoRotateRKE2Certs,
	state certrotation.ClusterRotationState,
) (certrotation.ClusterRotationState, time.Duration, error) {
	nodes, err := h.nodeCache.List(labels.Everything())
	if err != nil {
		return state, 0, fmt.Errorf("list nodes: %w", err)
	}

	threshold := time.Now().Add(time.Duration(cfg.ExpiringInHours) * time.Hour)
	earliest, earliestNode := earliestExpiry(nodes)
	if earliest.IsZero() {
		logger.Debugf("no node has %s annotation yet; will retry", certrotation.AnnotationRKE2CertInfo)
		return state, idleReconcileInterval, nil
	}

	if !earliest.Before(threshold) {
		until := earliest.Sub(threshold)
		next := idleReconcileInterval
		if until < next {
			next = until
		}
		logger.WithFields(logrus.Fields{
			"earliestExpiry":     earliest,
			"earliestExpiryNode": earliestNode,
			"threshold":          threshold,
			"requeueAfter":       next,
		}).Debug("no expiring certs; reconciling later")
		return state, next, nil
	}

	// Start a new rotation.
	now := metav1.Now()
	next := certrotation.ClusterRotationState{
		Generation: state.Generation + 1,
		Phase:      certrotation.PhaseCPRotation,
		StartedAt:  now,
		UpdatedAt:  now,
	}
	logger.WithFields(logrus.Fields{
		"newGeneration":      next.Generation,
		"earliestExpiry":     earliest,
		"earliestExpiryNode": earliestNode,
	}).Info("starting RKE2 cert rotation")
	return next, activeReconcileInterval, nil
}

func (h *Handler) advancePhase(
	logger *logrus.Entry,
	state certrotation.ClusterRotationState,
	role nodeRole,
	nextPhase string,
) (certrotation.ClusterRotationState, time.Duration, error) {
	nodes, err := h.nodeCache.List(labels.Everything())
	if err != nil {
		return state, 0, fmt.Errorf("list nodes: %w", err)
	}

	candidates := nodesForRole(nodes, role)
	node := pickNextNodeForRotation(candidates, state.Generation)
	now := metav1.Now()
	if node == nil {
		// All nodes in this phase are at the current generation; advance.
		next := state
		next.Phase = nextPhase
		next.CurrentNode = ""
		next.UpdatedAt = now
		if nextPhase == certrotation.PhaseCompleted {
			logger.WithField("generation", state.Generation).
				Info("RKE2 cert rotation completed successfully")
			return next, idleReconcileInterval, nil
		}
		logger.WithFields(logrus.Fields{
			"fromPhase": state.Phase,
			"toPhase":   nextPhase,
		}).Info("phase complete; advancing")
		return next, activeReconcileInterval, nil
	}

	nodeState, action, err := h.driveNode(logger.WithField("node", node.Name), node, state.Generation, now)
	if err != nil {
		return state, 0, err
	}

	switch action {
	case nodeActionWait:
		next := state
		next.CurrentNode = node.Name
		next.UpdatedAt = now
		return next, activeReconcileInterval, nil

	case nodeActionAdvance:
		next := state
		next.CurrentNode = ""
		next.UpdatedAt = now
		// pick node in the next prompt reconcile
		return next, time.Second, nil

	case nodeActionFailed:
		next := state
		next.Phase = certrotation.PhaseFailed
		next.CurrentNode = node.Name
		next.LastError = nodeState.Reason
		next.UpdatedAt = now
		logger.WithFields(logrus.Fields{
			"node":       node.Name,
			"generation": state.Generation,
			"reason":     nodeState.Reason,
		}).Error("node rotation failed; halting cluster-wide rotation")
		return next, 0, nil

	default:
		// unexpected action, keep in the same state
		return state, activeReconcileInterval, nil
	}
}

// nodeAction is the outcome of a single per-node decision pass.
type nodeAction int

const (
	nodeActionWait nodeAction = iota
	nodeActionAdvance
	nodeActionFailed
)

func (h *Handler) driveNode(
	logger *logrus.Entry,
	node *corev1.Node,
	generation int64,
	now metav1.Time,
) (certrotation.NodeRotationState, nodeAction, error) {
	current, _, err := certrotation.LoadNodeStateFromAnnotations(node.Annotations)
	if err != nil {
		logger.WithError(err).Warnf("malformed %s annotation; resetting", certrotation.AnnotationRKE2CertRotationNodeState)
	}
	if err != nil || current.Generation != generation {
		current = certrotation.NodeRotationState{
			Generation: generation,
			Phase:      certrotation.NodePhasePending,
			StartedAt:  now,
			UpdatedAt:  now,
		}
		if err := h.persistNodeStatus(node, current); err != nil {
			return current, nodeActionWait, err
		}
	}

	// Check for a stuck phase (UpdatedAt too far in the past).
	if (current.Phase == certrotation.NodePhaseRotating || current.Phase == certrotation.NodePhaseVerifying) && now.Sub(current.UpdatedAt.Time) > rotationStaleThreshold {
		logger.WithFields(logrus.Fields{
			"phase":   current.Phase,
			"stalled": now.Sub(current.UpdatedAt.Time),
		}).Warn("node rotation appears stuck; marking failed")
		stuckPhase := current.Phase
		current.Phase = certrotation.NodePhaseFailed
		current.Reason = fmt.Sprintf("phase %q stuck for %s without progress", stuckPhase, now.Sub(current.UpdatedAt.Time))
		current.UpdatedAt = now
		if err := h.persistNodeStatus(node, current); err != nil {
			return current, nodeActionWait, err
		}
		return current, nodeActionFailed, nil
	}

	role := roleOfNode(node)
	roleArg := "agent"
	if role == roleManagement {
		roleArg = "server"
	}

	switch current.Phase {
	case certrotation.NodePhaseCompleted:
		return current, nodeActionAdvance, nil

	case certrotation.NodePhaseFailed:
		return current, nodeActionFailed, nil

	case certrotation.NodePhasePending, "":
		// Dedupe: any in-flight rotate Job for this node?
		running, err := h.runningRotationJobsForNode(node.Name)
		if err != nil {
			return current, nodeActionWait, err
		}
		if len(running) > 0 {
			logger.WithField("job", running[0].Name).
				Debug("rotate Job already in flight; recording state and waiting")
			current.Phase = certrotation.NodePhaseRotating
			current.JobName = running[0].Name
			current.UpdatedAt = now
			if err := h.persistNodeStatus(node, current); err != nil {
				return current, nodeActionWait, err
			}
			return current, nodeActionWait, nil
		}
		h.pruneJobs(node.Name, true)
		// Dispatch a new rotate Job.
		job, err := h.dispatchRotateJob(node, generation, roleArg)
		if err != nil {
			return current, nodeActionWait, fmt.Errorf("dispatch rotate Job: %w", err)
		}
		logger.WithFields(logrus.Fields{
			"job":  job.Name,
			"role": roleArg,
		}).Info("dispatched RKE2 cert-rotate Job")
		current.Phase = certrotation.NodePhaseRotating
		current.JobName = job.Name
		current.UpdatedAt = now
		if err := h.persistNodeStatus(node, current); err != nil {
			return current, nodeActionWait, err
		}
		return current, nodeActionWait, nil

	case certrotation.NodePhaseRotating:
		// Wait for the rotate Job to finish.
		job, err := h.jobCache.Get(h.namespace, current.JobName)
		if err != nil {
			return current, nodeActionWait, err
		}
		if job == nil {
			// The Job we dispatched is gone (TTL'd, deleted manually,
			// etc.) without us ever observing completion. Treat as failed
			// to avoid silently hanging.
			logger.Warn("rotate Job disappeared before completion; marking failed")
			current.Phase = certrotation.NodePhaseFailed
			current.Reason = "rotate Job disappeared before completion"
			current.UpdatedAt = now
			if err := h.persistNodeStatus(node, current); err != nil {
				return current, nodeActionWait, err
			}
			return current, nodeActionFailed, nil
		}
		if condition.Cond(batchv1.JobFailed).IsTrue(job) {
			current.Phase = certrotation.NodePhaseFailed
			current.Reason = fmt.Sprintf("rotate Job %s failed; see pod logs", job.Name)
			current.UpdatedAt = now
			if err := h.persistNodeStatus(node, current); err != nil {
				return current, nodeActionWait, err
			}
			return current, nodeActionFailed, nil
		}
		if !condition.Cond(batchv1.JobComplete).IsTrue(job) {
			return current, nodeActionWait, nil
		}
		h.pruneJobs(node.Name, false)
		verifyJob, err := h.dispatchVerifyJob(node, generation)
		if err != nil {
			return current, nodeActionWait, fmt.Errorf("dispatch verify Job: %w", err)
		}
		logger.WithField("verifyJob", verifyJob.Name).Info("dispatched RKE2 cert-verify Job")
		current.Phase = certrotation.NodePhaseVerifying
		current.VerifyJobName = verifyJob.Name
		current.UpdatedAt = now
		if err := h.persistNodeStatus(node, current); err != nil {
			return current, nodeActionWait, err
		}
		return current, nodeActionWait, nil

	case certrotation.NodePhaseVerifying:
		verifyJob, err := h.jobCache.Get(h.namespace, current.VerifyJobName)
		if err != nil {
			return current, nodeActionWait, err
		}
		if verifyJob == nil {
			if now.Sub(current.UpdatedAt.Time) < verifyJobMissingRedispatchAfter {
				logger.Debug("verify Job not yet visible in cache; waiting")
				return current, nodeActionWait, nil
			}
			logger.Warn("verify Job missing; re-dispatching")
			redispatched, err := h.dispatchVerifyJob(node, generation)
			if err != nil {
				return current, nodeActionWait, fmt.Errorf("re-dispatch verify Job: %w", err)
			}
			current.VerifyJobName = redispatched.Name
			current.UpdatedAt = now
			if err := h.persistNodeStatus(node, current); err != nil {
				return current, nodeActionWait, err
			}
			return current, nodeActionWait, nil
		}
		if condition.Cond(batchv1.JobFailed).IsTrue(verifyJob) {
			current.Phase = certrotation.NodePhaseFailed
			current.Reason = fmt.Sprintf("verify Job %s failed; see pod logs", verifyJob.Name)
			current.UpdatedAt = now
			if err := h.persistNodeStatus(node, current); err != nil {
				return current, nodeActionWait, err
			}
			return current, nodeActionFailed, nil
		}
		if !condition.Cond(batchv1.JobComplete).IsTrue(verifyJob) {
			return current, nodeActionWait, nil
		}
		info, ok, err := certrotation.LoadCertInfoFromAnnotations(node.Annotations)
		if err != nil || !ok {
			logger.Debug("verify Job complete but cert annotation not yet observed; waiting")
			return current, nodeActionWait, nil
		}
		latest := info.UpdatedAt.Time
		floor := current.UpdatedAt.Time
		if !latest.After(floor) {
			logger.WithFields(logrus.Fields{
				"latest": latest,
				"floor":  floor,
			}).Debug("cert annotation not yet refreshed past verifying-phase floor; waiting")
			return current, nodeActionWait, nil
		}
		current.Phase = certrotation.NodePhaseCompleted
		current.UpdatedAt = now
		if err := h.persistNodeStatus(node, current); err != nil {
			return current, nodeActionWait, err
		}
		closest := info.ClosestExpiryTime.Time
		logger.WithField("closestExpiry", closest).Info("node rotation verified")
		return current, nodeActionAdvance, nil

	default:
		return current, nodeActionWait, nil
	}
}

func (h *Handler) rotationJobOnChanged(_ string, job *batchv1.Job) (*batchv1.Job, error) {
	if job == nil || job.Labels == nil || job.DeletionTimestamp != nil {
		return job, nil
	}
	isRotateJob := job.Labels[certrotation.LabelRKE2CertAction] == certrotation.CertActionRotate
	isVerifyJob := job.Labels[certrotation.LabelRKE2CertCheckPurpose] == string(certrotation.CertCheckPurposeVerify)
	if !isRotateJob && !isVerifyJob {
		return job, nil
	}
	if condition.Cond(batchv1.JobComplete).IsTrue(job) || condition.Cond(batchv1.JobFailed).IsTrue(job) {
		h.settingController.Enqueue(settings.AutoRotateRKE2CertsSettingName)
		nodeName := job.Labels[certrotation.LabelRKE2CertNode]
		h.pruneJobs(nodeName, isRotateJob)
	}
	return job, nil
}

func (h *Handler) pruneJobs(nodeName string, isRotateJob bool) {
	logger := logrus.WithFields(logrus.Fields{
		"node": nodeName,
	})
	if isRotateJob {
		sel := certrotation.RotateJobSelector(nodeName, nil)
		if err := certrotation.PruneJobs(
			h.jobCache, h.jobs, h.namespace, sel, logger,
		); err != nil {
			logger.WithError(err).Warn("prune cert-rotate jobs failed; will retry on next reconcile")
		}
	} else {
		sel := certrotation.CheckJobSelector(nodeName, ptr.To(certrotation.CertCheckPurposeVerify), nil)
		if err := certrotation.PruneJobs(
			h.jobCache, h.jobs, h.namespace, sel, logger,
		); err != nil {
			logger.WithError(err).Warn("prune cert-verify jobs failed; will retry on next reconcile")
		}
	}
}

func (h *Handler) dispatchRotateJob(node *corev1.Node, generation int64, role string) (*batchv1.Job, error) {
	image, err := utilHelm.FetchImageFromHelmValues(h.clientset, h.namespace, "harvester", []string{"generalJob", "image"})
	if err != nil {
		return nil, fmt.Errorf("get harvester generalJob image: %w", err)
	}
	return h.jobs.Create(
		certrotation.BuildRotateJob(certrotation.RotationJobSpec{
			Node:       node,
			Namespace:  h.namespace,
			Image:      image.ImageName(),
			Role:       role,
			Generation: generation,
		}))
}

func (h *Handler) runningRotationJobsForNode(nodeName string) ([]*batchv1.Job, error) {
	s := certrotation.RotateJobSelector(nodeName, nil)
	all, err := h.jobCache.List(h.namespace, s)
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

func (h *Handler) dispatchVerifyJob(node *corev1.Node, generation int64) (*batchv1.Job, error) {
	image, err := utilHelm.FetchImageFromHelmValues(h.clientset, h.namespace, util.HarvesterChartReleaseName, []string{"generalJob", "image"})
	if err != nil {
		return nil, fmt.Errorf("get harvester generalJob image: %w", err)
	}
	job := certrotation.BuildCheckJob(certrotation.CheckJobSpec{
		Node:                node,
		Namespace:           h.namespace,
		Image:               image.ImageName(),
		HelperConfigMapName: "harvester-helpers",
		Purpose:             certrotation.CertCheckPurposeVerify,
		Generation:          &generation,
	})
	return h.jobs.Create(job)
}

func (h *Handler) persistClusterState(setting *harvesterv1.Setting, state certrotation.ClusterRotationState) error {
	raw, err := certrotation.MarshalClusterState(state)
	if err != nil {
		return fmt.Errorf("marshal cluster rotation state: %w", err)
	}
	live, err := h.settings.Get(setting.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get setting %s: %w", setting.Name, err)
	}
	toUpdate := live.DeepCopy()
	if toUpdate.Annotations == nil {
		toUpdate.Annotations = map[string]string{}
	}
	if toUpdate.Annotations[certrotation.AnnotationRKE2CertRotationState] == raw {
		return nil
	}
	toUpdate.Annotations[certrotation.AnnotationRKE2CertRotationState] = raw
	if _, err := h.settings.Update(toUpdate); err != nil {
		return fmt.Errorf("update setting %s: %w", setting.Name, err)
	}
	return nil
}

func (h *Handler) persistNodeStatus(node *corev1.Node, status certrotation.NodeRotationState) error {
	raw, err := certrotation.MarshalNodeState(status)
	if err != nil {
		return fmt.Errorf("marshal node rotation status: %w", err)
	}
	live, err := h.nodeCache.Get(node.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get node %s: %w", node.Name, err)
	}
	toUpdate := live.DeepCopy()
	if toUpdate.Annotations == nil {
		toUpdate.Annotations = map[string]string{}
	}
	if toUpdate.Annotations[certrotation.AnnotationRKE2CertRotationNodeState] == raw {
		return nil
	}
	toUpdate.Annotations[certrotation.AnnotationRKE2CertRotationNodeState] = raw
	if _, err := h.nodeClient.Update(toUpdate); err != nil {
		return fmt.Errorf("update node %s: %w", node.Name, err)
	}
	return nil
}

func earliestExpiry(nodes []*corev1.Node) (time.Time, string) {
	var earliest time.Time
	var earliestNode string
	for _, n := range nodes {
		if n == nil {
			continue
		}
		info, ok, err := certrotation.LoadCertInfoFromAnnotations(n.Annotations)
		if err != nil || !ok {
			continue
		}
		t := info.ClosestExpiryTime.Time
		if earliest.IsZero() || t.Before(earliest) {
			earliest = t
			earliestNode = n.Name
		}
	}
	return earliest, earliestNode
}

func roleOfNode(node *corev1.Node) nodeRole {
	if node == nil {
		return roleWorker
	}
	if isManagementByLabels(node) {
		return roleManagement
	}
	return roleWorker
}

func isManagementByLabels(node *corev1.Node) bool {
	for _, key := range []string{
		"node-role.kubernetes.io/control-plane",
		"node-role.kubernetes.io/etcd",
	} {
		if v, ok := node.Labels[key]; ok && v == "true" {
			return true
		}
	}
	return false
}

func nodesForRole(nodes []*corev1.Node, role nodeRole) []*corev1.Node {
	out := make([]*corev1.Node, 0, len(nodes))
	for _, n := range nodes {
		if n == nil || n.DeletionTimestamp != nil {
			continue
		}
		if roleOfNode(n) == role {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func pickNextNodeForRotation(nodes []*corev1.Node, generation int64) *corev1.Node {
	for _, n := range nodes {
		st, ok, err := certrotation.LoadNodeStateFromAnnotations(n.Annotations)
		if err != nil || !ok {
			return n
		}
		if st.Generation != generation {
			return n
		}
		if st.Phase != certrotation.NodePhaseCompleted {
			return n
		}
	}
	return nil
}
