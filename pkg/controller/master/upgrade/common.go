package upgrade

import (
	"fmt"

	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/name"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/node"
)

const (
	nodeComponent     = "node"
	manifestComponent = "manifest"

	labelArch               = "kubernetes.io/arch"
	labelCriticalAddonsOnly = "CriticalAddonsOnly"

	// keep jobs for 7 days
	defaultTTLSecondsAfterFinished = 604800
)

func setNodeUpgradeStatus(upgrade *harvesterv1.Upgrade, nodeName string, state, reason, message string) {
	if upgrade == nil {
		return
	}
	if upgrade.Status.NodeStatuses == nil {
		upgrade.Status.NodeStatuses = make(map[string]harvesterv1.NodeUpgradeStatus)
	}
	if current, ok := upgrade.Status.NodeStatuses[nodeName]; ok &&
		current.State == state && current.Reason == reason && current.Message == message {
		return
	}
	upgrade.Status.NodeStatuses[nodeName] = harvesterv1.NodeUpgradeStatus{
		State:   state,
		Reason:  reason,
		Message: message,
	}
	if state == StateFailed {
		setNodesUpgradedCondition(upgrade, corev1.ConditionFalse, reason, message)
		return
	}

	if upgrade.Labels[upgradeStateLabel] == StateUpgradingNodes {
		for _, nodeStatus := range upgrade.Status.NodeStatuses {
			if nodeStatus.State != StateSucceeded {
				return
			}
		}
		setNodesUpgradedCondition(upgrade, corev1.ConditionTrue, "", "")
	}
}

func setImageReadyCondition(upgrade *harvesterv1.Upgrade, status corev1.ConditionStatus, reason, message string) {
	harvesterv1.ImageReady.SetStatus(upgrade, string(status))
	harvesterv1.ImageReady.Reason(upgrade, reason)
	harvesterv1.ImageReady.Message(upgrade, message)
	markComplete(upgrade)
}

func setRepoProvisionedCondition(upgrade *harvesterv1.Upgrade, status corev1.ConditionStatus, reason, message string) {
	harvesterv1.RepoProvisioned.SetStatus(upgrade, string(status))
	harvesterv1.RepoProvisioned.Reason(upgrade, reason)
	harvesterv1.RepoProvisioned.Message(upgrade, message)
	markComplete(upgrade)
}

func setNodesPreparedCondition(upgrade *harvesterv1.Upgrade, status corev1.ConditionStatus, reason, message string) {
	harvesterv1.NodesPrepared.SetStatus(upgrade, string(status))
	harvesterv1.NodesPrepared.Reason(upgrade, reason)
	harvesterv1.NodesPrepared.Message(upgrade, message)
	markComplete(upgrade)
}

func setNodesUpgradedCondition(upgrade *harvesterv1.Upgrade, status corev1.ConditionStatus, reason, message string) {
	harvesterv1.NodesUpgraded.SetStatus(upgrade, string(status))
	harvesterv1.NodesUpgraded.Reason(upgrade, reason)
	harvesterv1.NodesUpgraded.Message(upgrade, message)
	markComplete(upgrade)
}

func setUpgradeCompletedCondition(upgrade *harvesterv1.Upgrade, state string, status corev1.ConditionStatus, reason, message string) {
	upgrade.Labels[upgradeStateLabel] = state
	harvesterv1.UpgradeCompleted.SetStatus(upgrade, string(status))
	harvesterv1.UpgradeCompleted.Reason(upgrade, reason)
	harvesterv1.UpgradeCompleted.Message(upgrade, message)
}

func setHelmChartUpgradeStatus(upgrade *harvesterv1.Upgrade, status corev1.ConditionStatus, reason, message string) {
	if upgrade == nil ||
		harvesterv1.SystemServicesUpgraded.IsTrue(upgrade) ||
		harvesterv1.SystemServicesUpgraded.IsFalse(upgrade) {
		return
	}
	harvesterv1.SystemServicesUpgraded.SetStatus(upgrade, string(status))
	harvesterv1.SystemServicesUpgraded.Reason(upgrade, reason)
	harvesterv1.SystemServicesUpgraded.Message(upgrade, message)
	markComplete(upgrade)
}

func markComplete(upgrade *harvesterv1.Upgrade) {
	if upgrade.Labels == nil {
		upgrade.Labels = make(map[string]string)
	}
	if harvesterv1.SystemServicesUpgraded.IsTrue(upgrade) &&
		harvesterv1.NodesUpgraded.IsTrue(upgrade) {
		harvesterv1.UpgradeCompleted.True(upgrade)
		upgrade.Labels[upgradeStateLabel] = StateSucceeded
	}
	if harvesterv1.ImageReady.IsFalse(upgrade) || harvesterv1.RepoProvisioned.IsFalse(upgrade) ||
		harvesterv1.SystemServicesUpgraded.IsFalse(upgrade) || harvesterv1.NodesUpgraded.IsFalse(upgrade) {
		harvesterv1.UpgradeCompleted.False(upgrade)
		upgrade.Labels[upgradeStateLabel] = StateFailed
	}
}

func preparePlan(upgrade *harvesterv1.Upgrade) *upgradev1.Plan {
	planVersion := upgrade.Name

	// Use current running version because new images are not preloaded yet.
	imageVersion := upgrade.Status.PreviousVersion
	return &upgradev1.Plan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-prepare", upgrade.Name),
			Namespace: sucNamespace,
			Labels: map[string]string{
				harvesterUpgradeLabel:          upgrade.Name,
				harvesterUpgradeComponentLabel: nodeComponent,
			},
		},
		Spec: upgradev1.PlanSpec{
			Concurrency: int64(1),
			Version:     planVersion,
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					harvesterManagedLabel: "true",
				},
			},
			ServiceAccountName: upgradeServiceAccount,
			Tolerations: []corev1.Toleration{
				{
					Key:      labelCriticalAddonsOnly,
					Operator: corev1.TolerationOpExists,
				},
				{
					Key:      "kubevirt.io/drain",
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
				},
				{
					Key:      node.KubeControlPlaneNodeLabelKey,
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoExecute,
				},
				{
					Key:      labelArch,
					Operator: corev1.TolerationOpEqual,
					Effect:   corev1.TaintEffectNoSchedule,
					Value:    "amd64",
				},
				{
					Key:      labelArch,
					Operator: corev1.TolerationOpEqual,
					Effect:   corev1.TaintEffectNoSchedule,
					Value:    "arm64",
				},
				{
					Key:      labelArch,
					Operator: corev1.TolerationOpEqual,
					Effect:   corev1.TaintEffectNoSchedule,
					Value:    "arm",
				},
			},
			Upgrade: &upgradev1.ContainerSpec{
				Image:   fmt.Sprintf("%s:%s", upgradeImageRepository, imageVersion),
				Command: []string{"upgrade_node.sh"},
				Args:    []string{"prepare"},
				Env: []corev1.EnvVar{
					{
						Name:  "HARVESTER_UPGRADE_NAME",
						Value: upgrade.Name,
					},
				},
			},
		},
	}
}

func applyNodeJob(upgrade *harvesterv1.Upgrade, repoInfo *UpgradeRepoInfo, nodeName string, jobType string) *batchv1.Job {
	// Use the image tag in the upgrade repo because it's already preloaded and might contain updated codes.
	imageVersion := repoInfo.Release.Harvester
	hostPathDirectory := corev1.HostPathDirectory
	privileged := true
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.SafeConcatName(upgrade.Name, jobType, nodeName),
			Namespace: upgrade.Namespace,
			Labels: map[string]string{
				harvesterUpgradeLabel:          upgrade.Name,
				harvesterUpgradeComponentLabel: nodeComponent,
				harvesterNodeLabel:             nodeName,
				upgradeJobTypeLabel:            jobType,
			},
			OwnerReferences: []metav1.OwnerReference{
				upgradeReference(upgrade),
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: pointer.Int32Ptr(defaultTTLSecondsAfterFinished),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						harvesterUpgradeLabel:          upgrade.Name,
						harvesterUpgradeComponentLabel: nodeComponent,
						upgradeJobTypeLabel:            jobType,
					},
				},
				Spec: corev1.PodSpec{
					HostIPC:       true,
					HostPID:       true,
					HostNetwork:   true,
					DNSPolicy:     corev1.DNSClusterFirstWithHostNet,
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "apply",
							Image:   fmt.Sprintf("%s:%s", upgradeImageRepository, imageVersion),
							Command: []string{"upgrade_node.sh"},
							Args:    []string{jobType},
							Env: []corev1.EnvVar{
								{
									Name:  "HARVESTER_UPGRADE_NAME",
									Value: upgrade.Name,
								},
								{
									Name:  "HARVESTER_UPGRADE_NODE_NAME",
									Value: nodeName,
								},
								{
									Name: "HARVESTER_UPGRADE_POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "host-root", MountPath: "/host"},
							},
							SecurityContext: &corev1.SecurityContext{
								Privileged: &privileged,
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{
										corev1.Capability("CAP_SYS_BOOT"),
									},
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
											nodeName,
										},
									}},
								}},
							},
						},
					},
					Tolerations: getDefaultTolerations(),
					Volumes: []corev1.Volume{
						{
							Name: `host-root`,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/", Type: &hostPathDirectory,
								},
							},
						},
					},
				},
			},
		},
	}
}

func applyManifestsJob(upgrade *harvesterv1.Upgrade, repoInfo *UpgradeRepoInfo) *batchv1.Job {
	// Use the image tag in the upgrade repo because it's already preloaded and might contain updated codes.
	imageVersion := repoInfo.Release.Harvester
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.SafeConcatName(upgrade.Name, "apply-manifests"),
			Namespace: upgrade.Namespace,
			Labels: map[string]string{
				harvesterUpgradeLabel:          upgrade.Name,
				harvesterUpgradeComponentLabel: manifestComponent,
			},
			OwnerReferences: []metav1.OwnerReference{
				upgradeReference(upgrade),
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: pointer.Int32Ptr(defaultTTLSecondsAfterFinished),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						harvesterUpgradeLabel:          upgrade.Name,
						harvesterUpgradeComponentLabel: manifestComponent,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "apply",
							Image:   fmt.Sprintf("%s:%s", upgradeImageRepository, imageVersion),
							Command: []string{"upgrade_manifests.sh"},
							Env: []corev1.EnvVar{
								{
									Name:  "HARVESTER_UPGRADE_NAME",
									Value: upgrade.Name,
								},
							},
						},
					},
					ServiceAccountName: "harvester",
					Tolerations:        getDefaultTolerations(),
				},
			},
		},
	}
}

func getDefaultTolerations() []corev1.Toleration {
	return []corev1.Toleration{
		{
			Key:      corev1.TaintNodeUnschedulable,
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      node.KubeControlPlaneNodeLabelKey,
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoExecute,
		},
		{
			Key:      "kubevirt.io/drain",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      labelCriticalAddonsOnly,
			Operator: corev1.TolerationOpExists,
		},
		{
			Key:      corev1.TaintNodeUnreachable,
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoExecute,
		},
		{
			Key:      labelArch,
			Operator: corev1.TolerationOpEqual,
			Effect:   corev1.TaintEffectNoSchedule,
			Value:    "amd64",
		},
		{
			Key:      labelArch,
			Operator: corev1.TolerationOpEqual,
			Effect:   corev1.TaintEffectNoSchedule,
			Value:    "arm64",
		},
		{
			Key:      labelArch,
			Operator: corev1.TolerationOpEqual,
			Effect:   corev1.TaintEffectNoSchedule,
			Value:    "arm",
		},
	}
}

const (
	testJobName      = "test-job"
	testPlanName     = "test-plan"
	testNodeName     = "test-node"
	testUpgradeName  = "test-upgrade"
	testVersion      = "test-version"
	testUpgradeImage = "test-upgrade-image"
	testPlanHash     = "test-hash"
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
		WithLabel(harvesterUpgradeComponentLabel, manifestComponent)
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
				Name:      name,
				Namespace: upgradeNamespace,
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
	upgrade *harvesterv1.Upgrade
}

func newUpgradeBuilder(name string) *upgradeBuilder {
	return &upgradeBuilder{
		upgrade: &harvesterv1.Upgrade{
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

func (p *upgradeBuilder) WithAnnotation(key, value string) *upgradeBuilder {
	if p.upgrade.Annotations == nil {
		p.upgrade.Annotations = make(map[string]string)
	}
	p.upgrade.Annotations[key] = value
	return p
}

func (p *upgradeBuilder) WithImage(image string) *upgradeBuilder {
	p.upgrade.Spec.Image = fmt.Sprintf("%s/%s", upgradeNamespace, image)
	return p
}

func (p *upgradeBuilder) Version(version string) *upgradeBuilder {
	p.upgrade.Spec.Version = version
	return p
}

func (p *upgradeBuilder) ImageReadyCondition(status corev1.ConditionStatus, reason, message string) *upgradeBuilder {
	setImageReadyCondition(p.upgrade, status, reason, message)
	return p
}

func (p *upgradeBuilder) RepoProvisionedCondition(status corev1.ConditionStatus, reason, message string) *upgradeBuilder {
	setRepoProvisionedCondition(p.upgrade, status, "", "")
	return p
}

func (p *upgradeBuilder) NodeUpgradeStatus(nodeName string, state, reason, message string) *upgradeBuilder {
	setNodeUpgradeStatus(p.upgrade, nodeName, state, reason, message)
	return p
}

func (p *upgradeBuilder) ImageIDStatus(imageName string) *upgradeBuilder {
	p.upgrade.Status.ImageID = imageName
	return p
}

func (p *upgradeBuilder) NodesPreparedCondition(status corev1.ConditionStatus, reason, message string) *upgradeBuilder {
	setNodesPreparedCondition(p.upgrade, status, reason, message)
	return p
}

func (p *upgradeBuilder) NodesUpgradedCondition(status corev1.ConditionStatus, reason, message string) *upgradeBuilder {
	setNodesUpgradedCondition(p.upgrade, status, reason, message)
	return p
}

func (p *upgradeBuilder) ChartUpgradeStatus(status corev1.ConditionStatus, reason, message string) *upgradeBuilder {
	setHelmChartUpgradeStatus(p.upgrade, status, reason, message)
	return p
}

func (p *upgradeBuilder) InitStatus() *upgradeBuilder {
	initStatus(p.upgrade)
	return p
}

func (p *upgradeBuilder) Build() *harvesterv1.Upgrade {
	return p.upgrade
}

type versionBuilder struct {
	version *harvesterv1.Version
}

func newVersionBuilder(name string) *versionBuilder {
	return &versionBuilder{
		version: &harvesterv1.Version{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: upgradeNamespace,
			},
		},
	}
}

func (v *versionBuilder) Build() *harvesterv1.Version {
	return v.version
}

type planBuilder struct {
	plan *upgradev1.Plan
}

func newPlanBuilder(name string) *planBuilder {
	return &planBuilder{
		plan: &upgradev1.Plan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: sucNamespace,
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
	node *corev1.Node
}

func newNodeBuilder(name string) *nodeBuilder {
	return &nodeBuilder{
		node: &corev1.Node{
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

func (n *nodeBuilder) Build() *corev1.Node {
	return n.node
}

func upgradeReference(upgrade *harvesterv1.Upgrade) metav1.OwnerReference {
	return metav1.OwnerReference{
		Name:       upgrade.Name,
		Kind:       upgrade.Kind,
		UID:        upgrade.UID,
		APIVersion: upgrade.APIVersion,
	}
}
