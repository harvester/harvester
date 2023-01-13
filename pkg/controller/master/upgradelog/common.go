package upgradelog

import (
	"errors"
	"fmt"

	loggingv1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/filter"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/output"
	"github.com/banzaicloud/operator-tools/pkg/volume"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

const (
	defaultJobBackoffLimit    int32 = 5
	defaultDeploymentReplicas int32 = 1
	logPackagingScript              = `
#!/usr/bin/env sh
set -e

echo "start to package upgrade logs"

archive="$ARCHIVE_NAME.tar.gz"
tmpdir=$(mktemp -d)
mkdir $tmpdir/logs

cd /archive/logs

for f in *.log
do
    cat $f | awk '{$1=$2=""; print $0}' | jq -r .message > $tmpdir/logs/$f
done

tar -zcvf /archive/$archive -C $tmpdir .
ls -l /archive/$archive
echo "done"
`
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
					harvesterUpgradeLogLabel:          upgradeLog.Name,
					harvesterUpgradeLogComponentLabel: ShipperComponent,
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
					harvesterUpgradeLogLabel:          upgradeLog.Name,
					harvesterUpgradeLogComponentLabel: AggregatorComponent,
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

func prepareLogDownloader(upgradeLog *harvesterv1.UpgradeLog, imageVersion string) *appsv1.Deployment {
	replicas := defaultDeploymentReplicas
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				harvesterUpgradeLogLabel:          upgradeLog.Name,
				harvesterUpgradeLogComponentLabel: DownloaderComponent,
			},
			Name:      fmt.Sprintf("%s-log-downloader", upgradeLog.Name),
			Namespace: upgradeLogNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					harvesterUpgradeLogLabel:          upgradeLog.Name,
					harvesterUpgradeLogComponentLabel: DownloaderComponent,
					"app":                             "downloader",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						harvesterUpgradeLogLabel:          upgradeLog.Name,
						harvesterUpgradeLogComponentLabel: DownloaderComponent,
						"app":                             "downloader",
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
												harvesterUpgradeLogLabel:          upgradeLog.Name,
												harvesterUpgradeLogComponentLabel: AggregatorComponent,
											},
										},
										Namespaces: []string{
											upgradeLogNamespace,
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
							Image: fmt.Sprintf("%s:%s", downloaderImageRepository, imageVersion),
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
									ClaimName: fmt.Sprintf("%s-log-archive", upgradeLog.Name),
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

func (p *upgradeLogBuilder) Archive(name string, size int64, time string, ready bool) *upgradeLogBuilder {
	SetUpgradeLogArchive(p.upgradeLog, name, size, time, ready)
	return p
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

func (p *upgradeLogBuilder) UpgradeEndedCondition(status corev1.ConditionStatus, reason, message string) *upgradeLogBuilder {
	setUpgradeEndedCondition(p.upgradeLog, status, reason, message)
	return p
}

func (p *upgradeLogBuilder) DownloadReadyCondition(status corev1.ConditionStatus, reason, message string) *upgradeLogBuilder {
	setDownloadReadyCondition(p.upgradeLog, status, reason, message)
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
				Namespace: upgradeLogNamespace,
			},
		},
	}
}

func (p *daemonSetBuilder) WithLabel(key, value string) *daemonSetBuilder {
	if p.daemonSet.Labels == nil {
		p.daemonSet.Labels = make(map[string]string)
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
				Namespace: upgradeLogNamespace,
			},
		},
	}
}

func (p *jobBuilder) WithLabel(key, value string) *jobBuilder {
	if p.job.Labels == nil {
		p.job.Labels = make(map[string]string)
	}
	p.job.Labels[key] = value
	return p
}

func (p *jobBuilder) WithAnnotation(key, value string) *jobBuilder {
	if p.job.Annotations == nil {
		p.job.Annotations = make(map[string]string)
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

type statefulSetBuilder struct {
	statefulSet *appsv1.StatefulSet
}

func newStatefulSetBuilder(name string) *statefulSetBuilder {
	return &statefulSetBuilder{
		statefulSet: &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: upgradeLogNamespace,
			},
		},
	}
}

func (p *statefulSetBuilder) WithLabel(key, value string) *statefulSetBuilder {
	if p.statefulSet.Labels == nil {
		p.statefulSet.Labels = make(map[string]string)
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

func PrepareLogPackager(upgradeLog *harvesterv1.UpgradeLog, imageVersion, archiveName, component string) *batchv1.Job {
	backoffLimit := defaultJobBackoffLimit
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				archiveNameAnnotation: archiveName,
			},
			Labels: map[string]string{
				harvesterUpgradeLogLabel:          upgradeLog.Name,
				harvesterUpgradeLogComponentLabel: PackagerComponent,
			},
			GenerateName: fmt.Sprintf("%s-log-packager-", upgradeLog.Name),
			Namespace:    upgradeLogNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       upgradeLog.Name,
					Kind:       upgradeLog.Kind,
					UID:        upgradeLog.UID,
					APIVersion: upgradeLog.APIVersion,
				},
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						harvesterUpgradeLogLabel:          upgradeLog.Name,
						harvesterUpgradeLogComponentLabel: PackagerComponent,
					},
				},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						PodAffinity: &corev1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											harvesterUpgradeLogLabel:          upgradeLog.Name,
											harvesterUpgradeLogComponentLabel: component,
										},
									},
									Namespaces: []string{
										upgradeLogNamespace,
									},
									TopologyKey: "kubernetes.io/hostname",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "log-packager",
							Image: fmt.Sprintf("%s:%s", packagerImageRepository, imageVersion),
							Command: []string{
								"sh", "-c", logPackagingScript,
							},
							Env: []corev1.EnvVar{
								{
									Name:  "ARCHIVE_NAME",
									Value: archiveName,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "log-archive",
									MountPath: "/archive",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "log-archive",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("%s-log-archive", upgradeLog.Name),
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyOnFailure,
				},
			},
			BackoffLimit: &backoffLimit,
		},
	}
}

func SetUpgradeLogArchive(upgradeLog *harvesterv1.UpgradeLog, archiveName string, archiveSize int64, generatedTime string, ready bool) {
	if upgradeLog == nil {
		return
	}
	if upgradeLog.Status.Archives == nil {
		upgradeLog.Status.Archives = make(map[string]harvesterv1.Archive)
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

func GetDownloaderPodIP(podCache ctlcorev1.PodCache, upgradeLog *harvesterv1.UpgradeLog) (string, error) {
	sets := labels.Set{
		"app":                    "downloader",
		harvesterUpgradeLogLabel: upgradeLog.Name,
	}

	pods, err := podCache.List(upgradeLog.Namespace, sets.AsSelector())
	if err != nil {
		return "", err

	}
	if len(pods) == 0 {
		return "", errors.New("downloader pod is not created")
	}
	if len(pods) > 1 {
		return "", errors.New("more than one downloader pods found")
	}
	if pods[0].Status.PodIP == "" {
		return "", errors.New("downloader pod IP is not allocated")
	}

	return pods[0].Status.PodIP, nil
}
