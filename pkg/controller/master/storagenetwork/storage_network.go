package storagenetwork

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	cniv1 "github.com/containernetworking/cni/pkg/types"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	ctlmgmtv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	v1 "github.com/rancher/wrangler/pkg/generated/controllers/apps/v1"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	ctlmonitoringv1 "github.com/harvester/harvester/pkg/generated/controllers/monitoring.coreos.com/v1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	ControllerName                  = "harvester-storage-network-controller"
	StorageNetworkAnnotation        = "storage-network.settings.harvesterhci.io"
	ReplicaStorageNetworkAnnotation = StorageNetworkAnnotation + "/replica"
	PausedStorageNetworkAnnotation  = StorageNetworkAnnotation + "/paused"
	HashStorageNetworkAnnotation    = StorageNetworkAnnotation + "/hash"
	NadStorageNetworkAnnotation     = StorageNetworkAnnotation + "/net-attach-def"
	OldNadStorageNetworkAnnotation  = StorageNetworkAnnotation + "/old-net-attach-def"

	StorageNetworkNetAttachDefPrefix    = "storagenetwork-"
	StorageNetworkNetAttachDefNamespace = "harvester-system"

	BridgeSuffix = "-br"
	CNIVersion   = "0.3.1"
	DefaultPVID  = 1
	DefaultCNI   = "bridge"
	DefaultIPAM  = "whereabouts"

	// status
	ReasonInProgress         = "In Progress"
	ReasonCompleted          = "Completed"
	MsgRestartPod            = "Restarting Pods"
	MsgStopPod               = "Stopping Pods"
	MsgWaitForVolumes        = "Waiting for all volumes detached: %s"
	MsgUpdateLonghornSetting = "Update Longhorn setting"

	longhornStorageNetworkName = "storage-network"

	// Rancher monitoring
	CattleMonitoringSystemNamespace = "cattle-monitoring-system"
	RancherMonitoringPrometheus     = "rancher-monitoring-prometheus"
	RancherMonitoringAlertmanager   = "rancher-monitoring-alertmanager"
	FleetLocalNamespace             = "fleet-local"
	RancherMonitoring               = "rancher-monitoring"
	RancherMonitoringGrafana        = "rancher-monitoring-grafana"

	// VM import controller
	HarvesterSystemNamespace    = "harvester-system"
	HarvesterVMImportController = "harvester-vm-import-controller"
)

type Config struct {
	ClusterNetwork string   `json:"clusterNetwork,omitempty"`
	Vlan           uint16   `json:"vlan,omitempty"`
	Range          string   `json:"range,omitempty"`
	Exclude        []string `json:"exclude,omitempty"`
}

type BridgeConfig struct {
	cniv1.NetConf
	Bridge      string     `json:"bridge"`
	PromiscMode bool       `json:"promiscMode"`
	Vlan        uint16     `json:"vlan"`
	IPAM        IPAMConfig `json:"ipam"`
}

type IPAMConfig struct {
	Type    string   `json:"type"`
	Range   string   `json:"range"`
	Exclude []string `json:"exclude,omitempty"`
}

func NewBridgeConfig() *BridgeConfig {
	return &BridgeConfig{
		NetConf: cniv1.NetConf{
			CNIVersion: CNIVersion,
			Type:       DefaultCNI,
		},
		PromiscMode: true,
		Vlan:        DefaultPVID,
		IPAM: IPAMConfig{
			Type: DefaultIPAM,
		},
	}
}

type Handler struct {
	ctx                               context.Context
	settings                          ctlharvesterv1.SettingClient
	longhornSettings                  ctllonghornv1.SettingClient
	longhornSettingCache              ctllonghornv1.SettingCache
	longhornVolumeCache               ctllonghornv1.VolumeCache
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
}

// register the setting controller and reconsile longhorn setting when storage network changed
func Register(ctx context.Context, management *config.Management, opts config.Options) error {
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	longhornSettings := management.LonghornFactory.Longhorn().V1beta1().Setting()
	longhornVolumes := management.LonghornFactory.Longhorn().V1beta1().Volume()
	prometheus := management.MonitoringFactory.Monitoring().V1().Prometheus()
	alertmanager := management.MonitoringFactory.Monitoring().V1().Alertmanager()
	deployments := management.AppsFactory.Apps().V1().Deployment()
	managedCharts := management.RancherManagementFactory.Management().V3().ManagedChart()
	networkAttachmentDefinitions := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()

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
func (h *Handler) OnStorageNetworkChange(key string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
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

	currentNad := setting.Annotations[NadStorageNetworkAnnotation]
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
	nadName := settingCopy.Annotations[NadStorageNetworkAnnotation]
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
	savedHash := setting.Annotations[HashStorageNetworkAnnotation]
	return currentHash == savedHash
}

func (h *Handler) setHashAnnotations(setting *harvesterv1.Setting) *harvesterv1.Setting {
	setting.Annotations[HashStorageNetworkAnnotation] = h.sha1(setting.Value)
	return setting
}

func (h *Handler) setNadAnnotations(setting *harvesterv1.Setting, newNad string) *harvesterv1.Setting {
	setting.Annotations[OldNadStorageNetworkAnnotation] = setting.Annotations[NadStorageNetworkAnnotation]
	setting.Annotations[NadStorageNetworkAnnotation] = newNad
	return setting
}

func (h *Handler) createNad(setting *harvesterv1.Setting) (string, error) {
	var config Config
	bridgeConfig := NewBridgeConfig()

	if err := json.Unmarshal([]byte(setting.Value), &config); err != nil {
		return "", fmt.Errorf("parsing value error %v", err)
	}

	bridgeConfig.Bridge = config.ClusterNetwork + BridgeSuffix
	bridgeConfig.IPAM.Range = config.Range

	if config.Vlan == 0 {
		config.Vlan = DefaultPVID
	}
	bridgeConfig.Vlan = config.Vlan

	if len(config.Exclude) > 0 {
		bridgeConfig.IPAM.Exclude = config.Exclude
	}

	nadConfig, err := json.Marshal(bridgeConfig)
	if err != nil {
		return "", fmt.Errorf("output json error %v", err)
	}

	nad := nadv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: StorageNetworkNetAttachDefPrefix,
			Namespace:    StorageNetworkNetAttachDefNamespace,
		},
	}
	nad.Annotations = map[string]string{
		StorageNetworkAnnotation: "true",
	}
	nad.Spec.Config = string(nadConfig)

	// create nad
	var nadResult *nadv1.NetworkAttachmentDefinition
	if nadResult, err = h.networkAttachmentDefinitions.Create(&nad); err != nil {
		return "", fmt.Errorf("create net-attach-def failed %v", err)
	}

	return fmt.Sprintf("%s/%s", nadResult.Namespace, nadResult.Name), nil
}

func (h *Handler) checkValueIsChanged(setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	var updatedSetting *harvesterv1.Setting
	var err error
	nadName := ""

	if h.checkIsSameHashValue(setting) {
		return setting, nil
	}

	if setting.Value != "" {
		nadName, err = h.createNad(setting)
		if err != nil {
			return setting, err
		}
	}

	setting = h.setNadAnnotations(setting, nadName)
	setting = h.setHashAnnotations(setting)

	if updatedSetting, err = h.setConfiguredCondition(setting, false, ReasonInProgress, "create NAD"); err != nil {
		return setting, fmt.Errorf("create nad update status error %v", err)
	}
	return updatedSetting, fmt.Errorf("check hash again")
}

func (h *Handler) removeOldNad(setting *harvesterv1.Setting) error {
	oldNad := setting.Annotations[OldNadStorageNetworkAnnotation]
	if oldNad == "" {
		return nil
	}

	nadName := strings.Split(oldNad, "/")
	if len(nadName) != 2 {
		logrus.Errorf("split nad namespace and name failed %s", oldNad)
		setting.Annotations[OldNadStorageNetworkAnnotation] = ""
		return nil
	}
	namespace := nadName[0]
	name := nadName[1]

	if _, err := h.networkAttachmentDefinitionsCache.Get(namespace, name); err != nil {
		if apierrors.IsNotFound(err) {
			setting.Annotations[OldNadStorageNetworkAnnotation] = ""
			return nil
		}

		// retry again
		return fmt.Errorf("check net-attach-def existing error %v", err)
	}

	if err := h.networkAttachmentDefinitions.Delete(namespace, name, &metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("remove nad error %v", err)
	}

	setting.Annotations[OldNadStorageNetworkAnnotation] = ""
	return nil
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
		return fmt.Errorf(MsgWaitForVolumes, strings.Join(attachedVolume, ","))
	}

	return nil
}

func (h *Handler) checkPrometheusStatusAndStart() error {
	// check prometheus cattle-monitoring-system/rancher-monitoring-prometheus replica
	prometheus, err := h.prometheusCache.Get(CattleMonitoringSystemNamespace, RancherMonitoringPrometheus)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("prometheus not found. skip")
			return nil
		}
		return fmt.Errorf("prometheus get error %v", err)
	}

	// check started or not
	if replicasStr, ok := prometheus.Annotations[ReplicaStorageNetworkAnnotation]; ok {
		logrus.Infof("current prometheus replicas: %v", *prometheus.Spec.Replicas)
		logrus.Infof("start prometheus")
		prometheusCopy := prometheus.DeepCopy()
		replicas, err := strconv.ParseInt(replicasStr, 10, 32)
		if err != nil {
			return fmt.Errorf("strconv ParseInt error %v", err)
		}
		*prometheusCopy.Spec.Replicas = int32(replicas)
		delete(prometheusCopy.Annotations, ReplicaStorageNetworkAnnotation)

		if _, err := h.prometheus.Update(prometheusCopy); err != nil {
			return fmt.Errorf("prometheus update error %v", err)
		}
		return nil
	}

	return nil
}

func (h *Handler) checkAlertmanagerStatusAndStart() error {
	// check alertmanager cattle-monitoring-system/rancher-monitoring-alertmanager replica
	alertmanager, err := h.alertmanagerCache.Get(CattleMonitoringSystemNamespace, RancherMonitoringAlertmanager)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("Alertmanager not found. skip")
			return nil
		}
		return fmt.Errorf("alertmanager get error %v", err)
	}

	// check started or not
	if replicasStr, ok := alertmanager.Annotations[ReplicaStorageNetworkAnnotation]; ok {
		logrus.Infof("current alertmanager replicas: %v", *alertmanager.Spec.Replicas)
		logrus.Infof("start alertmanager")
		alertmanagerCopy := alertmanager.DeepCopy()
		replicas, err := strconv.ParseInt(replicasStr, 10, 32)
		if err != nil {
			return fmt.Errorf("strconv ParseInt error %v", err)
		}
		*alertmanagerCopy.Spec.Replicas = int32(replicas)
		delete(alertmanagerCopy.Annotations, ReplicaStorageNetworkAnnotation)

		if _, err := h.alertmanager.Update(alertmanagerCopy); err != nil {
			return fmt.Errorf("alertmanager update error %v", err)
		}
		return nil
	}

	return nil
}

func (h *Handler) checkGrafanaStatusAndStart() error {
	// check deployment cattle-monitoring-system/rancher-monitoring-grafana replica
	grafana, err := h.deploymentCache.Get(CattleMonitoringSystemNamespace, RancherMonitoringGrafana)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("grafana not found. skip")
			return nil
		}
		return fmt.Errorf("grafana get error %v", err)
	}

	// check started or not
	if replicasStr, ok := grafana.Annotations[ReplicaStorageNetworkAnnotation]; ok {
		logrus.Infof("current Grafana replicas: %v", *grafana.Spec.Replicas)
		logrus.Infof("start grafana")
		grafanaCopy := grafana.DeepCopy()
		replicas, err := strconv.ParseInt(replicasStr, 10, 32)
		if err != nil {
			return fmt.Errorf("strconv ParseInt error %v", err)
		}
		*grafanaCopy.Spec.Replicas = int32(replicas)
		delete(grafanaCopy.Annotations, ReplicaStorageNetworkAnnotation)

		if _, err := h.deployments.Update(grafanaCopy); err != nil {
			return fmt.Errorf("Grafana update error %v", err)
		}
		return nil
	}

	return nil
}

func (h *Handler) checkRancherMonitoringStatusAndStart() error {
	// check managedchart fleet-local/rancher-monitoring paused
	monitoring, err := h.managedChartCache.Get(FleetLocalNamespace, RancherMonitoring)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("rancher monitoring not found. skip")
			return nil
		}
		return fmt.Errorf("rancher monitoring get error %v", err)
	}

	// check pause or not
	if _, ok := monitoring.Annotations[PausedStorageNetworkAnnotation]; ok {
		logrus.Infof("current Rancher Monitoring paused: %v", monitoring.Spec.Paused)
		logrus.Infof("start rancher monitoring")
		monitoringCopy := monitoring.DeepCopy()
		monitoringCopy.Spec.Paused = false
		delete(monitoringCopy.Annotations, PausedStorageNetworkAnnotation)

		if _, err := h.managedCharts.Update(monitoringCopy); err != nil {
			return fmt.Errorf("rancher monitoring error %v", err)
		}
		return nil
	}

	return nil
}

func (h *Handler) checkVMImportControllerStatusAndStart() error {
	// check deployment harvester-system/harvester-harvester-vm-import-controller replica
	vmImportControllerDeploy, err := h.deploymentCache.Get(HarvesterSystemNamespace, HarvesterVMImportController)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("VM import controller not found. skip")
			return nil
		}
		return fmt.Errorf("vm import controller get error %v", err)
	}

	logrus.Infof("current VM Import Controller replicas: %v", *vmImportControllerDeploy.Spec.Replicas)
	// check started or not
	if replicasStr, ok := vmImportControllerDeploy.Annotations[ReplicaStorageNetworkAnnotation]; ok {
		logrus.Infof("start vm import controller")
		vmImportControllerDeployCopy := vmImportControllerDeploy.DeepCopy()
		replicas, err := strconv.ParseInt(replicasStr, 10, 32)
		if err != nil {
			return fmt.Errorf("strconv ParseInt error %v", err)
		}
		*vmImportControllerDeployCopy.Spec.Replicas = int32(replicas)
		delete(vmImportControllerDeployCopy.Annotations, ReplicaStorageNetworkAnnotation)

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
	monitoring, err := h.managedChartCache.Get(FleetLocalNamespace, RancherMonitoring)
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
		monitoringCopy.Annotations[PausedStorageNetworkAnnotation] = "false"
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
	prometheus, err := h.prometheusCache.Get(CattleMonitoringSystemNamespace, RancherMonitoringPrometheus)
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
		prometheusCopy.Annotations[ReplicaStorageNetworkAnnotation] = strconv.Itoa(int(*prometheus.Spec.Replicas))
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
	alertmanager, err := h.alertmanagerCache.Get(CattleMonitoringSystemNamespace, RancherMonitoringAlertmanager)
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
		alertmanagerCopy.Annotations[ReplicaStorageNetworkAnnotation] = strconv.Itoa(int(*alertmanager.Spec.Replicas))
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
	grafana, err := h.deploymentCache.Get(CattleMonitoringSystemNamespace, RancherMonitoringGrafana)
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
		grafanaCopy.Annotations[ReplicaStorageNetworkAnnotation] = strconv.Itoa(int(*grafana.Spec.Replicas))
		*grafanaCopy.Spec.Replicas = 0

		if _, err := h.deployments.Update(grafanaCopy); err != nil {
			return fmt.Errorf("Grafana update error %v", err)
		}
		return nil
	}

	return nil
}

func (h *Handler) checkVMImportControllerStatusAndStop() error {
	// check deployment harvester-system/harvester-harvester-vm-import-controller replica
	vmimportcontroller, err := h.deploymentCache.Get(HarvesterSystemNamespace, HarvesterVMImportController)
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
		vmimportcontrollerCopy.Annotations[ReplicaStorageNetworkAnnotation] = strconv.Itoa(int(*vmimportcontroller.Spec.Replicas))
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
