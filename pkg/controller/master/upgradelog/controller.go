package upgradelog

import (
	"context"
	"reflect"

	loggingv1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	mgmtv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	ctlmgmtv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	ctlappsv1 "github.com/rancher/wrangler/pkg/generated/controllers/apps/v1"
	ctlbatchv1 "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/name"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlloggingv1 "github.com/harvester/harvester/pkg/generated/controllers/logging.banzaicloud.io/v1beta1"
)

const (
	harvesterUpgradeLogLabel            = "harvesterhci.io/upgradeLog"
	harvesterUpgradeLogComponentLabel   = "harvesterhci.io/upgradeLogComponent"
	harvesterUpgradeLogStorageClassName = "harvester-longhorn"
	harvesterUpgradeLogVolumeMode       = corev1.PersistentVolumeFilesystem
	upgradeLogNamespace                 = "harvester-system"
	packagerImageRepository             = "rancher/harvester-upgrade"
	downloaderImageRepository           = "rancher/harvester-upgrade"
	addonNamespace                      = "cattle-logging-system"
	managedChartNamespace               = "fleet-local"
	operatorNamespace                   = "cattle-logging-system"
	rancherLoggingChart                 = "rancher-logging"
	rancherLoggingAddonName             = "rancher-logging"
	rancherLoggingManagedChartName      = "rancher-logging"

	upgradeNamespace                 = "harvester-system"
	upgradeReadMessageLabel          = "harvesterhci.io/read-message"
	upgradeStateLabel                = "harvesterhci.io/upgradeState"
	UpgradeStateLoggingInfraPrepared = "LoggingInfraPrepared"

	// Annotations for the sub-components of logging infrastructure
	upgradeLogFluentBitAnnotation     = "harvesterhci.io/fluentBit"
	upgradeLogFluentdAnnotation       = "harvesterhci.io/fluentd"
	upgradeLogFluentBitReady          = "FluentBitReady"
	upgradeLogFluentdReady            = "FluentdReady"
	upgradeLogClusterFlowAnnotation   = "harvesterhci.io/clusterFlow"
	upgradeLogClusterOutputAnnotation = "harvesterhci.io/clusterOutput"
	upgradeLogClusterFlowReady        = "ClusterFlowReady"
	upgradeLogClusterOutputReady      = "ClusterOutputReady"
	upgradeLogStateAnnotation         = "harvesterhci.io/upgradeLogState"
	upgradeLogStateCollecting         = "Collecting"
	upgradeLogStateStopped            = "Stopped"
	archiveNameAnnotation             = "harvesterhci.io/archiveName"

	// Logging infra images
	fluentBitImageRepo      = "rancher/mirrored-fluent-fluent-bit"
	fluentBitImageTag       = "1.9.5"
	fluentdImageRepo        = "rancher/mirrored-banzaicloud-fluentd"
	fluentdImageTag         = "v1.14.6-alpine-5"
	configReloaderImageRepo = "rancher/mirrored-jimmidyson-configmap-reload"
	configReloaderImageTag  = "v0.4.0"
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
	managedChartClient  ctlmgmtv3.ManagedChartClient
	managedChartCache   ctlmgmtv3.ManagedChartCache
	pvcClient           ctlcorev1.PersistentVolumeClaimClient
	statefulSetClient   ctlappsv1.StatefulSetClient
	statefulSetCache    ctlappsv1.StatefulSetCache
	upgradeClient       ctlharvesterv1.UpgradeClient
	upgradeCache        ctlharvesterv1.UpgradeCache
	upgradeLogClient    ctlharvesterv1.UpgradeLogClient
	upgradeLogCache     ctlharvesterv1.UpgradeLogCache
}

func (h *handler) OnUpgradeLogChange(_ string, upgradeLog *harvesterv1.UpgradeLog) (*harvesterv1.UpgradeLog, error) {
	if upgradeLog == nil || upgradeLog.DeletionTimestamp != nil {
		return upgradeLog, nil
	}
	logrus.Infof("Processing UpgradeLog %s/%s", upgradeLog.Namespace, upgradeLog.Name)

	if harvesterv1.UpgradeLogReady.GetStatus(upgradeLog) == "" {
		logrus.Info("Initialize UpgradeLog")
		toUpdate := upgradeLog.DeepCopy()
		harvesterv1.UpgradeLogReady.CreateUnknownIfNotExists(toUpdate)
		return h.upgradeLogClient.Update(toUpdate)
	}

	if harvesterv1.OperatorDeployed.GetStatus(upgradeLog) == "" {
		logrus.Info("Check if there are any existing logging-operator")

		toUpdate := upgradeLog.DeepCopy()
		harvesterv1.OperatorDeployed.CreateUnknownIfNotExists(toUpdate)

		// Detect rancher-logging Addon
		if addon, err := h.addonCache.Get(addonNamespace, rancherLoggingAddonName); err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, err
			}
			logrus.Info("rancher-logging Addon is not installed")
		} else {
			if addon.Status.Status == harvesterv1.AddonEnabled {
				setOperatorDeployedCondition(toUpdate, corev1.ConditionTrue, "Skipped", "rancher-logging Addon is enabled")
				return h.upgradeLogClient.Update(toUpdate)
			}
			logrus.Info("rancher-logging Addon is not enabled")
		}

		// Detect the rancher-logging ManagedChart
		if managedChart, err := h.managedChartCache.Get(managedChartNamespace, rancherLoggingManagedChartName); err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, err
			}
			logrus.Info("rancher-logging ManagedChart is not installed")
		} else {
			if managedChart.Status.Summary.DesiredReady > 0 && managedChart.Status.Summary.DesiredReady == managedChart.Status.Summary.Ready {
				setOperatorDeployedCondition(toUpdate, corev1.ConditionTrue, "Skipped", "rancher-logging ManagedChart is ready")
				return h.upgradeLogClient.Update(toUpdate)
			}
			logrus.Warn("rancher-logging ManagedChart is not ready")
			return nil, err
		}

		// If none of the above exists, install the customized rancher-logging ManagedChart
		logrus.Info("Deploy logging-operator")
		if _, err := h.managedChartClient.Create(prepareOperator(upgradeLog)); err != nil && !apierrors.IsAlreadyExists(err) {
			return nil, err
		}

		return h.upgradeLogClient.Update(toUpdate)
	}

	if harvesterv1.OperatorDeployed.IsTrue(upgradeLog) && harvesterv1.InfraScaffolded.GetStatus(upgradeLog) == "" {
		logrus.Info("Start to scaffold the logging infrastructure for upgrade procedure")

		toUpdate := upgradeLog.DeepCopy()

		if _, err := h.pvcClient.Create(preparePvc(upgradeLog)); err != nil && !apierrors.IsAlreadyExists(err) {
			return nil, err
		}
		if _, err := h.loggingClient.Create(prepareLogging(upgradeLog)); err != nil && !apierrors.IsAlreadyExists(err) {
			return nil, err
		}

		harvesterv1.InfraScaffolded.CreateUnknownIfNotExists(toUpdate)

		return h.upgradeLogClient.Update(toUpdate)
	} else if harvesterv1.OperatorDeployed.IsTrue(upgradeLog) && harvesterv1.InfraScaffolded.IsUnknown(upgradeLog) {
		logrus.Info("Check if the logging infrastructure is ready")

		toUpdate := upgradeLog.DeepCopy()

		fluentBitAnnotation, ok := upgradeLog.Annotations[upgradeLogFluentBitAnnotation]
		if !ok {
			return upgradeLog, nil
		}
		fluentdAnnotation, ok := upgradeLog.Annotations[upgradeLogFluentdAnnotation]
		if !ok {
			return upgradeLog, nil
		}

		if isInfraReady := (fluentBitAnnotation == upgradeLogFluentBitReady) && (fluentdAnnotation == upgradeLogFluentdReady); isInfraReady {
			logrus.Info("Logging infrastructure is ready")
			setInfraScaffoldedCondition(toUpdate, corev1.ConditionTrue, "", "")
			return h.upgradeLogClient.Update(toUpdate)
		}

		return upgradeLog, nil
	}

	if harvesterv1.InfraScaffolded.IsTrue(upgradeLog) && harvesterv1.UpgradeLogReady.IsUnknown(upgradeLog) {
		logrus.Info("Check if the log-collecting rules are installed")

		toUpdate := upgradeLog.DeepCopy()

		clusterFlowAnnotation := upgradeLog.Annotations[upgradeLogClusterFlowAnnotation]
		clusterOutputAnnotation := upgradeLog.Annotations[upgradeLogClusterOutputAnnotation]

		if isLogReady := (clusterOutputAnnotation == upgradeLogClusterOutputReady) && (clusterFlowAnnotation == upgradeLogClusterFlowReady); isLogReady {
			logrus.Info("Log-collecting rules exist and are activated")
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

	if harvesterv1.UpgradeLogReady.IsTrue(upgradeLog) && harvesterv1.UpgradeEnded.GetStatus(upgradeLog) == "" {
		logrus.Info("Logging infrastructure is ready, proceed the upgrade procedure")

		// handle corresponding upgrade resource
		upgradeName := upgradeLog.Spec.Upgrade
		upgrade, err := h.upgradeCache.Get(upgradeLogNamespace, upgradeName)
		if err != nil {
			return nil, err
		}
		upgradeToUpdate := upgrade.DeepCopy()

		if upgradeToUpdate.Labels == nil {
			upgradeToUpdate.Labels = map[string]string{}
		}
		upgradeToUpdate.Labels[upgradeStateLabel] = UpgradeStateLoggingInfraPrepared
		harvesterv1.LogReady.SetStatus(upgradeToUpdate, string(corev1.ConditionTrue))
		harvesterv1.LogReady.Reason(upgradeToUpdate, "")
		harvesterv1.LogReady.Message(upgradeToUpdate, "")

		if _, err := h.upgradeClient.Update(upgradeToUpdate); err != nil {
			return upgradeLog, err
		}

		// handle upgradeLog resource
		toUpdate := upgradeLog.DeepCopy()
		if toUpdate.Annotations == nil {
			toUpdate.Annotations = make(map[string]string)
		}
		toUpdate.Annotations[upgradeLogStateAnnotation] = upgradeLogStateCollecting
		harvesterv1.UpgradeEnded.CreateUnknownIfNotExists(toUpdate)

		return h.upgradeLogClient.Update(toUpdate)
	}

	if harvesterv1.UpgradeEnded.IsUnknown(upgradeLog) && harvesterv1.DownloadReady.GetStatus(upgradeLog) == "" {
		logrus.Info("Spin up downloader")

		// Get image version for log-downloader
		upgradeName := upgradeLog.Spec.Upgrade
		upgrade, err := h.upgradeCache.Get(upgradeLogNamespace, upgradeName)
		if err != nil {
			return nil, err
		}
		imageVersion := upgrade.Status.PreviousVersion

		if _, err := h.deploymentClient.Create(prepareLogDownloader(upgradeLog, imageVersion)); err != nil && !apierrors.IsAlreadyExists(err) {
			return nil, err
		}

		toUpdate := upgradeLog.DeepCopy()
		harvesterv1.DownloadReady.CreateUnknownIfNotExists(toUpdate)

		return h.upgradeLogClient.Update(toUpdate)
	}

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
	if clusterFlow == nil || clusterFlow.DeletionTimestamp != nil || clusterFlow.Labels == nil || clusterFlow.Namespace != upgradeLogNamespace {
		return clusterFlow, nil
	}
	logrus.Debugf("Processing ClusterFlow %s/%s", clusterFlow.Namespace, clusterFlow.Name)

	upgradeLogName, ok := clusterFlow.Labels[harvesterUpgradeLogLabel]
	if !ok {
		return clusterFlow, nil
	}
	upgradeLog, err := h.upgradeLogCache.Get(upgradeLogNamespace, upgradeLogName)
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
			toUpdate.Annotations = make(map[string]string)
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
	if clusterOutput == nil || clusterOutput.DeletionTimestamp != nil || clusterOutput.Labels == nil || clusterOutput.Namespace != upgradeLogNamespace {
		return clusterOutput, nil
	}
	logrus.Debugf("Processing ClusterOutput %s/%s", clusterOutput.Namespace, clusterOutput.Name)

	upgradeLogName, ok := clusterOutput.Labels[harvesterUpgradeLogLabel]
	if !ok {
		return clusterOutput, nil
	}
	upgradeLog, err := h.upgradeLogCache.Get(upgradeLogNamespace, upgradeLogName)
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
			toUpdate.Annotations = make(map[string]string)
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
	if daemonSet == nil || daemonSet.DeletionTimestamp != nil || daemonSet.Labels == nil || daemonSet.Namespace != upgradeLogNamespace {
		return daemonSet, nil
	}
	logrus.Debugf("Processing DaemonSet %s/%s", daemonSet.Namespace, daemonSet.Name)

	upgradeLogName, ok := daemonSet.Labels[harvesterUpgradeLogLabel]
	if !ok {
		return daemonSet, nil
	}
	upgradeLog, err := h.upgradeLogCache.Get(upgradeLogNamespace, upgradeLogName)
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

	if daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled {
		if toUpdate.Annotations == nil {
			toUpdate.Annotations = make(map[string]string)
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
	if deployment == nil || deployment.DeletionTimestamp != nil || deployment.Labels == nil || deployment.Namespace != upgradeLogNamespace {
		return deployment, nil
	}
	logrus.Debugf("Processing Deployment %s/%s", deployment.Namespace, deployment.Name)

	upgradeLogName, ok := deployment.Labels[harvesterUpgradeLogLabel]
	if !ok {
		return deployment, nil
	}
	upgradeLog, err := h.upgradeLogCache.Get(upgradeLogNamespace, upgradeLogName)
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

	upgradeLogName, ok := job.Labels[harvesterUpgradeLogLabel]
	if !ok {
		return job, nil
	}
	upgradeLog, err := h.upgradeLogCache.Get(upgradeLogNamespace, upgradeLogName)
	if err != nil {
		return job, err
	}
	logrus.Debugf("Found relevant UpgradeLog %s/%s", upgradeLog.Namespace, upgradeLog.Name)

	toUpdate := upgradeLog.DeepCopy()
	if job.Status.Succeeded > 0 {
		archiveName, ok := job.Annotations[archiveNameAnnotation]
		if !ok {
			return job, nil
		}
		if err := setUpgradeLogArchiveReady(toUpdate, archiveName, true); err != nil {
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

	upgradeLogName, ok := managedChart.Labels[harvesterUpgradeLogLabel]
	if !ok {
		return managedChart, nil
	}
	upgradeLog, err := h.upgradeLogCache.Get(upgradeLogNamespace, upgradeLogName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return managedChart, nil
		}
		return nil, err
	}
	logrus.Debugf("Found relevant UpgradeLog %s/%s", upgradeLog.Namespace, upgradeLog.Name)

	toUpdate := upgradeLog.DeepCopy()
	if managedChart.Status.Summary.DesiredReady > 0 && managedChart.Status.Summary.DesiredReady == managedChart.Status.Summary.Ready {
		setOperatorDeployedCondition(toUpdate, corev1.ConditionTrue, "", "")
		if _, err := h.upgradeLogClient.Update(toUpdate); err != nil {
			return managedChart, err
		}
	}

	return managedChart, nil
}

func (h *handler) OnStatefulSetChange(_ string, statefulSet *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	if statefulSet == nil || statefulSet.DeletionTimestamp != nil || statefulSet.Labels == nil || statefulSet.Namespace != upgradeLogNamespace {
		return statefulSet, nil
	}
	logrus.Debugf("Processing StatefulSet %s/%s", statefulSet.Namespace, statefulSet.Name)

	upgradeLogName, ok := statefulSet.Labels[harvesterUpgradeLogLabel]
	if !ok {
		return statefulSet, nil
	}
	upgradeLog, err := h.upgradeLogCache.Get(upgradeLogNamespace, upgradeLogName)
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
			toUpdate.Annotations = make(map[string]string)
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

func (h *handler) OnUpgradeChange(_ string, upgrade *harvesterv1.Upgrade) (*harvesterv1.Upgrade, error) {
	if upgrade == nil || upgrade.DeletionTimestamp != nil || upgrade.Labels == nil || upgrade.Namespace != upgradeNamespace {
		return upgrade, nil
	}
	logrus.Debugf("Processing Upgrade %s/%s", upgrade.Namespace, upgrade.Name)

	upgradeLogName := upgrade.Status.UpgradeLog
	if upgradeLogName == "" {
		logrus.Info("No related UpgradeLog resource found, skip purging")
		return upgrade, nil
	}
	upgradeLog, err := h.upgradeLogCache.Get(upgradeLogNamespace, upgradeLogName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Debugf("The corresponding UpgradeLog %s/%s is not found, skip purging", upgradeLogNamespace, upgradeLogName)
			return upgrade, nil
		}
		return nil, err
	}

	if upgrade.Labels[upgradeReadMessageLabel] == "true" {
		logrus.Infof("Purging UpgradeLog %s/%s and its relevant sub-components", upgradeLog.Namespace, upgradeLog.Name)
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

	if err := h.clusterFlowClient.Delete(upgradeLogNamespace, name.SafeConcatName(upgradeLog.Name, FlowComponent), &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err := h.clusterOutputClient.Delete(upgradeLogNamespace, name.SafeConcatName(upgradeLog.Name, OutputComponent), &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err := h.loggingClient.Delete(name.SafeConcatName(upgradeLog.Name, InfraComponent), &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err := h.managedChartClient.Delete(managedChartNamespace, name.SafeConcatName(upgradeLog.Name, OperatorComponent), &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

func (h *handler) cleanup(upgradeLog *harvesterv1.UpgradeLog) error {
	logrus.Info("Removing logging-operator ManagedChart if any")

	if err := h.managedChartClient.Delete(managedChartNamespace, name.SafeConcatName(upgradeLog.Name, OperatorComponent), &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}
