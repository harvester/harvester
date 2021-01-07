package upgrade

import (
	"fmt"

	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	apisv1alpha1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/controller/master/node"
)

const (
	agentComponent    = "agent"
	serverComponent   = "server"
	manifestComponent = "manifest"

	labelArch               = "kubernetes.io/arch"
	labelCriticalAddonsOnly = "CriticalAddonsOnly"

	defaultTTLSecondsAfterFinished = 900
)

func setNodeUpgradeStatus(upgrade *v1alpha1.Upgrade, nodeName string, state, reason, message string) {
	if upgrade == nil {
		return
	}
	if upgrade.Status.NodeStatuses == nil {
		upgrade.Status.NodeStatuses = make(map[string]v1alpha1.NodeUpgradeStatus)
	}
	if current, ok := upgrade.Status.NodeStatuses[nodeName]; ok &&
		current.State == state && current.Reason == reason && current.Message == message {
		return
	}
	upgrade.Status.NodeStatuses[nodeName] = v1alpha1.NodeUpgradeStatus{
		State:   state,
		Reason:  reason,
		Message: message,
	}
	if state == stateFailed {
		setNodesUpgradedCondition(upgrade, v1.ConditionFalse, reason, message)
	}
}

func setNodesUpgradedCondition(upgrade *v1alpha1.Upgrade, status v1.ConditionStatus, reason, message string) {
	apisv1alpha1.NodesUpgraded.SetStatus(upgrade, string(status))
	apisv1alpha1.NodesUpgraded.Reason(upgrade, reason)
	apisv1alpha1.NodesUpgraded.Message(upgrade, message)
	markComplete(upgrade)
}

func setHelmChartUpgradeStatus(upgrade *v1alpha1.Upgrade, status v1.ConditionStatus, reason, message string) {
	if upgrade == nil ||
		v1alpha1.SystemServicesUpgraded.IsTrue(upgrade) ||
		v1alpha1.SystemServicesUpgraded.IsFalse(upgrade) {
		return
	}
	v1alpha1.SystemServicesUpgraded.SetStatus(upgrade, string(status))
	v1alpha1.SystemServicesUpgraded.Reason(upgrade, reason)
	v1alpha1.SystemServicesUpgraded.Message(upgrade, message)
	markComplete(upgrade)
}

func markComplete(upgrade *v1alpha1.Upgrade) {
	if upgrade.Labels == nil {
		upgrade.Labels = make(map[string]string)
	}
	if v1alpha1.SystemServicesUpgraded.IsTrue(upgrade) &&
		v1alpha1.NodesUpgraded.IsTrue(upgrade) {
		v1alpha1.UpgradeCompleted.True(upgrade)
		upgrade.Labels[upgradeStateLabel] = stateSucceeded
	}
	if v1alpha1.SystemServicesUpgraded.IsFalse(upgrade) ||
		v1alpha1.NodesUpgraded.IsFalse(upgrade) {
		v1alpha1.UpgradeCompleted.False(upgrade)
		upgrade.Labels[upgradeStateLabel] = stateFailed
	}
}

func serverPlan(upgrade *apisv1alpha1.Upgrade, disableEviction bool) *upgradev1.Plan {
	plan := basePlan(upgrade, disableEviction)
	plan.Name = fmt.Sprintf("%s-server", upgrade.Name)
	plan.Labels[harvesterUpgradeComponentLabel] = serverComponent
	plan.Spec.NodeSelector.MatchExpressions = []metav1.LabelSelectorRequirement{
		{
			Key:      node.KubeControlPlaneNodeLabelKey,
			Operator: metav1.LabelSelectorOpIn,
			Values:   []string{"true"},
		},
	}
	return plan
}

func agentPlan(upgrade *apisv1alpha1.Upgrade) *upgradev1.Plan {
	plan := basePlan(upgrade, false)
	plan.Name = fmt.Sprintf("%s-agent", upgrade.Name)
	plan.Labels[harvesterUpgradeComponentLabel] = agentComponent
	plan.Spec.NodeSelector.MatchExpressions = []metav1.LabelSelectorRequirement{
		{
			Key:      node.KubeControlPlaneNodeLabelKey,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
	}
	return plan
}

func basePlan(upgrade *apisv1alpha1.Upgrade, disableEviction bool) *upgradev1.Plan {
	version := upgrade.Spec.Version
	return &upgradev1.Plan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      upgrade.Name,
			Namespace: k3osSystemNamespace,
			Labels: map[string]string{
				harvesterVersionLabel: version,
				harvesterUpgradeLabel: upgrade.Name,
			},
		},
		Spec: upgradev1.PlanSpec{
			Concurrency: int64(1),
			Version:     version,
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					harvesterManagedLabel: "true",
				},
			},
			ServiceAccountName: k3osUpgradeServiceAccount,
			Drain: &upgradev1.DrainSpec{
				Force:           true,
				DisableEviction: disableEviction,
			},
			Tolerations: []v1.Toleration{
				{
					Key:      labelCriticalAddonsOnly,
					Operator: v1.TolerationOpExists,
				},
				{
					Key:      "kubevirt.io/drain",
					Operator: v1.TolerationOpExists,
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      node.KubeControlPlaneNodeLabelKey,
					Operator: v1.TolerationOpExists,
					Effect:   v1.TaintEffectNoExecute,
				},
				{
					Key:      labelArch,
					Operator: v1.TolerationOpEqual,
					Effect:   v1.TaintEffectNoSchedule,
					Value:    "amd64",
				},
				{
					Key:      labelArch,
					Operator: v1.TolerationOpEqual,
					Effect:   v1.TaintEffectNoSchedule,
					Value:    "arm64",
				},
				{
					Key:      labelArch,
					Operator: v1.TolerationOpEqual,
					Effect:   v1.TaintEffectNoSchedule,
					Value:    "arm",
				},
			},
			Prepare: &upgradev1.ContainerSpec{
				Image: fmt.Sprintf("%s:%s", upgradeImageRepository, version),
				Args:  []string{"--prepare"},
			},
			Upgrade: &upgradev1.ContainerSpec{
				Image: fmt.Sprintf("%s:%s", upgradeImageRepository, version),
			},
		},
	}
}

func applyManifestsJob(upgrade *apisv1alpha1.Upgrade) *batchv1.Job {
	version := upgrade.Spec.Version
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-apply-manifests", upgrade.Name),
			Namespace: upgrade.Namespace,
			Labels: map[string]string{
				harvesterVersionLabel: version,
				harvesterUpgradeLabel: upgrade.Name,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: pointer.Int32Ptr(defaultTTLSecondsAfterFinished),
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						harvesterVersionLabel:          version,
						harvesterUpgradeLabel:          upgrade.Name,
						harvesterUpgradeComponentLabel: manifestComponent,
					},
				},
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyNever,
					Containers: []v1.Container{
						{
							Name:  "apply",
							Image: fmt.Sprintf("%s:%s", upgradeImageRepository, version),
							Command: []string{
								"kubectl",
								"apply",
								"-f",
								"/manifests",
							},
						},
					},
					ServiceAccountName: "harvester",
					Tolerations: []v1.Toleration{
						{
							Key:      labelCriticalAddonsOnly,
							Operator: v1.TolerationOpExists,
						},
						{
							Key:      node.KubeControlPlaneNodeLabelKey,
							Operator: v1.TolerationOpExists,
							Effect:   v1.TaintEffectNoExecute,
						},
						{
							Key:      labelArch,
							Operator: v1.TolerationOpEqual,
							Effect:   v1.TaintEffectNoSchedule,
							Value:    "amd64",
						},
						{
							Key:      labelArch,
							Operator: v1.TolerationOpEqual,
							Effect:   v1.TaintEffectNoSchedule,
							Value:    "arm64",
						},
						{
							Key:      labelArch,
							Operator: v1.TolerationOpEqual,
							Effect:   v1.TaintEffectNoSchedule,
							Value:    "arm",
						},
					},
				},
			},
		},
	}
}

const (
	testJobName       = "test-job"
	testPlanName      = "test-plan"
	testNodeName      = "test-node"
	testUpgradeName   = "test-upgrade"
	testVersion       = "test-version"
	testPlanHash      = "test-hash"
	testAgentPlanHash = "test-agent-hash"
)

func newTestNodeJobBuilder() *jobBuilder {
	return newJobBuilder(testJobName).
		WithLabel(upgradePlanLabel, testPlanName).
		WithLabel(upgradeNodeLabel, testNodeName)
}

func newTestPlanBuilder() *planBuilder {
	return newPlanBuilder(testPlanName).
		Version(testVersion).
		WithLabel(harvesterUpgradeLabel, testUpgradeName).
		Hash(testPlanHash)
}

func newTestChartJobBuilder() *jobBuilder {
	return newJobBuilder(testJobName).
		WithLabel(helmChartLabel, harvesterChartname)
}

func newTestUpgradeBuilder() *upgradeBuilder {
	return newUpgradeBuilder(testUpgradeName).
		WithLabel(harvesterLatestUpgradeLabel, "true").
		Version(testVersion)
}

type jobBuilder struct {
	job *batchv1.Job
}

func newJobBuilder(name string) *jobBuilder {
	return &jobBuilder{
		job: &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}
}

func (j *jobBuilder) WithLabel(key, value string) *jobBuilder {
	if j.job.Labels == nil {
		j.job.Labels = make(map[string]string)
	}
	j.job.Labels[key] = value
	return j
}

func (j *jobBuilder) Running() *jobBuilder {
	j.job.Status.Active = 1
	return j
}

func (j *jobBuilder) Completed() *jobBuilder {
	j.job.Status.Succeeded = 1
	j.job.Status.Conditions = append(j.job.Status.Conditions, batchv1.JobCondition{
		Type:   batchv1.JobComplete,
		Status: "True",
	})
	return j
}

func (j *jobBuilder) Failed(reason, message string) *jobBuilder {
	j.job.Status.Failed = 1
	j.job.Status.Conditions = append(j.job.Status.Conditions, batchv1.JobCondition{
		Type:    batchv1.JobFailed,
		Status:  "True",
		Reason:  reason,
		Message: message,
	})
	return j
}

func (j *jobBuilder) Build() *batchv1.Job {
	return j.job
}

type upgradeBuilder struct {
	upgrade *apisv1alpha1.Upgrade
}

func newUpgradeBuilder(name string) *upgradeBuilder {
	return &upgradeBuilder{
		upgrade: &apisv1alpha1.Upgrade{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: harvesterSystemNamespace,
			},
		},
	}
}

func (p *upgradeBuilder) WithLabel(key, value string) *upgradeBuilder {
	if p.upgrade.Labels == nil {
		p.upgrade.Labels = make(map[string]string)
	}
	p.upgrade.Labels[key] = value
	return p
}

func (p *upgradeBuilder) Version(version string) *upgradeBuilder {
	p.upgrade.Spec.Version = version
	return p
}

func (p *upgradeBuilder) NodeUpgradeStatus(nodeName string, state, reason, message string) *upgradeBuilder {
	setNodeUpgradeStatus(p.upgrade, nodeName, state, reason, message)
	return p
}

func (p *upgradeBuilder) NodesUpgradedCondition(status v1.ConditionStatus, reason, message string) *upgradeBuilder {
	setNodesUpgradedCondition(p.upgrade, status, reason, message)
	return p
}

func (p *upgradeBuilder) ChartUpgradeStatus(status v1.ConditionStatus, reason, message string) *upgradeBuilder {
	setHelmChartUpgradeStatus(p.upgrade, status, reason, message)
	return p
}

func (p *upgradeBuilder) InitStatus() *upgradeBuilder {
	initStatus(p.upgrade)
	return p
}

func (p *upgradeBuilder) Build() *apisv1alpha1.Upgrade {
	return p.upgrade
}

type planBuilder struct {
	plan *upgradev1.Plan
}

func newPlanBuilder(name string) *planBuilder {
	return &planBuilder{
		plan: &upgradev1.Plan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: k3osSystemNamespace,
			},
		},
	}
}

func (p *planBuilder) WithLabel(key, value string) *planBuilder {
	if p.plan.Labels == nil {
		p.plan.Labels = make(map[string]string)
	}
	p.plan.Labels[key] = value
	return p
}

func (p *planBuilder) Version(version string) *planBuilder {
	p.plan.Spec.Version = version
	return p
}

func (p *planBuilder) Hash(hash string) *planBuilder {
	p.plan.Status.LatestHash = hash
	return p
}

func (p *planBuilder) Build() *upgradev1.Plan {
	return p.plan
}

type nodeBuilder struct {
	node *v1.Node
}

func newNodeBuilder(name string) *nodeBuilder {
	return &nodeBuilder{
		node: &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}
}

func (n *nodeBuilder) ControlPlane() *nodeBuilder {
	n.WithLabel(node.KubeControlPlaneNodeLabelKey, "true")
	return n
}

func (n *nodeBuilder) Managed() *nodeBuilder {
	n.WithLabel(harvesterManagedLabel, "true")
	return n
}

func (n *nodeBuilder) WithLabel(key, value string) *nodeBuilder {
	if n.node.Labels == nil {
		n.node.Labels = make(map[string]string)
	}
	n.node.Labels[key] = value
	return n
}

func (n *nodeBuilder) Build() *v1.Node {
	return n.node
}
