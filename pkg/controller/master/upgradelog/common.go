package upgradelog

import (
	"fmt"

	loggingv1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/filter"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/output"
	"github.com/banzaicloud/operator-tools/pkg/volume"
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func preparePvc(upgradeLog *harvesterv1.UpgradeLog) *corev1.PersistentVolumeClaim {
	upgradeLogStorageClassName := harvesterUpgradeLogStorageClassName
	volumeMode := harvesterUpgradeLogVolumeMode

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				harvesterUpgradeLogLabel: upgradeLog.Name,
			},
			Name:      fmt.Sprintf("%s-log-archive", upgradeLog.Name),
			Namespace: upgradeLogNamespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					"storage": resource.MustParse("1Gi"),
				},
			},
			StorageClassName: &upgradeLogStorageClassName,
			VolumeMode:       &volumeMode,
		},
	}
}

func prepareLogging(upgradeLog *harvesterv1.UpgradeLog) *loggingv1.Logging {
	return &loggingv1.Logging{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				harvesterUpgradeLogLabel: upgradeLog.Name,
			},
			Name: fmt.Sprintf("%s-infra", upgradeLog.Name),
		},
		Spec: loggingv1.LoggingSpec{
			ControlNamespace:        upgradeLog.Namespace,
			FlowConfigCheckDisabled: true,
			FluentbitSpec: &loggingv1.FluentbitSpec{
				Labels: map[string]string{
					harvesterUpgradeLogLabel: upgradeLog.Name,
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      "node-role.kubernetes.io/master",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
			},
			FluentdSpec: &loggingv1.FluentdSpec{
				Labels: map[string]string{
					harvesterUpgradeLogLabel: upgradeLog.Name,
				},
				DisablePvc: true,
				ExtraVolumes: []loggingv1.ExtraVolume{
					{
						ContainerName: "fluentd",
						Path:          "/archive",
						VolumeName:    "log-archive",
						Volume: &volume.KubernetesVolume{
							PersistentVolumeClaim: &volume.PersistentVolumeClaim{
								PersistentVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("%s-log-archive", upgradeLog.Name),
									ReadOnly:  false,
								},
							},
						},
					},
				},
				FluentOutLogrotate: &loggingv1.FluentOutLogrotate{
					Age:     "10",
					Enabled: true,
					Path:    "/fluentd/log/out",
					Size:    "10485760",
				},
			},
		},
	}
}

func prepareClusterFlow(upgradeLog *harvesterv1.UpgradeLog) *loggingv1.ClusterFlow {
	tagNormaliser := filter.TagNormaliser{}
	dedot := filter.DedotFilterConfig{
		Separator: "-",
		Nested:    true,
	}

	return &loggingv1.ClusterFlow{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				harvesterUpgradeLogLabel: upgradeLog.Name,
			},
			Name:      fmt.Sprintf("%s-clusterflow", upgradeLog.Name),
			Namespace: upgradeLogNamespace,
		},
		Spec: loggingv1.ClusterFlowSpec{
			Filters: []loggingv1.Filter{
				{
					TagNormaliser: &tagNormaliser,
				},
				{
					Dedot: &dedot,
				},
			},
			Match: []loggingv1.ClusterMatch{
				{
					ClusterSelect: &loggingv1.ClusterSelect{
						Labels:     map[string]string{},
						Namespaces: []string{"kube-system"},
					},
				},
				{
					ClusterSelect: &loggingv1.ClusterSelect{
						Labels: map[string]string{
							"app.kubernetes.io/name": "harvester",
						},
						Namespaces: []string{"harvester-system"},
					},
				},
				{
					ClusterSelect: &loggingv1.ClusterSelect{
						Labels: map[string]string{
							"longhorn.io/component": "instance-manager",
						},
						Namespaces: []string{"longhorn-system"},
					},
				},
				{
					ClusterSelect: &loggingv1.ClusterSelect{
						Labels: map[string]string{
							"app": "longhorn-manager",
						},
						Namespaces: []string{"longhorn-system"},
					},
				},
				{
					ClusterSelect: &loggingv1.ClusterSelect{
						Labels: map[string]string{
							"app": "rancher",
						},
						Namespaces: []string{"cattle-system"},
					},
				},
				{
					ClusterSelect: &loggingv1.ClusterSelect{
						Labels: map[string]string{
							"upgrade.cattle.io/controller": "system-upgrade-controller",
						},
						Namespaces:     []string{"cattle-system"},
						ContainerNames: []string{"upgrade"},
					},
				},
				{
					ClusterSelect: &loggingv1.ClusterSelect{
						Labels: map[string]string{
							"harvesterhci.io/upgradeComponent": "manifest",
						},
						Namespaces: []string{"harvester-system"},
					},
				},
				{
					ClusterSelect: &loggingv1.ClusterSelect{
						Labels: map[string]string{
							"harvesterhci.io/upgradeComponent": "node",
						},
						Namespaces: []string{"harvester-system"},
					},
				},
			},
			GlobalOutputRefs: []string{fmt.Sprintf("%s-clusteroutput", upgradeLog.Name)},
		},
	}
}

func prepareClusterOutput(upgradeLog *harvesterv1.UpgradeLog) *loggingv1.ClusterOutput {
	return &loggingv1.ClusterOutput{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				harvesterUpgradeLogLabel: upgradeLog.Name,
			},
			Name:      fmt.Sprintf("%s-clusteroutput", upgradeLog.Name),
			Namespace: upgradeLogNamespace,
		},
		Spec: loggingv1.ClusterOutputSpec{
			OutputSpec: loggingv1.OutputSpec{
				FileOutput: &output.FileOutputConfig{
					Path:   "/archive/logs/${tag}",
					Append: true,
					Buffer: &output.Buffer{
						FlushMode:     "immediate",
						Timekey:       "1d",
						TimekeyWait:   "0m",
						TimekeyUseUtc: true,
					},
				},
			},
		},
	}
}

func setOperatorDeployedCondition(upgradeLog *harvesterv1.UpgradeLog, status corev1.ConditionStatus, reason, message string) {
	harvesterv1.OperatorDeployed.SetStatus(upgradeLog, string(status))
	harvesterv1.OperatorDeployed.Reason(upgradeLog, reason)
	harvesterv1.OperatorDeployed.Message(upgradeLog, message)
}

func setInfraScaffoldedCondition(upgradeLog *harvesterv1.UpgradeLog, status corev1.ConditionStatus, reason, message string) {
	harvesterv1.InfraScaffolded.SetStatus(upgradeLog, string(status))
	harvesterv1.InfraScaffolded.Reason(upgradeLog, reason)
	harvesterv1.InfraScaffolded.Message(upgradeLog, message)
}

func setUpgradeLogReadyCondition(upgradeLog *harvesterv1.UpgradeLog, status corev1.ConditionStatus, reason, message string) {
	harvesterv1.UpgradeLogReady.SetStatus(upgradeLog, string(status))
	harvesterv1.UpgradeLogReady.Reason(upgradeLog, reason)
	harvesterv1.UpgradeLogReady.Message(upgradeLog, message)
}

func setUpgradeEndedCondition(upgradeLog *harvesterv1.UpgradeLog, status corev1.ConditionStatus, reason, message string) {
	harvesterv1.UpgradeEnded.SetStatus(upgradeLog, string(status))
	harvesterv1.UpgradeEnded.Reason(upgradeLog, reason)
	harvesterv1.UpgradeEnded.Message(upgradeLog, message)
}

func setDownloadReadyCondition(upgradeLog *harvesterv1.UpgradeLog, status corev1.ConditionStatus, reason, message string) {
	harvesterv1.DownloadReady.SetStatus(upgradeLog, string(status))
	harvesterv1.DownloadReady.Reason(upgradeLog, reason)
	harvesterv1.DownloadReady.Message(upgradeLog, message)
}

type upgradeBuilder struct {
	upgrade *harvesterv1.Upgrade
}

func newUpgradeBuilder(name string) *upgradeBuilder {
	return &upgradeBuilder{
		upgrade: &harvesterv1.Upgrade{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: upgradeLogNamespace,
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

func (p *upgradeBuilder) Build() *harvesterv1.Upgrade {
	return p.upgrade
}

func (p *upgradeBuilder) LogReadyCondition(status corev1.ConditionStatus, reason, message string) *upgradeBuilder {
	harvesterv1.LogReady.SetStatus(p.upgrade, string(status))
	harvesterv1.LogReady.Reason(p.upgrade, reason)
	harvesterv1.LogReady.Message(p.upgrade, message)
	return p
}

type upgradeLogBuilder struct {
	upgradeLog *harvesterv1.UpgradeLog
}

func newUpgradeLogBuilder(name string) *upgradeLogBuilder {
	return &upgradeLogBuilder{
		upgradeLog: &harvesterv1.UpgradeLog{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: upgradeLogNamespace,
			},
		},
	}
}

func (p *upgradeLogBuilder) WithAnnotation(key, value string) *upgradeLogBuilder {
	if p.upgradeLog.Annotations == nil {
		p.upgradeLog.Annotations = make(map[string]string)
	}
	p.upgradeLog.Annotations[key] = value
	return p
}

func (p *upgradeLogBuilder) WithLabel(key, value string) *upgradeLogBuilder {
	if p.upgradeLog.Labels == nil {
		p.upgradeLog.Labels = make(map[string]string)
	}
	p.upgradeLog.Labels[key] = value
	return p
}

func (p *upgradeLogBuilder) Upgrade(value string) *upgradeLogBuilder {
	p.upgradeLog.Spec.Upgrade = value
	return p
}

func (p *upgradeLogBuilder) Build() *harvesterv1.UpgradeLog {
	return p.upgradeLog
}

func (p *upgradeLogBuilder) OperatorDeployedCondition(status corev1.ConditionStatus, reason, message string) *upgradeLogBuilder {
	setOperatorDeployedCondition(p.upgradeLog, status, reason, message)
	return p
}

func (p *upgradeLogBuilder) InfraScaffoldedCondition(status corev1.ConditionStatus, reason, message string) *upgradeLogBuilder {
	setInfraScaffoldedCondition(p.upgradeLog, status, reason, message)
	return p
}

func (p *upgradeLogBuilder) UpgradeLogReadyCondition(status corev1.ConditionStatus, reason, message string) *upgradeLogBuilder {
	setUpgradeLogReadyCondition(p.upgradeLog, status, reason, message)
	return p
}

type clusterFlowBuilder struct {
	clusterFlow *loggingv1.ClusterFlow
}

func newClusterFlowBuilder(name string) *clusterFlowBuilder {
	return &clusterFlowBuilder{
		clusterFlow: &loggingv1.ClusterFlow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: upgradeLogNamespace,
			},
		},
	}
}

func (p *clusterFlowBuilder) WithLabel(key, value string) *clusterFlowBuilder {
	if p.clusterFlow.Labels == nil {
		p.clusterFlow.Labels = make(map[string]string)
	}
	p.clusterFlow.Labels[key] = value
	return p
}

func (p *clusterFlowBuilder) Namespace(namespace string) *clusterFlowBuilder {
	p.clusterFlow.Namespace = namespace
	return p
}

func (p *clusterFlowBuilder) Build() *loggingv1.ClusterFlow {
	return p.clusterFlow
}

type clusterOutputBuilder struct {
	clusterOutput *loggingv1.ClusterOutput
}

func newClusterOutputBuilder(name string) *clusterOutputBuilder {
	return &clusterOutputBuilder{
		clusterOutput: &loggingv1.ClusterOutput{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: upgradeLogNamespace,
			},
		},
	}
}

func (p *clusterOutputBuilder) WithLabel(key, value string) *clusterOutputBuilder {
	if p.clusterOutput.Labels == nil {
		p.clusterOutput.Labels = make(map[string]string)
	}
	p.clusterOutput.Labels[key] = value
	return p
}

func (p *clusterOutputBuilder) Namespace(namespace string) *clusterOutputBuilder {
	p.clusterOutput.Namespace = namespace
	return p
}

func (p *clusterOutputBuilder) Build() *loggingv1.ClusterOutput {
	return p.clusterOutput
}

type loggingBuilder struct {
	logging *loggingv1.Logging
}

func newLoggingBuilder(name string) *loggingBuilder {
	return &loggingBuilder{
		logging: &loggingv1.Logging{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: upgradeLogNamespace,
			},
		},
	}
}

func (p *loggingBuilder) WithLabel(key, value string) *loggingBuilder {
	if p.logging.Labels == nil {
		p.logging.Labels = make(map[string]string)
	}
	p.logging.Labels[key] = value
	return p
}

func (p *loggingBuilder) Build() *loggingv1.Logging {
	return p.logging
}

type pvcBuilder struct {
	pvc *corev1.PersistentVolumeClaim
}

func newPvcBuilder(name string) *pvcBuilder {
	return &pvcBuilder{
		pvc: &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: upgradeLogNamespace,
			},
		},
	}
}

func (p *pvcBuilder) WithLabel(key, value string) *pvcBuilder {
	if p.pvc.Labels == nil {
		p.pvc.Labels = make(map[string]string)
	}
	p.pvc.Labels[key] = value
	return p
}

func (p *pvcBuilder) Build() *corev1.PersistentVolumeClaim {
	return p.pvc
}
