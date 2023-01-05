package upgradelog

import (
	"context"
	"fmt"
	"reflect"

	"github.com/sirupsen/logrus"

	loggingv1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlloggingv1 "github.com/harvester/harvester/pkg/generated/controllers/logging.banzaicloud.io/v1beta1"
	ctlappsv1 "github.com/rancher/wrangler/pkg/generated/controllers/apps/v1"
	ctlbatchv1 "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	harvesterUpgradeLogLabel            = "harvesterhci.io/upgradeLog"
	harvesterUpgradeLogStorageClassName = "harvester-longhorn"
	harvesterUpgradeLogVolumeMode       = corev1.PersistentVolumeFilesystem
	upgradeLogNamespace                 = "harvester-system"

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
)

type handler struct {
	ctx                 context.Context
	namespace           string
	clusterFlowClient   ctlloggingv1.ClusterFlowClient
	clusterOutputClient ctlloggingv1.ClusterOutputClient
	daemonSetClient     ctlappsv1.DaemonSetClient
	daemonSetCache      ctlappsv1.DaemonSetCache
	jobClient           ctlbatchv1.JobClient
	loggingClient       ctlloggingv1.LoggingClient
	pvcClient           ctlcorev1.PersistentVolumeClaimClient
	statefulSetClient   ctlappsv1.StatefulSetClient
	statefulSetCache    ctlappsv1.StatefulSetCache
	upgradeClient       ctlharvesterv1.UpgradeClient
	upgradeCache        ctlharvesterv1.UpgradeCache
	upgradeLogClient    ctlharvesterv1.UpgradeLogClient
	upgradeLogCache     ctlharvesterv1.UpgradeLogCache
}

func (h *handler) OnUpgradeLogChange(_ string, upgradeLog *harvesterv1.UpgradeLog) (*harvesterv1.UpgradeLog, error) {
	if upgradeLog == nil {
		return upgradeLog, nil
	}

	if harvesterv1.UpgradeLogReady.GetStatus(upgradeLog) == "" {
		logrus.Infof("[%s] Initialize upgradeLog %s/%s", upgradeLogControllerName, upgradeLog.Namespace, upgradeLog.Name)

		toUpdate := upgradeLog.DeepCopy()
		harvesterv1.UpgradeLogReady.CreateUnknownIfNotExists(toUpdate)

		logrus.Infof("[%s] Deploy logging-operator", upgradeLogControllerName)
		harvesterv1.OperatorDeployed.CreateUnknownIfNotExists(toUpdate)

		// NOTE: As of v1.1.1, the logging-operator is by default deployed (rancher-logging), so we set the condition to true directly.
		setOperatorDeployedCondition(toUpdate, corev1.ConditionTrue, "", "")
		logrus.Infof("[%s] logging-operator deployed", upgradeLogControllerName)

		return h.upgradeLogClient.Update(toUpdate)
	}

	if harvesterv1.OperatorDeployed.IsTrue(upgradeLog) && harvesterv1.InfraScaffolded.GetStatus(upgradeLog) == "" {
		logrus.Infof("[%s] Start to scaffold the logging infrastructure for upgrade procedure", upgradeLogControllerName)

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
		logrus.Infof("[%s] Check if the logging infrastructure is ready", upgradeLogControllerName)

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
			logrus.Infof("[%s] Logging infrastructure is ready", upgradeLogControllerName)
			setInfraScaffoldedCondition(toUpdate, corev1.ConditionTrue, "", "")
			return h.upgradeLogClient.Update(toUpdate)
		}

		return upgradeLog, nil
	}

	if harvesterv1.InfraScaffolded.IsTrue(upgradeLog) && harvesterv1.UpgradeLogReady.IsUnknown(upgradeLog) {
		logrus.Infof("[%s] Check if the log collecting rules are installed", upgradeLogControllerName)

		toUpdate := upgradeLog.DeepCopy()

		clusterFlowAnnotation := upgradeLog.Annotations[upgradeLogClusterFlowAnnotation]
		clusterOutputAnnotation := upgradeLog.Annotations[upgradeLogClusterOutputAnnotation]

		if isLogReady := (clusterOutputAnnotation == upgradeLogClusterOutputReady) && (clusterFlowAnnotation == upgradeLogClusterFlowReady); isLogReady {
			logrus.Infof("[%s] Log collecting rules are existed and activated", upgradeLogControllerName)
			setUpgradeLogReadyCondition(toUpdate, corev1.ConditionTrue, "", "")
			return h.upgradeLogClient.Update(toUpdate)
		} else {
			logrus.Infof("[%s] Start to create the clusterflow and clusteroutput resources for collecting upgrade logs", upgradeLogControllerName)
			if _, err := h.clusterOutputClient.Create(prepareClusterOutput(upgradeLog)); err != nil && !apierrors.IsAlreadyExists(err) {
				return nil, err
			}
			if _, err := h.clusterFlowClient.Create(prepareClusterFlow(upgradeLog)); err != nil && !apierrors.IsAlreadyExists(err) {
				return nil, err
			}
			return upgradeLog, nil
		}
	}

	if harvesterv1.UpgradeLogReady.IsTrue(upgradeLog) && harvesterv1.UpgradeEnded.GetStatus(upgradeLog) == "" {
		logrus.Infof("[%s] Logging infrastructure is ready, proceed the upgrade procedure", upgradeLogControllerName)

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
		harvesterv1.UpgradeEnded.CreateUnknownIfNotExists(toUpdate)

		return h.upgradeLogClient.Update(toUpdate)
	}

	if harvesterv1.UpgradeEnded.IsTrue(upgradeLog) && harvesterv1.DownloadReady.GetStatus(upgradeLog) == "" {
		logrus.Infof("[%s] Stop collecting logs", upgradeLogControllerName)

		if err := h.cleanup(upgradeLog); err != nil {
			return upgradeLog, err
		}

		toUpdate := upgradeLog.DeepCopy()
		harvesterv1.DownloadReady.CreateUnknownIfNotExists(toUpdate)

		return h.upgradeLogClient.Update(toUpdate)
	}

	return upgradeLog, nil
}

func (h *handler) OnUpgradeLogRemove(_ string, upgradeLog *harvesterv1.UpgradeLog) (*harvesterv1.UpgradeLog, error) {
	if upgradeLog == nil {
		return nil, nil
	}

	logrus.Infof("[%s] Deleting upgradeLog %s", upgradeLogControllerName, upgradeLog.Name)

	return upgradeLog, h.cleanup(upgradeLog)
}

func (h *handler) OnClusterFlowChange(_ string, clusterFlow *loggingv1.ClusterFlow) (*loggingv1.ClusterFlow, error) {
	if clusterFlow == nil || clusterFlow.DeletionTimestamp != nil || clusterFlow.Labels == nil || clusterFlow.Namespace != upgradeLogNamespace {
		return clusterFlow, nil
	}

	upgradeLogName, ok := clusterFlow.Labels[harvesterUpgradeLogLabel]
	if !ok {
		return clusterFlow, nil
	}

	upgradeLog, err := h.upgradeLogCache.Get(upgradeLogNamespace, upgradeLogName)
	if err != nil {
		return clusterFlow, err
	}

	toUpdate := upgradeLog.DeepCopy()

	if clusterFlow.Status.Active == nil {
		return clusterFlow, nil
	} else if *clusterFlow.Status.Active {
		logrus.Infof("[%s] clusterFlow %s/%s is now active", clusterFlowControllerName, clusterFlow.Namespace, clusterFlow.Name)
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

	upgradeLogName, ok := clusterOutput.Labels[harvesterUpgradeLogLabel]
	if !ok {
		return clusterOutput, nil
	}

	upgradeLog, err := h.upgradeLogCache.Get(upgradeLogNamespace, upgradeLogName)
	if err != nil {
		return clusterOutput, err
	}

	toUpdate := upgradeLog.DeepCopy()

	if clusterOutput.Status.Active == nil {
		return clusterOutput, nil
	} else if *clusterOutput.Status.Active {
		logrus.Infof("[%s] clusterOutput %s/%s is now active", clusterOutputControllerName, clusterOutput.Namespace, clusterOutput.Name)
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

	logrus.Infof("[%s] Processing daemonSet %s/%s", daemonSetControllerName, daemonSet.Namespace, daemonSet.Name)

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
	logrus.Infof("[%s] Found relevant upgradeLog %s/%s", daemonSetControllerName, upgradeLog.Namespace, upgradeLog.Name)

	fluentBitAnnotation, ok := upgradeLog.Annotations[upgradeLogFluentBitAnnotation]
	if ok && (fluentBitAnnotation == upgradeLogFluentBitReady) {
		logrus.Infof("[%s] Skipped syncing because fluentbit was marked as ready", daemonSetControllerName)
		return daemonSet, nil
	}

	toUpdate := upgradeLog.DeepCopy()

	if daemonSet.Status.NumberReady != daemonSet.Status.DesiredNumberScheduled {
		setInfraScaffoldedCondition(toUpdate, corev1.ConditionFalse, "FluentBitNotReady", "")
	} else {
		setInfraScaffoldedCondition(toUpdate, corev1.ConditionTrue, "FluentBitReady", "")
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

func (h *handler) OnJobChange(_ string, job *batchv1.Job) (*batchv1.Job, error) {
	if job == nil || job.DeletionTimestamp != nil {
		return job, nil
	}

	logrus.Infof("[%s] Processing job %s/%s", jobControllerName, job.Namespace, job.Name)

	return job, nil
}

func (h *handler) OnStatefulSetChange(_ string, statefulSet *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	if statefulSet == nil || statefulSet.DeletionTimestamp != nil || statefulSet.Labels == nil || statefulSet.Namespace != upgradeLogNamespace {
		return statefulSet, nil
	}

	logrus.Infof("[%s] Processing statefulSet %s/%s", statefulSetControllerName, statefulSet.Namespace, statefulSet.Name)

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
	logrus.Infof("[%s] Found relevant upgradeLog %s/%s", statefulSetControllerName, upgradeLog.Namespace, upgradeLog.Name)

	fluentdAnnotation, ok := upgradeLog.Annotations[upgradeLogFluentdAnnotation]
	if ok && (fluentdAnnotation == upgradeLogFluentdReady) {
		logrus.Infof("[%s] Skipped syncing because fluentd was marked as ready", statefulSetControllerName)
		return statefulSet, nil
	}

	toUpdate := upgradeLog.DeepCopy()

	if statefulSet.Status.ReadyReplicas != *statefulSet.Spec.Replicas {
		setInfraScaffoldedCondition(toUpdate, corev1.ConditionFalse, "FluentdNotReady", "")
	} else {
		setInfraScaffoldedCondition(toUpdate, corev1.ConditionTrue, "FluentdReady", "")
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

func (h *handler) cleanup(upgradeLog *harvesterv1.UpgradeLog) error {
	logrus.Infof("[%s] Tearing down the logging infrastructure for upgrade procedure", upgradeLogControllerName)

	if err := h.clusterFlowClient.Delete(upgradeLogNamespace, fmt.Sprintf("%s-clusterflow", upgradeLog.Name), &metav1.DeleteOptions{}); err != nil {
		return err
	}
	if err := h.clusterOutputClient.Delete(upgradeLogNamespace, fmt.Sprintf("%s-clusteroutput", upgradeLog.Name), &metav1.DeleteOptions{}); err != nil {
		return err
	}
	if err := h.loggingClient.Delete(fmt.Sprintf("%s-infra", upgradeLog.Name), &metav1.DeleteOptions{}); err != nil {
		return err
	}

	return nil
}
