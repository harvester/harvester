package upgradelog

import (
	"context"
	"fmt"
	"reflect"

	loggingv1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	mgmtv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	ctlmgmtv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	ctlappsv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/apps/v1"
	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/name"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlloggingv1 "github.com/harvester/harvester/pkg/generated/controllers/logging.banzaicloud.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/helm"
)

const (
	// Annotations for the sub-components of logging infrastructure
	upgradeLogFluentBitAnnotation     = "harvesterhci.io/fluentBit"
	upgradeLogFluentdAnnotation       = "harvesterhci.io/fluentd"
	upgradeLogFluentBitReady          = "FluentBitReady"
	upgradeLogFluentdReady            = "FluentdReady"
	upgradeLogClusterFlowAnnotation   = "harvesterhci.io/clusterFlow"
	upgradeLogClusterOutputAnnotation = "harvesterhci.io/clusterOutput"
	upgradeLogClusterFlowReady        = "ClusterFlowReady"
	upgradeLogClusterOutputReady      = "ClusterOutputReady"

	// Annotation indicating the state of UpgradeLog
	upgradeLogStateAnnotation = "harvesterhci.io/upgradeLogState"
	upgradeLogStateCollecting = "Collecting"
	upgradeLogStateStopped    = "Stopped"

	appLabelName = "app.kubernetes.io/name"

	imageFluentbit      = "fluentbit"
	imageFluentd        = "fluentd"
	imageConfigReloader = "config_reloader"
)

var (
	logArchiveMatchingLabels = labels.Set{
		appLabelName:                  "fluentd",
		util.LabelUpgradeLogComponent: util.UpgradeLogAggregatorComponent,
	}

	loggingImagesList = map[string][]string{
		imageFluentbit:      {"images", imageFluentbit},
		imageFluentd:        {"images", imageFluentd},
		imageConfigReloader: {"images", imageConfigReloader},
	}
)

type handler struct {
	ctx                 context.Context
	namespace           string
	addonCache          ctlharvesterv1.AddonCache
	clusterFlowClient   ctlloggingv1.ClusterFlowClient
	clusterOutputClient ctlloggingv1.ClusterOutputClient
	daemonSetClient     ctlappsv1.DaemonSetClient
	daemonSetCache      ctlappsv1.DaemonSetCache
	deploymentClient    ctlappsv1.DeploymentClient
	jobClient           ctlbatchv1.JobClient
	jobCache            ctlbatchv1.JobCache
	loggingClient       ctlloggingv1.LoggingClient
	fbagentClient       ctlloggingv1.FluentbitAgentClient
	managedChartClient  ctlmgmtv3.ManagedChartClient
	managedChartCache   ctlmgmtv3.ManagedChartCache
	pvcClient           ctlcorev1.PersistentVolumeClaimClient
	serviceClient       ctlcorev1.ServiceClient
	statefulSetClient   ctlappsv1.StatefulSetClient
	statefulSetCache    ctlappsv1.StatefulSetCache
	upgradeClient       ctlharvesterv1.UpgradeClient
	upgradeCache        ctlharvesterv1.UpgradeCache
	upgradeLogClient    ctlharvesterv1.UpgradeLogClient
	upgradeLogCache     ctlharvesterv1.UpgradeLogCache
	clientset           *kubernetes.Clientset

	imageGetter ImageGetterInterface // for test code to mock helm
}

type Values struct {
	Images map[string]Image `mapstructure:"images"`
}

type Image struct {
	Repository string `mapstructure:"repository"`
	Tag        string `mapstructure:"tag"`
}

func (h *handler) OnUpgradeLogChange(_ string, upgradeLog *harvesterv1.UpgradeLog) (*harvesterv1.UpgradeLog, error) {
	if upgradeLog == nil || upgradeLog.DeletionTimestamp != nil {
		return upgradeLog, nil
	}
	logrus.Debugf("Processing UpgradeLog %s/%s", upgradeLog.Namespace, upgradeLog.Name)

	// Initialize the UpgradeLog resource
	if harvesterv1.UpgradeLogReady.GetStatus(upgradeLog) == "" {
		logrus.Info("Initialize UpgradeLog")
		toUpdate := upgradeLog.DeepCopy()
		harvesterv1.UpgradeLogReady.CreateUnknownIfNotExists(toUpdate)
		return h.upgradeLogClient.Update(toUpdate)
	}

	// Try to bring up the logging operator by installing rancher-logging ManagedChart
	if harvesterv1.LoggingOperatorDeployed.GetStatus(upgradeLog) == "" {
		logrus.Info("Check if there are any existing logging-operator")

		toUpdate := upgradeLog.DeepCopy()
		harvesterv1.LoggingOperatorDeployed.CreateUnknownIfNotExists(toUpdate)

		// Detect rancher-logging Addon
		addon, err := h.addonCache.Get(util.CattleLoggingSystemNamespaceName, util.RancherLoggingName)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, err
			}
			logrus.Info("rancher-logging Addon is not installed")
		} else {
			if addon.Spec.Enabled {
				// mark the operator source
				setLoggingOperatorSource(toUpdate, util.RancherLoggingName)
				setOperatorDeployedCondition(toUpdate, corev1.ConditionTrue, "Skipped", "rancher-logging Addon is enabled")
				return h.upgradeLogClient.Update(toUpdate)
			}
			logrus.Info("rancher-logging Addon is not enabled")
		}

		logrus.Info("Deploy logging-operator via a new ManagedChart")
		if _, err := h.managedChartClient.Create(prepareOperator(upgradeLog)); err != nil && !apierrors.IsAlreadyExists(err) {
			return nil, err
		}
		// format: hvst-upgrade-l5875-upgradelog-operator
		setLoggingOperatorSource(toUpdate, name.SafeConcatName(upgradeLog.Name, util.UpgradeLogOperatorComponent))

		return h.upgradeLogClient.Update(toUpdate)
	}

	// Try to establish the logging infrastructure by creating a customized Logging resource
	if harvesterv1.LoggingOperatorDeployed.IsTrue(upgradeLog) && harvesterv1.InfraReady.GetStatus(upgradeLog) == "" {
		logrus.Info("Start to create the logging infrastructure and fluentbitagent for the upgrade procedure")

		toUpdate := upgradeLog.DeepCopy()

		// The creation of the Logging resource will indirectly bring up fluent-bit DaemonSet and fluentd StatefulSet
		// get related images from helm chart
		ns, name, err := getLoggingImageSourceHelmChart(upgradeLog)
		if err != nil {
			logrus.Infof("%s", err.Error())
			return upgradeLog, err
		}
		candidateImages, err := h.imageGetter.GetConsolidatedLoggingImageListFromHelmValues(h.clientset, ns, name)
		if err != nil {
			logrus.Infof("%s", err.Error())
			return upgradeLog, err
		}

		if _, err := h.loggingClient.Create(prepareLogging(upgradeLog, candidateImages)); err != nil && !apierrors.IsAlreadyExists(err) {
			return nil, err
		}
		if _, err := h.fbagentClient.Create(prepareFluentbitAgent(upgradeLog, candidateImages)); err != nil && !apierrors.IsAlreadyExists(err) {
			return nil, err
		}

		harvesterv1.InfraReady.CreateUnknownIfNotExists(toUpdate)

		return h.upgradeLogClient.Update(toUpdate)
	} else if harvesterv1.LoggingOperatorDeployed.IsTrue(upgradeLog) && harvesterv1.InfraReady.IsUnknown(upgradeLog) {
		logrus.Info("Check if the logging infrastructure is ready")

		toUpdate := upgradeLog.DeepCopy()

		// These two annotations denote the readiness of the indirect resources respectively
		fluentBitAnnotation, ok := upgradeLog.Annotations[upgradeLogFluentBitAnnotation]
		if !ok {
			return upgradeLog, nil
		}
		fluentdAnnotation, ok := upgradeLog.Annotations[upgradeLogFluentdAnnotation]
		if !ok {
			return upgradeLog, nil
		}

		// Stay in the same phase until both fluent-bit and fluentd are ready
		isInfraReady := (fluentBitAnnotation == upgradeLogFluentBitReady) && (fluentdAnnotation == upgradeLogFluentdReady)
		if !isInfraReady {
			return upgradeLog, nil
		}

		logrus.Info("Logging infrastructure is ready")
		setInfraReadyCondition(toUpdate, corev1.ConditionTrue, "", "")
		return h.upgradeLogClient.Update(toUpdate)
	}

	// Try to install the rules. The desired logs will start to be collected once the rules are active
	if harvesterv1.InfraReady.IsTrue(upgradeLog) && harvesterv1.UpgradeLogReady.IsUnknown(upgradeLog) {
		logrus.Info("Check if the log-collecting rules are installed")

		toUpdate := upgradeLog.DeepCopy()

		clusterFlowAnnotation := upgradeLog.Annotations[upgradeLogClusterFlowAnnotation]
		clusterOutputAnnotation := upgradeLog.Annotations[upgradeLogClusterOutputAnnotation]

		// Move to the next phase if both the ClusterFlow and ClusterOutput are active
		isLogReady := (clusterOutputAnnotation == upgradeLogClusterOutputReady) && (clusterFlowAnnotation == upgradeLogClusterFlowReady)
		if isLogReady {
			logrus.Info("Log-collecting rules exist and are activated")
			if toUpdate.Annotations == nil {
				toUpdate.Annotations = make(map[string]string, 1)
			}
			toUpdate.Annotations[upgradeLogStateAnnotation] = upgradeLogStateCollecting
			setUpgradeLogReadyCondition(toUpdate, corev1.ConditionTrue, "", "")
			return h.upgradeLogClient.Update(toUpdate)
		}

		logrus.Info("Start to create the ClusterFlow and ClusterOutput resources for collecting upgrade logs")
		if _, err := h.clusterOutputClient.Create(prepareClusterOutput(upgradeLog)); err != nil && !apierrors.IsAlreadyExists(err) {
			return nil, err
		}
		if _, err := h.clusterFlowClient.Create(prepareClusterFlow(upgradeLog)); err != nil && !apierrors.IsAlreadyExists(err) {
			return nil, err
		}
		return upgradeLog, nil
	}

	// Signal to proceed the original upgrade flow
	if harvesterv1.UpgradeLogReady.IsTrue(upgradeLog) && harvesterv1.UpgradeEnded.GetStatus(upgradeLog) == "" {
		logrus.Info("Logging infrastructure is ready, proceed the upgrade procedure")

		toUpdate := upgradeLog.DeepCopy()

		// handle corresponding upgrade resource
		upgradeName := upgradeLog.Spec.UpgradeName
		upgrade, err := h.upgradeCache.Get(util.HarvesterSystemNamespaceName, upgradeName)
		if err != nil {
			// if the corresponding upgrade resource is not found, the upgradelog should be torn down immediately
			if apierrors.IsNotFound(err) {
				setUpgradeEndedCondition(toUpdate, corev1.ConditionTrue, "", "")
				return h.upgradeLogClient.Update(toUpdate)
			}
			return nil, err
		}
		upgradeToUpdate := upgrade.DeepCopy()
		if upgradeToUpdate.Labels == nil {
			upgradeToUpdate.Labels = map[string]string{}
		}
		upgradeToUpdate.Labels[util.LabelUpgradeState] = util.UpgradeStateLoggingInfraPrepared
		harvesterv1.LogReady.SetStatus(upgradeToUpdate, string(corev1.ConditionTrue))
		harvesterv1.LogReady.Reason(upgradeToUpdate, "")
		harvesterv1.LogReady.Message(upgradeToUpdate, "")

		if _, err := h.upgradeClient.Update(upgradeToUpdate); err != nil {
			return upgradeLog, err
		}

		// handle upgradeLog resource
		harvesterv1.UpgradeEnded.CreateUnknownIfNotExists(toUpdate)

		return h.upgradeLogClient.Update(toUpdate)
	}

	// Spin up the log downloader to serve the log downloading requests
	if harvesterv1.UpgradeEnded.IsUnknown(upgradeLog) && harvesterv1.DownloadReady.GetStatus(upgradeLog) == "" {
		logrus.Info("Spin up downloader")

		// Get image version for log-downloader
		upgradeName := upgradeLog.Spec.UpgradeName
		upgrade, err := h.upgradeCache.Get(util.HarvesterSystemNamespaceName, upgradeName)
		if err != nil {
			return nil, err
		}
		imageVersion := upgrade.Status.PreviousVersion

		if _, err := h.deploymentClient.Create(prepareLogDownloader(upgradeLog, imageVersion)); err != nil && !apierrors.IsAlreadyExists(err) {
			return nil, err
		}
		if _, err := h.serviceClient.Create(prepareLogDownloaderSvc(upgradeLog)); err != nil && !apierrors.IsAlreadyExists(err) {
			return nil, err
		}

		toUpdate := upgradeLog.DeepCopy()
		harvesterv1.DownloadReady.CreateUnknownIfNotExists(toUpdate)

		return h.upgradeLogClient.Update(toUpdate)
	}

	// Tear down the loggin infrastructure but keep the log downloader and the archive volume
	if harvesterv1.UpgradeEnded.IsTrue(upgradeLog) {
		upgradeLogState, ok := upgradeLog.Annotations[upgradeLogStateAnnotation]
		if !ok {
			return upgradeLog, nil
		}
		if upgradeLogState == upgradeLogStateCollecting {
			logrus.Info("Stop collecting logs")
			if err := h.stopCollect(upgradeLog); err != nil {
				return upgradeLog, err
			}
			toUpdate := upgradeLog.DeepCopy()
			toUpdate.Annotations[upgradeLogStateAnnotation] = upgradeLogStateStopped
			return h.upgradeLogClient.Update(toUpdate)
		}
		return upgradeLog, nil
	}

	return upgradeLog, nil
}

func (h *handler) OnUpgradeLogRemove(_ string, upgradeLog *harvesterv1.UpgradeLog) (*harvesterv1.UpgradeLog, error) {
	if upgradeLog == nil {
		return nil, nil
	}
	logrus.Infof("Delete UpgradeLog %s/%s", upgradeLog.Namespace, upgradeLog.Name)
	return upgradeLog, h.cleanup(upgradeLog)
}

func (h *handler) OnClusterFlowChange(_ string, clusterFlow *loggingv1.ClusterFlow) (*loggingv1.ClusterFlow, error) {
	if clusterFlow == nil || clusterFlow.DeletionTimestamp != nil || clusterFlow.Labels == nil || clusterFlow.Namespace != util.HarvesterSystemNamespaceName {
		return clusterFlow, nil
	}
	logrus.Debugf("Processing ClusterFlow %s/%s", clusterFlow.Namespace, clusterFlow.Name)

	upgradeLogName, ok := clusterFlow.Labels[util.LabelUpgradeLog]
	if !ok {
		return clusterFlow, nil
	}
	upgradeLog, err := h.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, upgradeLogName)
	if err != nil {
		return clusterFlow, err
	}
	logrus.Debugf("Found relevant UpgradeLog %s/%s", upgradeLog.Namespace, upgradeLog.Name)

	toUpdate := upgradeLog.DeepCopy()

	if clusterFlow.Status.Active == nil {
		return clusterFlow, nil
	} else if *clusterFlow.Status.Active {
		logrus.Debugf("ClusterFlow %s/%s is now active", clusterFlow.Namespace, clusterFlow.Name)
		if toUpdate.Annotations == nil {
			toUpdate.Annotations = make(map[string]string, 1)
		}
		toUpdate.Annotations[upgradeLogClusterFlowAnnotation] = upgradeLogClusterFlowReady
	}

	if !reflect.DeepEqual(upgradeLog, toUpdate) {
		if _, err := h.upgradeLogClient.Update(toUpdate); err != nil {
			return clusterFlow, err
		}
	}

	return clusterFlow, nil
}

func (h *handler) OnClusterOutputChange(_ string, clusterOutput *loggingv1.ClusterOutput) (*loggingv1.ClusterOutput, error) {
	if clusterOutput == nil || clusterOutput.DeletionTimestamp != nil || clusterOutput.Labels == nil || clusterOutput.Namespace != util.HarvesterSystemNamespaceName {
		return clusterOutput, nil
	}
	logrus.Debugf("Processing ClusterOutput %s/%s", clusterOutput.Namespace, clusterOutput.Name)

	upgradeLogName, ok := clusterOutput.Labels[util.LabelUpgradeLog]
	if !ok {
		return clusterOutput, nil
	}
	upgradeLog, err := h.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, upgradeLogName)
	if err != nil {
		return clusterOutput, err
	}
	logrus.Debugf("Found relevant UpgradeLog %s/%s", upgradeLog.Namespace, upgradeLog.Name)

	toUpdate := upgradeLog.DeepCopy()

	if clusterOutput.Status.Active == nil {
		return clusterOutput, nil
	} else if *clusterOutput.Status.Active {
		logrus.Debugf("ClusterOutput %s/%s is now active", clusterOutput.Namespace, clusterOutput.Name)
		if toUpdate.Annotations == nil {
			toUpdate.Annotations = make(map[string]string, 1)
		}
		toUpdate.Annotations[upgradeLogClusterOutputAnnotation] = upgradeLogClusterOutputReady
	}

	if !reflect.DeepEqual(upgradeLog, toUpdate) {
		if _, err := h.upgradeLogClient.Update(toUpdate); err != nil {
			return clusterOutput, err
		}
	}

	return clusterOutput, nil
}

func (h *handler) OnDaemonSetChange(_ string, daemonSet *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {
	if daemonSet == nil || daemonSet.DeletionTimestamp != nil || daemonSet.Labels == nil || daemonSet.Namespace != util.HarvesterSystemNamespaceName {
		return daemonSet, nil
	}
	logrus.Debugf("Processing DaemonSet %s/%s", daemonSet.Namespace, daemonSet.Name)

	upgradeLogName, ok := daemonSet.Labels[util.LabelUpgradeLog]
	if !ok {
		return daemonSet, nil
	}
	upgradeLog, err := h.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, upgradeLogName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return daemonSet, nil
		}
		return nil, err
	}
	logrus.Debugf("Found relevant UpgradeLog %s/%s", upgradeLog.Namespace, upgradeLog.Name)

	fluentBitAnnotation, ok := upgradeLog.Annotations[upgradeLogFluentBitAnnotation]
	if ok && (fluentBitAnnotation == upgradeLogFluentBitReady) {
		logrus.Debug("Skipped syncing because fluentbit was marked as ready")
		return daemonSet, nil
	}

	toUpdate := upgradeLog.DeepCopy()

	if daemonSet.Status.DesiredNumberScheduled > 0 && daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled {
		if toUpdate.Annotations == nil {
			toUpdate.Annotations = make(map[string]string, 1)
		}
		toUpdate.Annotations[upgradeLogFluentBitAnnotation] = upgradeLogFluentBitReady
	}

	if !reflect.DeepEqual(upgradeLog, toUpdate) {
		if _, err := h.upgradeLogClient.Update(toUpdate); err != nil {
			return daemonSet, err
		}
	}

	return daemonSet, nil
}

func (h *handler) OnDeploymentChange(_ string, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	if deployment == nil || deployment.DeletionTimestamp != nil || deployment.Labels == nil || deployment.Namespace != util.HarvesterSystemNamespaceName {
		return deployment, nil
	}
	logrus.Debugf("Processing Deployment %s/%s", deployment.Namespace, deployment.Name)

	upgradeLogName, ok := deployment.Labels[util.LabelUpgradeLog]
	if !ok {
		return deployment, nil
	}
	upgradeLog, err := h.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, upgradeLogName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return deployment, nil
		}
		return nil, err
	}
	logrus.Debugf("Found relevant UpgradeLog %s/%s", upgradeLog.Namespace, upgradeLog.Name)

	toUpdate := upgradeLog.DeepCopy()
	if *deployment.Spec.Replicas > 0 && deployment.Status.ReadyReplicas == *deployment.Spec.Replicas {
		setDownloadReadyCondition(toUpdate, corev1.ConditionTrue, "", "")
	} else {
		setDownloadReadyCondition(toUpdate, corev1.ConditionFalse, "", "")
	}

	if !reflect.DeepEqual(upgradeLog, toUpdate) {
		if _, err := h.upgradeLogClient.Update(toUpdate); err != nil {
			return deployment, err
		}
	}

	return deployment, nil
}

func (h *handler) OnJobChange(_ string, job *batchv1.Job) (*batchv1.Job, error) {
	if job == nil || job.DeletionTimestamp != nil || job.Labels == nil {
		return job, nil
	}
	logrus.Debugf("Processing Job %s/%s", job.Namespace, job.Name)

	upgradeLogName, ok := job.Labels[util.LabelUpgradeLog]
	if !ok {
		return job, nil
	}
	upgradeLog, err := h.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, upgradeLogName)
	if err != nil {
		return job, err
	}
	logrus.Debugf("Found relevant UpgradeLog %s/%s", upgradeLog.Namespace, upgradeLog.Name)

	toUpdate := upgradeLog.DeepCopy()
	if job.Status.Succeeded > 0 {
		archiveName, ok := job.Annotations[util.AnnotationArchiveName]
		if !ok {
			return job, nil
		}
		if err := setUpgradeLogArchiveReady(toUpdate, archiveName, true, ""); err != nil {
			return job, err
		}
	}

	if job.Status.Failed > 0 {
		archiveName, ok := job.Annotations[util.AnnotationArchiveName]
		if !ok {
			return job, nil
		}
		if err := setUpgradeLogArchiveReady(toUpdate, archiveName, false, "failed to package the logs"); err != nil {
			return job, err
		}
	}

	if !reflect.DeepEqual(upgradeLog, toUpdate) {
		if _, err := h.upgradeLogClient.Update(toUpdate); err != nil {
			return job, err
		}
	}

	return job, nil
}

func (h *handler) OnManagedChartChange(_ string, managedChart *mgmtv3.ManagedChart) (*mgmtv3.ManagedChart, error) {
	if managedChart == nil || managedChart.DeletionTimestamp != nil || managedChart.Labels == nil || managedChart.Namespace != "fleet-local" {
		return managedChart, nil
	}
	logrus.Debugf("Processing ManagedChart %s/%s", managedChart.Namespace, managedChart.Name)

	upgradeLogName, ok := managedChart.Labels[util.LabelUpgradeLog]
	if !ok {
		return managedChart, nil
	}
	upgradeLog, err := h.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, upgradeLogName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return managedChart, nil
		}
		return nil, err
	}
	logrus.Debugf("Found relevant UpgradeLog %s/%s", upgradeLog.Namespace, upgradeLog.Name)

	toUpdate := upgradeLog.DeepCopy()
	if managedChart.Status.Summary.DesiredReady > 0 && managedChart.Status.Summary.DesiredReady == managedChart.Status.Summary.Ready {
		setLoggingOperatorSource(toUpdate, name.SafeConcatName(upgradeLog.Name, util.UpgradeLogOperatorComponent))
		setOperatorDeployedCondition(toUpdate, corev1.ConditionTrue, "", "")
		if _, err := h.upgradeLogClient.Update(toUpdate); err != nil {
			return managedChart, err
		}
	}

	return managedChart, nil
}

func (h *handler) OnStatefulSetChange(_ string, statefulSet *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	if statefulSet == nil || statefulSet.DeletionTimestamp != nil || statefulSet.Labels == nil || statefulSet.Namespace != util.HarvesterSystemNamespaceName {
		return statefulSet, nil
	}
	logrus.Debugf("Processing StatefulSet %s/%s", statefulSet.Namespace, statefulSet.Name)

	upgradeLogName, ok := statefulSet.Labels[util.LabelUpgradeLog]
	if !ok {
		return statefulSet, nil
	}
	upgradeLog, err := h.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, upgradeLogName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return statefulSet, nil
		}
		return nil, err
	}
	logrus.Debugf("Found relevant UpgradeLog %s/%s", upgradeLog.Namespace, upgradeLog.Name)

	fluentdAnnotation, ok := upgradeLog.Annotations[upgradeLogFluentdAnnotation]
	if ok && (fluentdAnnotation == upgradeLogFluentdReady) {
		logrus.Debug("Skipped syncing because fluentd was marked as ready")
		return statefulSet, nil
	}

	toUpdate := upgradeLog.DeepCopy()

	if *statefulSet.Spec.Replicas > 0 && statefulSet.Status.ReadyReplicas == *statefulSet.Spec.Replicas {
		if toUpdate.Annotations == nil {
			toUpdate.Annotations = make(map[string]string, 1)
		}
		toUpdate.Annotations[upgradeLogFluentdAnnotation] = upgradeLogFluentdReady
	}

	if !reflect.DeepEqual(upgradeLog, toUpdate) {
		if _, err := h.upgradeLogClient.Update(toUpdate); err != nil {
			return statefulSet, err
		}
	}

	return statefulSet, err
}

func (h *handler) OnPvcChange(_ string, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	if pvc == nil || pvc.DeletionTimestamp != nil || pvc.Namespace != util.HarvesterSystemNamespaceName {
		return pvc, nil
	}

	// We only care about the log-archive PVC created by the fluentd StatefulSet
	pvcLabels := labels.Set(pvc.Labels)
	if !logArchiveMatchingLabels.AsSelector().Matches(pvcLabels) {
		return pvc, nil
	}
	upgradeLogName, ok := pvc.Labels[util.LabelUpgradeLog]
	if !ok {
		return pvc, nil
	}

	upgradeLog, err := h.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, upgradeLogName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return pvc, err
		}
		logrus.WithFields(logrus.Fields{
			"namespace": pvc.Namespace,
			"name":      pvc.Name,
			"kind":      pvc.Kind,
		}).Warn("upgradelog not found, skip it")
		return pvc, nil
	}

	// Add the PVC's name to the UpgradeLog annotation for tracking.
	// This is useful for the log downloader to know which PVC to download logs from.
	// For the PVC created by the fluentd StatefulSet, its name will look like:
	// hvst-upgrade-bczl4-upgradelog-infra-log-archive-hvst-upgrade-bczl4-upgradelog-infra-fluentd-0
	upgradeLogToUpdate := upgradeLog.DeepCopy()
	if upgradeLogToUpdate.Annotations == nil {
		upgradeLogToUpdate.Annotations = make(map[string]string, 1)
	}
	upgradeLogToUpdate.Annotations[util.AnnotationUpgradeLogLogArchiveAltName] = pvc.Name
	if !reflect.DeepEqual(upgradeLog, upgradeLogToUpdate) {
		logrus.WithFields(logrus.Fields{
			"namespace": upgradeLogToUpdate.Namespace,
			"name":      upgradeLogToUpdate.Name,
			"kind":      upgradeLogToUpdate.Kind,
		}).Info("updating log-archive pvc name annotation")
		if _, err := h.upgradeLogClient.Update(upgradeLogToUpdate); err != nil {
			return pvc, err
		}
	}

	newOwnerRef := metav1.OwnerReference{
		Name:       upgradeLog.Name,
		APIVersion: upgradeLog.APIVersion,
		UID:        upgradeLog.UID,
		Kind:       upgradeLog.Kind,
	}

	// Check if the OwnerReference already exists
	for _, ownerRef := range pvc.OwnerReferences {
		if ownerRef.UID == newOwnerRef.UID {
			return pvc, nil
		}
	}

	// Add UpgradeLog as an owner of the log-archive PVC because we want it to
	// live longer than its original owner, i.e., Logging, so pods like the
	// downloader and packager can still access the log-archive volume.
	toUpdate := pvc.DeepCopy()
	toUpdate.OwnerReferences = append(toUpdate.OwnerReferences, newOwnerRef)

	if !reflect.DeepEqual(pvc, toUpdate) {
		logrus.WithFields(logrus.Fields{
			"namespace": pvc.Namespace,
			"name":      pvc.Name,
			"kind":      pvc.Kind,
		}).Info("updating ownerReference")
		return h.pvcClient.Update(toUpdate)
	}

	return pvc, nil
}

func (h *handler) OnUpgradeChange(_ string, upgrade *harvesterv1.Upgrade) (*harvesterv1.Upgrade, error) {
	if upgrade == nil || upgrade.DeletionTimestamp != nil || upgrade.Labels == nil || upgrade.Namespace != util.HarvesterSystemNamespaceName {
		return upgrade, nil
	}
	logrus.Debugf("Processing Upgrade %s/%s", upgrade.Namespace, upgrade.Name)

	if !upgrade.Spec.LogEnabled {
		return upgrade, nil
	}

	if upgrade.Labels[util.LabelUpgradeReadMessage] == "true" {
		upgradeLogName := upgrade.Status.UpgradeLog
		if upgradeLogName == "" {
			logrus.Debug("No related UpgradeLog resource found, skip purging")
			return upgrade, nil
		}
		upgradeLog, err := h.upgradeLogCache.Get(util.HarvesterSystemNamespaceName, upgradeLogName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrus.Debugf("The corresponding UpgradeLog %s/%s is not found, skip purging", util.HarvesterSystemNamespaceName, upgradeLogName)
				return upgrade, nil
			}
			return nil, err
		}

		logrus.Infof("Purging UpgradeLog %s/%s and its sub-components", upgradeLog.Namespace, upgradeLog.Name)
		if err := h.upgradeLogClient.Delete(upgradeLog.Namespace, upgradeLog.Name, &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return upgrade, err
		}
		toUpdate := upgrade.DeepCopy()
		toUpdate.Status.UpgradeLog = ""
		return h.upgradeClient.Update(toUpdate)
	}

	return upgrade, nil
}

func (h *handler) stopCollect(upgradeLog *harvesterv1.UpgradeLog) error {
	logrus.Info("Tearing down the logging infrastructure for upgrade procedure")

	var err error
	err = h.clusterFlowClient.Delete(util.HarvesterSystemNamespaceName, name.SafeConcatName(upgradeLog.Name, util.UpgradeLogFlowComponent), &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete clusterflow %s/%s error %w", util.HarvesterSystemNamespaceName, name.SafeConcatName(upgradeLog.Name, util.UpgradeLogFlowComponent), err)
	}
	err = h.clusterOutputClient.Delete(util.HarvesterSystemNamespaceName, name.SafeConcatName(upgradeLog.Name, util.UpgradeLogOutputComponent), &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete clusteroutput %s/%s error %w", util.HarvesterSystemNamespaceName, name.SafeConcatName(upgradeLog.Name, util.UpgradeLogOutputComponent), err)
	}
	err = h.loggingClient.Delete(name.SafeConcatName(upgradeLog.Name, util.UpgradeLogInfraComponent), &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete logging client %s error %w", name.SafeConcatName(upgradeLog.Name, util.UpgradeLogInfraComponent), err)
	}
	err = h.fbagentClient.Delete(name.SafeConcatName(upgradeLog.Name, util.UpgradeLogFluentbitAgentComponent), &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete fluentbitagent %s error %w", name.SafeConcatName(upgradeLog.Name, util.UpgradeLogFluentbitAgentComponent), err)
	}
	err = h.managedChartClient.Delete(util.FleetLocalNamespaceName, name.SafeConcatName(upgradeLog.Name, util.UpgradeLogOperatorComponent), &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete logging managedchart %s/%s error %w", util.FleetLocalNamespaceName, name.SafeConcatName(upgradeLog.Name, util.UpgradeLogOperatorComponent), err)
	}

	return nil
}

func (h *handler) cleanup(upgradeLog *harvesterv1.UpgradeLog) error {
	// Cleanup the relationship from its corresponding Upgrade resource as more as possible, does not return on single error quickly
	upgradeName := upgradeLog.Spec.UpgradeName
	var err1, err2, err3 error
	err := fmt.Errorf("upgradeLog %s/%s upgrade %s cleanup errors", upgradeLog.Namespace, upgradeLog.Name, upgradeName)

	// when the upgrade object is deleted, it's DeletionTimestamp is set, and all ownering resources' DeletionTimestamp are also set
	// but there is no promise that upgrade and other object disappear one by another
	// the below function has high chance to return IsNotFound error
	upgrade, err1 := h.upgradeCache.Get(util.HarvesterSystemNamespaceName, upgradeName)
	for i := 0; i < 1; i++ {
		if err1 != nil {
			// can't return directly, need to clean other related resources
			if apierrors.IsNotFound(err1) {
				err1 = nil
				break
			}
			err = fmt.Errorf("%w failed to get upgrade %w", err, err1)
			break
		}
		// already updated
		if upgrade.Status.UpgradeLog == "" {
			break
		}
		upgradeToUpdate := upgrade.DeepCopy()
		upgradeToUpdate.Status.UpgradeLog = ""
		_, err2 = h.upgradeClient.Update(upgradeToUpdate)
		if err2 != nil {
			if apierrors.IsNotFound(err2) {
				err2 = nil
				break
			}
			err = fmt.Errorf("%w failed to update upgrade %w", err, err2)
			break
		}
	}

	// Remove all if the UpgradeLog resource is deleted before normal tear down
	logrus.Info("Removing all other related resources")
	err3 = h.stopCollect(upgradeLog)
	if err3 != nil {
		err = fmt.Errorf("%w failed to clean other resources %w", err, err3)
	}

	if err1 != nil || err2 != nil || err3 != nil {
		// retry
		logrus.Infof("%s, retry", err.Error())
		return err
	}

	return nil
}

type ImageGetter struct{}

func (i *ImageGetter) GetConsolidatedLoggingImageListFromHelmValues(c *kubernetes.Clientset, namespace, name string) (map[string]settings.Image, error) {
	images := make(map[string]settings.Image, len(loggingImagesList))

	for img, key := range loggingImagesList {
		imgTag, err := helm.FetchImageFromHelmValues(c, namespace, name, key)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %v image from helm chart %v/%v values, error %w", img, namespace, name, err)
		}
		images[img] = imgTag
	}

	return images, nil
}

func NewImageGetter() *ImageGetter {
	return &ImageGetter{}
}
