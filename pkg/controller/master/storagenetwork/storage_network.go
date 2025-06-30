package storagenetwork

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	ctlmgmtv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/apps/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	ctlmonitoringv1 "github.com/harvester/harvester/pkg/generated/controllers/monitoring.coreos.com/v1"
	whereaboutscniv1 "github.com/harvester/harvester/pkg/generated/controllers/whereabouts.cni.cncf.io/v1alpha1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/network"
)

const (
	ControllerName = "harvester-storage-network-controller"

	// for compatiability, will be removed on Harvester v1.6.0
	StorageNetworkAnnotation        = util.StorageNetworkAnnotation
	ReplicaStorageNetworkAnnotation = util.ReplicaStorageNetworkAnnotation
	PausedStorageNetworkAnnotation  = util.PausedStorageNetworkAnnotation
	HashStorageNetworkAnnotation    = util.HashStorageNetworkAnnotation
	NadStorageNetworkAnnotation     = util.NadStorageNetworkAnnotation
	OldNadStorageNetworkAnnotation  = util.OldNadStorageNetworkAnnotation

	HashStorageNetworkLabel = util.HashStorageNetworkLabel

	StorageNetworkNetAttachDefPrefix    = util.StorageNetworkNetAttachDefPrefix
	StorageNetworkNetAttachDefNamespace = util.StorageNetworkNetAttachDefNamespace

	BridgeSuffix = "-br"

	// status
	ReasonInProgress         = "In Progress"
	ReasonCompleted          = "Completed"
	MsgRestartPod            = "Restarting Pods"
	MsgStopPod               = "Stopping Pods"
	MsgUpdateLonghornSetting = "Update Longhorn setting"
	MsgIPAssignmentFailure   = "IP allocation failure for Longhorn Pods"

	// error messages
	msgWaitForVolumes = "waiting for all volumes detached: %s"

	longhornStorageNetworkName = "storage-network"
)

type Handler struct {
	ctx                               context.Context
	settings                          ctlharvesterv1.SettingClient
	longhornSettings                  ctllhv1.SettingClient
	longhornSettingCache              ctllhv1.SettingCache
	longhornVolumeCache               ctllhv1.VolumeCache
	prometheus                        ctlmonitoringv1.PrometheusClient
	prometheusCache                   ctlmonitoringv1.PrometheusCache
	alertmanager                      ctlmonitoringv1.AlertmanagerClient
	alertmanagerCache                 ctlmonitoringv1.AlertmanagerCache
	deployments                       v1.DeploymentClient
	deploymentCache                   v1.DeploymentCache
	managedCharts                     ctlmgmtv3.ManagedChartClient
	managedChartCache                 ctlmgmtv3.ManagedChartCache
	networkAttachmentDefinitions      ctlcniv1.NetworkAttachmentDefinitionClient
	networkAttachmentDefinitionsCache ctlcniv1.NetworkAttachmentDefinitionCache
	whereaboutsCNIIPPoolCache         whereaboutscniv1.IPPoolCache
	settingsController                ctlharvesterv1.SettingController
	nodeCache                         ctlcorev1.NodeCache
}

// register the setting controller and reconsile longhorn setting when storage network changed
func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	longhornSettings := management.LonghornFactory.Longhorn().V1beta2().Setting()
	longhornVolumes := management.LonghornFactory.Longhorn().V1beta2().Volume()
	prometheus := management.MonitoringFactory.Monitoring().V1().Prometheus()
	alertmanager := management.MonitoringFactory.Monitoring().V1().Alertmanager()
	deployments := management.AppsFactory.Apps().V1().Deployment()
	managedCharts := management.RancherManagementFactory.Management().V3().ManagedChart()
	networkAttachmentDefinitions := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()
	whereaboutsCNI := management.WhereaboutsCNIFactory.Whereabouts().V1alpha1()
	node := management.CoreFactory.Core().V1().Node()

	controller := &Handler{
		ctx:                               ctx,
		settings:                          settings,
		longhornSettings:                  longhornSettings,
		longhornSettingCache:              longhornSettings.Cache(),
		longhornVolumeCache:               longhornVolumes.Cache(),
		prometheus:                        prometheus,
		prometheusCache:                   prometheus.Cache(),
		alertmanager:                      alertmanager,
		alertmanagerCache:                 alertmanager.Cache(),
		deployments:                       deployments,
		deploymentCache:                   deployments.Cache(),
		managedCharts:                     managedCharts,
		managedChartCache:                 managedCharts.Cache(),
		networkAttachmentDefinitions:      networkAttachmentDefinitions,
		networkAttachmentDefinitionsCache: networkAttachmentDefinitions.Cache(),
		whereaboutsCNIIPPoolCache:         whereaboutsCNI.IPPool().Cache(),
		settingsController:                settings,
		nodeCache:                         node.Cache(),
	}

	settings.OnChange(ctx, ControllerName, controller.OnStorageNetworkChange)
	return nil
}

func (h *Handler) setConfiguredCondition(setting *harvesterv1.Setting, finish bool, reason string, msg string) (*harvesterv1.Setting, error) {
	settingCopy := setting.DeepCopy()
	if finish {
		harvesterv1.SettingConfigured.True(settingCopy)
	} else {
		harvesterv1.SettingConfigured.False(settingCopy)
	}

	harvesterv1.SettingConfigured.Reason(settingCopy, reason)
	harvesterv1.SettingConfigured.Message(settingCopy, msg)

	if !reflect.DeepEqual(settingCopy, setting) {
		s, err := h.settings.Update(settingCopy)
		if err != nil {
			return s, err
		}
		return s, nil
	}

	return setting, nil
}

// webhook needs check if VMs are off
func (h *Handler) OnStorageNetworkChange(_ string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil || setting.Name != settings.StorageNetworkName {
		return setting, nil
	}
	settingCopy := setting.DeepCopy()

	if settingCopy.Annotations == nil {
		if settingCopy.Value == "" {
			// initialization case, don't update status, just skip it.
			return setting, nil
		}
		settingCopy.Annotations = make(map[string]string)
	}

	var (
		updatedSetting *harvesterv1.Setting
		err            error
		value          string
	)

	if updatedSetting, err = h.checkValueIsChanged(settingCopy); err != nil {
		return updatedSetting, err
	}

	value, err = h.getLonghornStorageNetwork()
	if err != nil {
		return setting, err
	}

	currentNad := setting.Annotations[util.NadStorageNetworkAnnotation]
	if currentNad == value {
		// if post config is successful, it will finish the onChange
		return h.handleLonghornSettingPostConfig(settingCopy)
	}

	logrus.Infof("storage network change: %s", settingCopy.Value)

	// if replica eq 0, skip
	// save replica to annotation
	// set replica to 0
	if err = h.checkPodStatusAndStop(); err != nil {
		logrus.Infof("Requeue to check pod status again")
		updatedSetting, updateConditionErr := h.setConfiguredCondition(settingCopy, false, ReasonInProgress, MsgStopPod)
		if updateConditionErr != nil {
			return setting, fmt.Errorf("update status error %v", updateConditionErr)
		}
		return updatedSetting, err
	}

	// check volume detach before put LH settings
	if err = h.checkLonghornVolumeDetached(); err != nil {
		updatedSetting, updateConditionErr := h.setConfiguredCondition(settingCopy, false, ReasonInProgress, err.Error())
		if updateConditionErr != nil {
			return setting, fmt.Errorf("update status error %v", updateConditionErr)
		}
		return updatedSetting, err
	}

	logrus.Infof("all pods are stopped")
	logrus.Infof("all volumes are detached")
	logrus.Infof("update Longhorn settings")
	// push LH setting
	nadName := settingCopy.Annotations[util.NadStorageNetworkAnnotation]
	if err = h.updateLonghornStorageNetwork(nadName); err != nil {
		return setting, fmt.Errorf("update Longhorn setting error %v", err)
	}
	if updatedSetting, err = h.setConfiguredCondition(settingCopy, false, ReasonInProgress, MsgUpdateLonghornSetting); err != nil {
		return setting, fmt.Errorf("update status error %v", err)
	}

	return updatedSetting, nil
}

// calc sha1 hash
func (h *Handler) sha1(s string) string {
	hash := sha1.New()
	hash.Write([]byte(s))
	sha1sum := hash.Sum(nil)
	return fmt.Sprintf("%x", sha1sum)
}

func (h *Handler) checkIsSameHashValue(setting *harvesterv1.Setting) bool {
	currentHash := h.sha1(setting.Value)
	savedHash := setting.Annotations[util.HashStorageNetworkAnnotation]
	return currentHash == savedHash
}

func (h *Handler) setHashAnnotations(setting *harvesterv1.Setting) *harvesterv1.Setting {
	setting.Annotations[util.HashStorageNetworkAnnotation] = h.sha1(setting.Value)
	return setting
}

func (h *Handler) setNadAnnotations(setting *harvesterv1.Setting, newNad string) *harvesterv1.Setting {
	setting.Annotations[util.OldNadStorageNetworkAnnotation] = setting.Annotations[util.NadStorageNetworkAnnotation]
	setting.Annotations[util.NadStorageNetworkAnnotation] = newNad
	return setting
}

func (h *Handler) createNad(setting *harvesterv1.Setting) (*nadv1.NetworkAttachmentDefinition, error) {
	var config network.Config
	if err := json.Unmarshal([]byte(setting.Value), &config); err != nil {
		return nil, fmt.Errorf("parsing value error %v", err)
	}
	bridgeConfig := network.CreateBridgeConfig(config)

	nadConfig, err := json.Marshal(bridgeConfig)
	if err != nil {
		return nil, fmt.Errorf("output json error %v", err)
	}

	nad := nadv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: util.StorageNetworkNetAttachDefPrefix,
			Namespace:    util.StorageNetworkNetAttachDefNamespace,
		},
	}
	nad.Annotations = map[string]string{
		util.StorageNetworkAnnotation: "true",
	}
	nad.Labels = map[string]string{
		util.HashStorageNetworkLabel: h.sha1(setting.Value),
	}
	nad.Spec.Config = string(nadConfig)

	// create nad
	var nadResult *nadv1.NetworkAttachmentDefinition
	if nadResult, err = h.networkAttachmentDefinitions.Create(&nad); err != nil {
		return nil, fmt.Errorf("create net-attach-def failed %v", err)
	}

	return nadResult, nil
}

func (h *Handler) findOrCreateNad(setting *harvesterv1.Setting) (*nadv1.NetworkAttachmentDefinition, error) {
	nads, err := h.networkAttachmentDefinitions.List(util.StorageNetworkNetAttachDefNamespace, metav1.ListOptions{
		LabelSelector: labels.Set{
			util.HashStorageNetworkLabel: h.sha1(setting.Value),
		}.String(),
	})
	if err != nil {
		return nil, err
	}

	if len(nads.Items) == 0 {
		return h.createNad(setting)
	}

	if len(nads.Items) > 1 {
		logrus.WithFields(logrus.Fields{
			"num_of_nad":          len(nads.Items),
			"storage_network_nad": nads.Items[0].Name,
		}).Info("storage network: found more than one match nad")
	}

	return &nads.Items[0], nil
}

func (h *Handler) checkValueIsChanged(setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	var updatedSetting *harvesterv1.Setting
	var err error
	nadAnnotation := ""

	if h.checkIsSameHashValue(setting) {
		return setting, nil
	}

	if setting.Value != "" {
		nad, err := h.findOrCreateNad(setting)
		if err != nil {
			return setting, err
		}
		nadAnnotation = fmt.Sprintf("%s/%s", nad.Namespace, nad.Name)
	}

	setting = h.setNadAnnotations(setting, nadAnnotation)
	setting = h.setHashAnnotations(setting)

	if updatedSetting, err = h.setConfiguredCondition(setting, false, ReasonInProgress, "create NAD"); err != nil {
		return setting, fmt.Errorf("create nad update status error %v", err)
	}
	return updatedSetting, fmt.Errorf("check hash again")
}

func (h *Handler) removeOldNad(setting *harvesterv1.Setting) error {
	oldNad := setting.Annotations[util.OldNadStorageNetworkAnnotation]
	if oldNad == "" {
		return nil
	}

	nadName := strings.Split(oldNad, "/")
	if len(nadName) != 2 {
		logrus.Errorf("split nad namespace and name failed %s", oldNad)
		setting.Annotations[util.OldNadStorageNetworkAnnotation] = ""
		return nil
	}
	namespace := nadName[0]
	name := nadName[1]

	if _, err := h.networkAttachmentDefinitionsCache.Get(namespace, name); err != nil {
		if apierrors.IsNotFound(err) {
			setting.Annotations[util.OldNadStorageNetworkAnnotation] = ""
			return nil
		}

		// retry again
		return fmt.Errorf("check net-attach-def existing error %v", err)
	}

	if err := h.networkAttachmentDefinitions.Delete(namespace, name, &metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("remove nad error %v", err)
	}

	setting.Annotations[util.OldNadStorageNetworkAnnotation] = ""
	return nil
}

func (h *Handler) validateIPAddressesAllocations(setting *harvesterv1.Setting) error {
	if setting.Value == "" {
		return nil
	}

	nodes, err := h.nodeCache.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list node cache %v", err)
	}

	//Formula - https://docs.harvesterhci.io/v1.4/advanced/storagenetwork/
	//Dynamic parameters like number of images download/upload, backing-image-manager and backing-image-ds are skipped
	//and only the number of nodes each running an instance-manager pod is used
	MinAllocatableIPAddrs := len(nodes)

	var config network.Config

	if err := json.Unmarshal([]byte(setting.Value), &config); err != nil {
		return fmt.Errorf("parsing value error %v", err)
	}

	ipprefix := strings.Split(config.Range, "/")
	ippoolName := ipprefix[0] + "-" + ipprefix[1]

	ippool, err := h.whereaboutsCNIIPPoolCache.Get(util.KubeSystemNamespace, ippoolName)
	if err != nil {
		return fmt.Errorf("wherabouts IPPool not found for pool %s error %v", ippoolName, err)
	}

	if len(ippool.Spec.Allocations) >= MinAllocatableIPAddrs {
		return nil
	}

	return fmt.Errorf("whereabouts cni IP allocation failure for IPPool %s retrying again... required %d allocated %d",
		ippoolName, MinAllocatableIPAddrs, len(ippool.Spec.Allocations))
}

func (h *Handler) handleLonghornSettingPostConfig(setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	// check if we need to restart monitoring pods
	if err := h.checkPodStatusAndStart(); err != nil {
		if _, updateConditionErr := h.setConfiguredCondition(setting, false, ReasonInProgress, MsgRestartPod); updateConditionErr != nil {
			return setting, fmt.Errorf("update status error %v", updateConditionErr)
		}
		return setting, err
	}

	settingCopy := setting.DeepCopy()
	if err := h.removeOldNad(settingCopy); err != nil {
		return setting, fmt.Errorf("remove old nad error %v", err)
	}

	err := h.validateIPAddressesAllocations(setting)
	if err != nil {
		setting, err := h.setConfiguredCondition(setting, false, ReasonInProgress, MsgIPAssignmentFailure)
		if err != nil {
			return setting, fmt.Errorf("update status error %v", err)
		}
		h.settingsController.Enqueue(setting.Name)
		return setting, err
	}

	updatedSetting, err := h.setConfiguredCondition(settingCopy, true, ReasonCompleted, "")
	if err != nil {
		return setting, fmt.Errorf("update status error %v", err)
	}
	// finish config
	return updatedSetting, nil
}

// true: all detach
func (h *Handler) checkLonghornVolumeDetached() error {
	volumes, err := h.longhornVolumeCache.List(util.LonghornSystemNamespaceName, labels.Everything())
	if err != nil {
		return fmt.Errorf("volume error %v", err)
	}

	attachedVolume := make([]string, 0)
	for _, volume := range volumes {
		if volume.Status.State != "detached" {
			attachedVolume = append(attachedVolume, volume.Name)
		}
	}

	if len(attachedVolume) > 0 {
		return fmt.Errorf(msgWaitForVolumes, strings.Join(attachedVolume, ","))
	}

	return nil
}

func (h *Handler) checkPrometheusStatusAndStart() error {
	// check prometheus cattle-monitoring-system/rancher-monitoring-prometheus replica
	prometheus, err := h.prometheusCache.Get(util.CattleMonitoringSystemNamespace, util.RancherMonitoringPrometheus)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("prometheus not found. skip")
			return nil
		}
		return fmt.Errorf("prometheus get error %v", err)
	}

	// check started or not
	if replicasStr, ok := prometheus.Annotations[util.ReplicaStorageNetworkAnnotation]; ok {
		logrus.Infof("current prometheus replicas: %v", *prometheus.Spec.Replicas)
		logrus.Infof("start prometheus")
		prometheusCopy := prometheus.DeepCopy()
		replicas, err := strconv.ParseInt(replicasStr, 10, 32)
		if err != nil {
			return fmt.Errorf("strconv ParseInt error %v", err)
		}
		*prometheusCopy.Spec.Replicas = int32(replicas)
		delete(prometheusCopy.Annotations, util.ReplicaStorageNetworkAnnotation)

		if _, err := h.prometheus.Update(prometheusCopy); err != nil {
			return fmt.Errorf("prometheus update error %v", err)
		}
		return nil
	}

	return nil
}

func (h *Handler) checkAlertmanagerStatusAndStart() error {
	// check alertmanager cattle-monitoring-system/rancher-monitoring-alertmanager replica
	alertmanager, err := h.alertmanagerCache.Get(util.CattleMonitoringSystemNamespace, util.RancherMonitoringAlertmanager)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("Alertmanager not found. skip")
			return nil
		}
		return fmt.Errorf("alertmanager get error %v", err)
	}

	// check started or not
	if replicasStr, ok := alertmanager.Annotations[util.ReplicaStorageNetworkAnnotation]; ok {
		logrus.Infof("current alertmanager replicas: %v", *alertmanager.Spec.Replicas)
		logrus.Infof("start alertmanager")
		alertmanagerCopy := alertmanager.DeepCopy()
		replicas, err := strconv.ParseInt(replicasStr, 10, 32)
		if err != nil {
			return fmt.Errorf("strconv ParseInt error %v", err)
		}
		*alertmanagerCopy.Spec.Replicas = int32(replicas)
		delete(alertmanagerCopy.Annotations, util.ReplicaStorageNetworkAnnotation)

		if _, err := h.alertmanager.Update(alertmanagerCopy); err != nil {
			return fmt.Errorf("alertmanager update error %v", err)
		}
		return nil
	}

	return nil
}

func (h *Handler) checkGrafanaStatusAndStart() error {
	// check deployment cattle-monitoring-system/rancher-monitoring-grafana replica
	grafana, err := h.deploymentCache.Get(util.CattleMonitoringSystemNamespace, util.RancherMonitoringGrafana)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("grafana not found. skip")
			return nil
		}
		return fmt.Errorf("grafana get error %v", err)
	}

	// check started or not
	if replicasStr, ok := grafana.Annotations[util.ReplicaStorageNetworkAnnotation]; ok {
		logrus.Infof("current Grafana replicas: %v", *grafana.Spec.Replicas)
		logrus.Infof("start grafana")
		grafanaCopy := grafana.DeepCopy()
		replicas, err := strconv.ParseInt(replicasStr, 10, 32)
		if err != nil {
			return fmt.Errorf("strconv ParseInt error %v", err)
		}
		*grafanaCopy.Spec.Replicas = int32(replicas)
		delete(grafanaCopy.Annotations, util.ReplicaStorageNetworkAnnotation)

		if _, err := h.deployments.Update(grafanaCopy); err != nil {
			return fmt.Errorf("grafana update error %v", err)
		}
		return nil
	}

	return nil
}

func (h *Handler) checkRancherMonitoringStatusAndStart() error {
	// check managedchart fleet-local/rancher-monitoring paused
	monitoring, err := h.managedChartCache.Get(util.FleetLocalNamespaceName, util.RancherMonitoring)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("rancher monitoring not found. skip")
			return nil
		}
		return fmt.Errorf("rancher monitoring get error %v", err)
	}

	// check pause or not
	if _, ok := monitoring.Annotations[util.PausedStorageNetworkAnnotation]; ok {
		logrus.Infof("current Rancher Monitoring paused: %v", monitoring.Spec.Paused)
		logrus.Infof("start rancher monitoring")
		monitoringCopy := monitoring.DeepCopy()
		monitoringCopy.Spec.Paused = false
		delete(monitoringCopy.Annotations, util.PausedStorageNetworkAnnotation)

		if _, err := h.managedCharts.Update(monitoringCopy); err != nil {
			return fmt.Errorf("rancher monitoring error %v", err)
		}
		return nil
	}

	return nil
}

func (h *Handler) checkVMImportControllerStatusAndStart() error {
	// check deployment harvester-system/harvester-harvester-vm-import-controller replica
	vmImportControllerDeploy, err := h.deploymentCache.Get(util.HarvesterSystemNamespaceName, util.HarvesterVMImportController)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("VM import controller not found. skip")
			return nil
		}
		return fmt.Errorf("vm import controller get error %v", err)
	}

	logrus.Infof("current VM Import Controller replicas: %v", *vmImportControllerDeploy.Spec.Replicas)
	// check started or not
	if replicasStr, ok := vmImportControllerDeploy.Annotations[util.ReplicaStorageNetworkAnnotation]; ok {
		logrus.Infof("start vm import controller")
		vmImportControllerDeployCopy := vmImportControllerDeploy.DeepCopy()
		replicas, err := strconv.ParseInt(replicasStr, 10, 32)
		if err != nil {
			return fmt.Errorf("strconv ParseInt error %v", err)
		}
		*vmImportControllerDeployCopy.Spec.Replicas = int32(replicas)
		delete(vmImportControllerDeployCopy.Annotations, util.ReplicaStorageNetworkAnnotation)

		if _, err := h.deployments.Update(vmImportControllerDeployCopy); err != nil {
			return fmt.Errorf("VM Import Controller update error %v", err)
		}
		return nil
	}

	return nil
}

// check Pod status, if all pods are start, return true
func (h *Handler) checkPodStatusAndStart() error {
	if err := h.checkPrometheusStatusAndStart(); err != nil {
		return err
	}

	if err := h.checkAlertmanagerStatusAndStart(); err != nil {
		return err
	}

	if err := h.checkGrafanaStatusAndStart(); err != nil {
		return err
	}

	if err := h.checkRancherMonitoringStatusAndStart(); err != nil {
		return err
	}

	return h.checkVMImportControllerStatusAndStart()
}

func (h *Handler) checkRancherMonitoringStatusAndStop() error {
	// check managedchart fleet-local/rancher-monitoring paused
	monitoring, err := h.managedChartCache.Get(util.FleetLocalNamespaceName, util.RancherMonitoring)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("rancher monitoring not found. skip")
			return nil
		}
		return fmt.Errorf("rancher monitoring get error %v", err)
	}

	// check pause or not
	if !monitoring.Spec.Paused {
		logrus.Infof("current Rancher Monitoring paused: %v", monitoring.Spec.Paused)
		logrus.Infof("stop rancher monitoring")
		monitoringCopy := monitoring.DeepCopy()
		monitoringCopy.Annotations[util.PausedStorageNetworkAnnotation] = "false"
		monitoringCopy.Spec.Paused = true

		if _, err := h.managedCharts.Update(monitoringCopy); err != nil {
			return fmt.Errorf("rancher monitoring error %v", err)
		}
		return nil
	}

	return err
}

func (h *Handler) checkPrometheusStatusAndStop() error {
	// check prometheus cattle-monitoring-system/rancher-monitoring-prometheus replica
	prometheus, err := h.prometheusCache.Get(util.CattleMonitoringSystemNamespace, util.RancherMonitoringPrometheus)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("prometheus not found. skip")
			return nil
		}
		return fmt.Errorf("prometheus get error %v", err)
	}
	// check stopped or not
	if *prometheus.Spec.Replicas != 0 {
		logrus.Infof("current prometheus replicas: %v", *prometheus.Spec.Replicas)
		logrus.Infof("stop prometheus")
		prometheusCopy := prometheus.DeepCopy()
		prometheusCopy.Annotations[util.ReplicaStorageNetworkAnnotation] = strconv.Itoa(int(*prometheus.Spec.Replicas))
		*prometheusCopy.Spec.Replicas = 0

		if _, err := h.prometheus.Update(prometheusCopy); err != nil {
			return fmt.Errorf("prometheus update error %v", err)
		}
		return nil
	}

	return nil
}

func (h *Handler) checkAltermanagerStatusAndStop() error {
	// check alertmanager cattle-monitoring-system/rancher-monitoring-alertmanager replica
	alertmanager, err := h.alertmanagerCache.Get(util.CattleMonitoringSystemNamespace, util.RancherMonitoringAlertmanager)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("Alertmanager not found. skip")
			return nil
		}
		return fmt.Errorf("alertmanager get error %v", err)
	}

	// check stopped or not
	if *alertmanager.Spec.Replicas != 0 {
		logrus.Infof("current alertmanager replicas: %v", *alertmanager.Spec.Replicas)
		logrus.Infof("stop alertmanager")
		alertmanagerCopy := alertmanager.DeepCopy()
		alertmanagerCopy.Annotations[util.ReplicaStorageNetworkAnnotation] = strconv.Itoa(int(*alertmanager.Spec.Replicas))
		*alertmanagerCopy.Spec.Replicas = 0

		if _, err := h.alertmanager.Update(alertmanagerCopy); err != nil {
			return fmt.Errorf("alertmanager update error %v", err)
		}
		return nil
	}

	return nil
}

func (h *Handler) checkGrafanaStatusAndStop() error {
	// check deployment cattle-monitoring-system/rancher-monitoring-grafana replica
	grafana, err := h.deploymentCache.Get(util.CattleMonitoringSystemNamespace, util.RancherMonitoringGrafana)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("grafana no found. skip")
			return nil
		}
		return fmt.Errorf("grafana get error %v", err)
	}

	logrus.Infof("current Grafana replicas: %v", *grafana.Spec.Replicas)
	// check stopped or not
	if *grafana.Spec.Replicas != 0 {
		logrus.Infof("stop grafana")
		grafanaCopy := grafana.DeepCopy()
		grafanaCopy.Annotations[util.ReplicaStorageNetworkAnnotation] = strconv.Itoa(int(*grafana.Spec.Replicas))
		*grafanaCopy.Spec.Replicas = 0

		if _, err := h.deployments.Update(grafanaCopy); err != nil {
			return fmt.Errorf("grafana update error %v", err)
		}
		return nil
	}

	return nil
}

func (h *Handler) checkVMImportControllerStatusAndStop() error {
	// check deployment harvester-system/harvester-harvester-vm-import-controller replica
	vmimportcontroller, err := h.deploymentCache.Get(util.HarvesterSystemNamespaceName, util.HarvesterVMImportController)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("VM import controller no found. skip")
			return nil
		}
		return fmt.Errorf("vmimportcontroller get error %v", err)
	}

	// check stopped or not
	if *vmimportcontroller.Spec.Replicas != 0 {
		logrus.Infof("current VM Import Controller replicas: %v", *vmimportcontroller.Spec.Replicas)
		logrus.Infof("stop vmi import controller")
		vmimportcontrollerCopy := vmimportcontroller.DeepCopy()
		vmimportcontrollerCopy.Annotations[util.ReplicaStorageNetworkAnnotation] = strconv.Itoa(int(*vmimportcontroller.Spec.Replicas))
		*vmimportcontrollerCopy.Spec.Replicas = 0

		if _, err := h.deployments.Update(vmimportcontrollerCopy); err != nil {
			return fmt.Errorf("VM Import Controller update error %v", err)
		}
		return nil
	}

	return nil
}

// check Pod status, if all pods are stopped, return true
func (h *Handler) checkPodStatusAndStop() error {
	if err := h.checkRancherMonitoringStatusAndStop(); err != nil {
		return err
	}

	if err := h.checkPrometheusStatusAndStop(); err != nil {
		return err
	}

	if err := h.checkAltermanagerStatusAndStop(); err != nil {
		return err
	}

	if err := h.checkGrafanaStatusAndStop(); err != nil {
		return err
	}

	return h.checkVMImportControllerStatusAndStop()
}

func (h *Handler) getLonghornStorageNetwork() (string, error) {
	storage, err := h.longhornSettingCache.Get(util.LonghornSystemNamespaceName, longhornStorageNetworkName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return storage.Value, nil
}

func (h *Handler) updateLonghornStorageNetwork(storageNetwork string) error {
	storage, err := h.longhornSettingCache.Get(util.LonghornSystemNamespaceName, longhornStorageNetworkName)
	if err != nil {
		return err
	}

	storageCpy := storage.DeepCopy()
	storageCpy.Value = storageNetwork

	if !reflect.DeepEqual(storage, storageCpy) {
		_, err := h.longhornSettings.Update(storageCpy)
		return err
	}
	return nil
}
