package upgradelog

import (
	"fmt"

	loggingv1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/filter"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/output"
	"github.com/banzaicloud/operator-tools/pkg/volume"
	"github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	mgmtv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/name"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const defaultDeploymentReplicas int32 = 1

func upgradeLogReference(upgradeLog *harvesterv1.UpgradeLog) metav1.OwnerReference {
	return metav1.OwnerReference{
		Name:       upgradeLog.Name,
		Kind:       upgradeLog.Kind,
		UID:        upgradeLog.UID,
		APIVersion: upgradeLog.APIVersion,
	}
}

func preparePvc(upgradeLog *harvesterv1.UpgradeLog) *corev1.PersistentVolumeClaim {
	upgradeLogStorageClassName := util.HarvesterLonghornStorageClassName
	volumeMode := corev1.PersistentVolumeFilesystem

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				util.LabelUpgradeLog:          upgradeLog.Name,
				util.LabelUpgradeLogComponent: util.UpgradeLogArchiveComponent,
			},
			Name:      name.SafeConcatName(upgradeLog.Name, util.UpgradeLogArchiveComponent),
			Namespace: util.HarvesterSystemNamespaceName,
			OwnerReferences: []metav1.OwnerReference{
				upgradeLogReference(upgradeLog),
			},
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

func prepareOperator(upgradeLog *harvesterv1.UpgradeLog) *mgmtv3.ManagedChart {
	operatorName := name.SafeConcatName(upgradeLog.Name, util.UpgradeLogOperatorComponent)
	return &mgmtv3.ManagedChart{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				util.LabelUpgradeLog:          upgradeLog.Name,
				util.LabelUpgradeLogComponent: util.UpgradeLogOperatorComponent,
			},
			Name:      operatorName,
			Namespace: util.FleetLocalNamespaceName,
		},
		Spec: mgmtv3.ManagedChartSpec{
			Chart:            util.RancherLoggingName,
			ReleaseName:      operatorName,
			DefaultNamespace: util.CattleLoggingSystemNamespaceName,
			RepoName:         "harvester-charts",
			Targets: []v1alpha1.BundleTarget{
				{
					ClusterName: "local",
					ClusterSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "provisioning.cattle.io/unmanaged-system-agent",
								Operator: metav1.LabelSelectorOpDoesNotExist,
							},
						},
					},
				},
			},
		},
	}
}

func prepareLogging(upgradeLog *harvesterv1.UpgradeLog) *loggingv1.Logging {
	return &loggingv1.Logging{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				util.LabelUpgradeLog:          upgradeLog.Name,
				util.LabelUpgradeLogComponent: util.UpgradeLogInfraComponent,
			},
			Name: name.SafeConcatName(upgradeLog.Name, util.UpgradeLogInfraComponent),
			OwnerReferences: []metav1.OwnerReference{
				upgradeLogReference(upgradeLog),
			},
		},
		Spec: loggingv1.LoggingSpec{
			ControlNamespace:        upgradeLog.Namespace,
			FlowConfigCheckDisabled: true,
			FluentbitSpec: &loggingv1.FluentbitSpec{
				Labels: map[string]string{
					util.LabelUpgradeLog:          upgradeLog.Name,
					util.LabelUpgradeLogComponent: util.UpgradeLogShipperComponent,
				},
				Image: loggingv1.ImageSpec{
					Repository: fluentBitImageRepo,
					Tag:        fluentBitImageTag,
				},
			},
			FluentdSpec: &loggingv1.FluentdSpec{
				Labels: map[string]string{
					util.LabelUpgradeLog:          upgradeLog.Name,
					util.LabelUpgradeLogComponent: util.UpgradeLogAggregatorComponent,
				},
				Image: loggingv1.ImageSpec{
					Repository: fluentdImageRepo,
					Tag:        fluentdImageTag,
				},
				ConfigReloaderImage: loggingv1.ImageSpec{
					Repository: configReloaderImageRepo,
					Tag:        configReloaderImageTag,
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
									ClaimName: name.SafeConcatName(upgradeLog.Name, util.UpgradeLogArchiveComponent),
									ReadOnly:  false,
								},
							},
						},
					},
				},
				Scaling: &loggingv1.FluentdScaling{
					Drain: loggingv1.FluentdDrainConfig{
						Enabled: true,
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
	return &loggingv1.ClusterFlow{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				util.LabelUpgradeLog:          upgradeLog.Name,
				util.LabelUpgradeLogComponent: util.UpgradeLogFlowComponent,
			},
			Name:      name.SafeConcatName(upgradeLog.Name, util.UpgradeLogFlowComponent),
			Namespace: util.HarvesterSystemNamespaceName,
			OwnerReferences: []metav1.OwnerReference{
				upgradeLogReference(upgradeLog),
			},
		},
		Spec: loggingv1.ClusterFlowSpec{
			Filters: []loggingv1.Filter{
				{
					TagNormaliser: &filter.TagNormaliser{},
				},
				{
					Dedot: &filter.DedotFilterConfig{
						Separator: "-",
						Nested:    true,
					},
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
						Labels:     map[string]string{},
						Namespaces: []string{"harvester-system"},
					},
				},
				{
					ClusterSelect: &loggingv1.ClusterSelect{
						Labels:     map[string]string{},
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
			},
			GlobalOutputRefs: []string{name.SafeConcatName(upgradeLog.Name, util.UpgradeLogOutputComponent)},
		},
	}
}

func prepareClusterOutput(upgradeLog *harvesterv1.UpgradeLog) *loggingv1.ClusterOutput {
	return &loggingv1.ClusterOutput{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				util.LabelUpgradeLog:          upgradeLog.Name,
				util.LabelUpgradeLogComponent: util.UpgradeLogOutputComponent,
			},
			Name:      name.SafeConcatName(upgradeLog.Name, util.UpgradeLogOutputComponent),
			Namespace: util.HarvesterSystemNamespaceName,
			OwnerReferences: []metav1.OwnerReference{
				upgradeLogReference(upgradeLog),
			},
		},
		Spec: loggingv1.ClusterOutputSpec{
			OutputSpec: loggingv1.OutputSpec{
				FileOutput: &output.FileOutputConfig{
					Path:   "/archive/logs/${tag}",
					Append: true,
					Buffer: &output.Buffer{
						FlushAtShutdown: true,
						FlushMode:       "immediate",
						Timekey:         "1d",
						TimekeyWait:     "0m",
						TimekeyUseUtc:   true,
					},
				},
			},
		},
	}
}

func prepareLogDownloader(upgradeLog *harvesterv1.UpgradeLog, imageVersion string) *appsv1.Deployment {
	replicas := defaultDeploymentReplicas
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				util.LabelUpgradeLog:          upgradeLog.Name,
				util.LabelUpgradeLogComponent: util.UpgradeLogDownloaderComponent,
			},
			Name:      name.SafeConcatName(upgradeLog.Name, util.UpgradeLogDownloaderComponent),
			Namespace: util.HarvesterSystemNamespaceName,
			OwnerReferences: []metav1.OwnerReference{
				upgradeLogReference(upgradeLog),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					util.LabelUpgradeLog:          upgradeLog.Name,
					util.LabelUpgradeLogComponent: util.UpgradeLogDownloaderComponent,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelUpgradeLog:          upgradeLog.Name,
						util.LabelUpgradeLogComponent: util.UpgradeLogDownloaderComponent,
					},
				},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						PodAffinity: &corev1.PodAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									Weight: 100,
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												util.LabelUpgradeLog:          upgradeLog.Name,
												util.LabelUpgradeLogComponent: util.UpgradeLogAggregatorComponent,
											},
										},
										Namespaces: []string{
											util.HarvesterSystemNamespaceName,
										},
										TopologyKey: "kubernetes.io/hostname",
									},
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "downloader",
							Image: fmt.Sprintf("%s:%s", util.HarvesterUpgradeImageRepository, imageVersion),
							Command: []string{
								"nginx", "-g", "daemon off;",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 80,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "log-archive",
									MountPath: "/srv/www/htdocs/",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "log-archive",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: name.SafeConcatName(upgradeLog.Name, util.UpgradeLogArchiveComponent),
									ReadOnly:  true,
								},
							},
						},
					},
				},
			},
		},
	}

}

func prepareLogDownloaderSvc(upgradeLog *harvesterv1.UpgradeLog) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				util.LabelUpgradeLog:          upgradeLog.Name,
				util.LabelUpgradeLogComponent: util.UpgradeLogDownloaderComponent,
			},
			Name:      upgradeLog.Name,
			Namespace: util.HarvesterSystemNamespaceName,
			OwnerReferences: []metav1.OwnerReference{
				upgradeLogReference(upgradeLog),
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				util.LabelUpgradeLog:          upgradeLog.Name,
				util.LabelUpgradeLogComponent: util.UpgradeLogDownloaderComponent,
			},
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Protocol:   corev1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.FromString("http"),
				},
			},
		},
	}
}

func setOperatorDeployedCondition(upgradeLog *harvesterv1.UpgradeLog, status corev1.ConditionStatus, reason, message string) {
	harvesterv1.LoggingOperatorDeployed.SetStatus(upgradeLog, string(status))
	harvesterv1.LoggingOperatorDeployed.Reason(upgradeLog, reason)
	harvesterv1.LoggingOperatorDeployed.Message(upgradeLog, message)
}

func setInfraReadyCondition(upgradeLog *harvesterv1.UpgradeLog, status corev1.ConditionStatus, reason, message string) {
	harvesterv1.InfraReady.SetStatus(upgradeLog, string(status))
	harvesterv1.InfraReady.Reason(upgradeLog, reason)
	harvesterv1.InfraReady.Message(upgradeLog, message)
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

func setUpgradeLogArchiveReady(upgradeLog *harvesterv1.UpgradeLog, archiveName string, ready bool) error {
	if archive, ok := upgradeLog.Status.Archives[archiveName]; ok {
		archive.Ready = ready
		upgradeLog.Status.Archives[archiveName] = archive
		return nil
	}
	return fmt.Errorf("archive %s of %s not found", archiveName, upgradeLog.Name)
}

type upgradeBuilder struct {
	upgrade *harvesterv1.Upgrade
}

func newUpgradeBuilder(name string) *upgradeBuilder {
	return &upgradeBuilder{
		upgrade: &harvesterv1.Upgrade{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: util.HarvesterSystemNamespaceName,
			},
		},
	}
}

func (p *upgradeBuilder) WithLabel(key, value string) *upgradeBuilder {
	if p.upgrade.Labels == nil {
		p.upgrade.Labels = make(map[string]string, 1)
	}
	p.upgrade.Labels[key] = value
	return p
}

func (p *upgradeBuilder) LogEnable(value bool) *upgradeBuilder {
	p.upgrade.Spec.LogEnabled = value
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

func (p *upgradeBuilder) UpgradeLogStatus(upgradeLogName string) *upgradeBuilder {
	p.upgrade.Status.UpgradeLog = upgradeLogName
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
				Namespace: util.HarvesterSystemNamespaceName,
			},
		},
	}
}

func (p *upgradeLogBuilder) WithAnnotation(key, value string) *upgradeLogBuilder {
	if p.upgradeLog.Annotations == nil {
		p.upgradeLog.Annotations = make(map[string]string, 1)
	}
	p.upgradeLog.Annotations[key] = value
	return p
}

func (p *upgradeLogBuilder) WithLabel(key, value string) *upgradeLogBuilder {
	if p.upgradeLog.Labels == nil {
		p.upgradeLog.Labels = make(map[string]string, 1)
	}
	p.upgradeLog.Labels[key] = value
	return p
}

func (p *upgradeLogBuilder) Upgrade(value string) *upgradeLogBuilder {
	p.upgradeLog.Spec.UpgradeName = value
	return p
}

func (p *upgradeLogBuilder) Build() *harvesterv1.UpgradeLog {
	return p.upgradeLog
}

func (p *upgradeLogBuilder) Archive(name string, size int64, time string, ready bool) *upgradeLogBuilder {
	SetUpgradeLogArchive(p.upgradeLog, name, size, time, ready)
	return p
}

func (p *upgradeLogBuilder) OperatorDeployedCondition(status corev1.ConditionStatus, reason, message string) *upgradeLogBuilder {
	setOperatorDeployedCondition(p.upgradeLog, status, reason, message)
	return p
}

func (p *upgradeLogBuilder) InfraReadyCondition(status corev1.ConditionStatus, reason, message string) *upgradeLogBuilder {
	setInfraReadyCondition(p.upgradeLog, status, reason, message)
	return p
}

func (p *upgradeLogBuilder) UpgradeLogReadyCondition(status corev1.ConditionStatus, reason, message string) *upgradeLogBuilder {
	setUpgradeLogReadyCondition(p.upgradeLog, status, reason, message)
	return p
}

func (p *upgradeLogBuilder) UpgradeEndedCondition(status corev1.ConditionStatus, reason, message string) *upgradeLogBuilder {
	setUpgradeEndedCondition(p.upgradeLog, status, reason, message)
	return p
}

func (p *upgradeLogBuilder) DownloadReadyCondition(status corev1.ConditionStatus, reason, message string) *upgradeLogBuilder {
	setDownloadReadyCondition(p.upgradeLog, status, reason, message)
	return p
}

type addonBuilder struct {
	addon *harvesterv1.Addon
}

func newAddonBuilder(name string) *addonBuilder {
	return &addonBuilder{
		addon: &harvesterv1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: util.CattleLoggingSystemNamespaceName,
			},
		},
	}
}

func (p *addonBuilder) Enable(value bool) *addonBuilder {
	p.addon.Spec.Enabled = true
	return p
}

func (p *addonBuilder) Build() *harvesterv1.Addon {
	return p.addon
}

type clusterFlowBuilder struct {
	clusterFlow *loggingv1.ClusterFlow
}

func newClusterFlowBuilder(name string) *clusterFlowBuilder {
	return &clusterFlowBuilder{
		clusterFlow: &loggingv1.ClusterFlow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: util.HarvesterSystemNamespaceName,
			},
		},
	}
}

func (p *clusterFlowBuilder) WithLabel(key, value string) *clusterFlowBuilder {
	if p.clusterFlow.Labels == nil {
		p.clusterFlow.Labels = make(map[string]string, 1)
	}
	p.clusterFlow.Labels[key] = value
	return p
}

func (p *clusterFlowBuilder) Namespace(namespace string) *clusterFlowBuilder {
	p.clusterFlow.Namespace = namespace
	return p
}

func (p *clusterFlowBuilder) Active() *clusterFlowBuilder {
	active := true
	p.clusterFlow.Status.Active = &active
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
				Namespace: util.HarvesterSystemNamespaceName,
			},
		},
	}
}

func (p *clusterOutputBuilder) WithLabel(key, value string) *clusterOutputBuilder {
	if p.clusterOutput.Labels == nil {
		p.clusterOutput.Labels = make(map[string]string, 1)
	}
	p.clusterOutput.Labels[key] = value
	return p
}

func (p *clusterOutputBuilder) Namespace(namespace string) *clusterOutputBuilder {
	p.clusterOutput.Namespace = namespace
	return p
}

func (p *clusterOutputBuilder) Active() *clusterOutputBuilder {
	active := true
	p.clusterOutput.Status.Active = &active
	return p
}

func (p *clusterOutputBuilder) Build() *loggingv1.ClusterOutput {
	return p.clusterOutput
}

type daemonSetBuilder struct {
	daemonSet *appsv1.DaemonSet
}

func newDaemonSetBuilder(name string) *daemonSetBuilder {
	return &daemonSetBuilder{
		daemonSet: &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: util.HarvesterSystemNamespaceName,
			},
		},
	}
}

func (p *daemonSetBuilder) WithLabel(key, value string) *daemonSetBuilder {
	if p.daemonSet.Labels == nil {
		p.daemonSet.Labels = make(map[string]string, 1)
	}
	p.daemonSet.Labels[key] = value
	return p
}

func (p *daemonSetBuilder) NotReady() *daemonSetBuilder {
	p.daemonSet.Status.DesiredNumberScheduled = 3
	p.daemonSet.Status.NumberReady = 1
	return p
}

func (p *daemonSetBuilder) Ready() *daemonSetBuilder {
	p.daemonSet.Status.DesiredNumberScheduled = 3
	p.daemonSet.Status.NumberReady = 3
	return p
}

func (p *daemonSetBuilder) Build() *appsv1.DaemonSet {
	return p.daemonSet
}

type jobBuilder struct {
	job *batchv1.Job
}

func newJobBuilder(name string) *jobBuilder {
	return &jobBuilder{
		job: &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: util.HarvesterSystemNamespaceName,
			},
		},
	}
}

func (p *jobBuilder) WithLabel(key, value string) *jobBuilder {
	if p.job.Labels == nil {
		p.job.Labels = make(map[string]string, 1)
	}
	p.job.Labels[key] = value
	return p
}

func (p *jobBuilder) WithAnnotation(key, value string) *jobBuilder {
	if p.job.Annotations == nil {
		p.job.Annotations = make(map[string]string, 1)
	}
	p.job.Annotations[key] = value
	return p
}

func (p *jobBuilder) Done() *jobBuilder {
	p.job.Status.Succeeded = 1
	return p
}

func (p *jobBuilder) Build() *batchv1.Job {
	return p.job
}

type loggingBuilder struct {
	logging *loggingv1.Logging
}

func newLoggingBuilder(name string) *loggingBuilder {
	return &loggingBuilder{
		logging: &loggingv1.Logging{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}
}

func (p *loggingBuilder) WithLabel(key, value string) *loggingBuilder {
	if p.logging.Labels == nil {
		p.logging.Labels = make(map[string]string, 1)
	}
	p.logging.Labels[key] = value
	return p
}

func (p *loggingBuilder) Build() *loggingv1.Logging {
	return p.logging
}

type managedChartBuilder struct {
	managedChart *mgmtv3.ManagedChart
}

func newManagedChartBuilder(name string) *managedChartBuilder {
	return &managedChartBuilder{
		managedChart: &mgmtv3.ManagedChart{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: util.FleetLocalNamespaceName,
			},
		},
	}
}

func (p *managedChartBuilder) WithLabel(key, value string) *managedChartBuilder {
	if p.managedChart.Labels == nil {
		p.managedChart.Labels = make(map[string]string, 1)
	}
	p.managedChart.Labels[key] = value
	return p
}

func (p *managedChartBuilder) Ready() *managedChartBuilder {
	p.managedChart.Status.Summary.DesiredReady = 1
	p.managedChart.Status.Summary.Ready = 1
	return p
}

func (p *managedChartBuilder) Build() *mgmtv3.ManagedChart {
	return p.managedChart
}

type pvcBuilder struct {
	pvc *corev1.PersistentVolumeClaim
}

func newPvcBuilder(name string) *pvcBuilder {
	return &pvcBuilder{
		pvc: &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: util.HarvesterSystemNamespaceName,
			},
		},
	}
}

func (p *pvcBuilder) WithLabel(key, value string) *pvcBuilder {
	if p.pvc.Labels == nil {
		p.pvc.Labels = make(map[string]string, 1)
	}
	p.pvc.Labels[key] = value
	return p
}

func (p *pvcBuilder) Build() *corev1.PersistentVolumeClaim {
	return p.pvc
}

type statefulSetBuilder struct {
	statefulSet *appsv1.StatefulSet
}

func newStatefulSetBuilder(name string) *statefulSetBuilder {
	return &statefulSetBuilder{
		statefulSet: &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: util.HarvesterSystemNamespaceName,
			},
		},
	}
}

func (p *statefulSetBuilder) WithLabel(key, value string) *statefulSetBuilder {
	if p.statefulSet.Labels == nil {
		p.statefulSet.Labels = make(map[string]string, 1)
	}
	p.statefulSet.Labels[key] = value
	return p
}

func (p *statefulSetBuilder) Replicas(replicas int32) *statefulSetBuilder {
	p.statefulSet.Spec.Replicas = &replicas
	return p
}

func (p *statefulSetBuilder) ReadyReplicas(replicas int32) *statefulSetBuilder {
	p.statefulSet.Status.ReadyReplicas = replicas
	return p
}

func (p *statefulSetBuilder) Build() *appsv1.StatefulSet {
	return p.statefulSet
}

func SetUpgradeLogArchive(upgradeLog *harvesterv1.UpgradeLog, archiveName string, archiveSize int64, generatedTime string, ready bool) {
	if upgradeLog == nil {
		return
	}
	if upgradeLog.Status.Archives == nil {
		upgradeLog.Status.Archives = make(map[string]harvesterv1.Archive, 1)
	}

	if current, ok := upgradeLog.Status.Archives[archiveName]; ok &&
		current.Size == archiveSize && current.GeneratedTime == generatedTime && current.Ready == ready {
		return
	}
	upgradeLog.Status.Archives[archiveName] = harvesterv1.Archive{
		Size:          archiveSize,
		GeneratedTime: generatedTime,
		Ready:         ready,
	}
}
