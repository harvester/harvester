package kubevirt

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/network"
)

const (
	ControllerName = "harvester-vm-migration-network-controller"

	VMMigrationNetworkPrefix         = "vm-migration-network.settings.harvesterhci.io"
	VMMigrationNetworkHashLabel      = VMMigrationNetworkPrefix + "/hash"
	VMMigrationNetworkHashAnnotation = VMMigrationNetworkHashLabel
	VMMigrationNetworkNadAnnotation  = VMMigrationNetworkPrefix + "/net-attach-def"

	KubeVirtName                         = "kubevirt"
	VMMigrationNetworkNetAttachDefPrefix = "vm-migration-network-"
)

type Handler struct {
	ctx                               context.Context
	settings                          ctlharvesterv1.SettingClient
	networkAttachmentDefinitions      ctlcniv1.NetworkAttachmentDefinitionClient
	networkAttachmentDefinitionsCache ctlcniv1.NetworkAttachmentDefinitionCache
	kubevirts                         ctlkubevirtv1.KubeVirtClient
	kubevirtCache                     ctlkubevirtv1.KubeVirtCache
}

// register the setting controller and reconsile longhorn setting when storage network changed
func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	networkAttachmentDefinitions := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()
	kubevirts := management.VirtFactory.Kubevirt().V1().KubeVirt()

	controller := &Handler{
		ctx:                               ctx,
		settings:                          settings,
		networkAttachmentDefinitions:      networkAttachmentDefinitions,
		networkAttachmentDefinitionsCache: networkAttachmentDefinitions.Cache(),
		kubevirts:                         kubevirts,
		kubevirtCache:                     kubevirts.Cache(),
	}

	settings.OnChange(ctx, ControllerName, controller.OnVMMigrationNetworkChange)
	return nil
}

func (h *Handler) setConfiguredCondition(setting *harvesterv1.Setting, finish bool, reason, msg, nadAnnotation string) (*harvesterv1.Setting, error) {
	settingCopy := setting.DeepCopy()
	if settingCopy.Annotations == nil {
		settingCopy.Annotations = make(map[string]string)
	}
	settingCopy.Annotations[VMMigrationNetworkNadAnnotation] = nadAnnotation
	settingCopy.Annotations[VMMigrationNetworkHashAnnotation] = h.sha1(setting.Value)

	harvesterv1.SettingConfigured.Reason(settingCopy, reason)
	harvesterv1.SettingConfigured.Message(settingCopy, msg)
	if finish {
		harvesterv1.SettingConfigured.True(settingCopy)
	} else {
		harvesterv1.SettingConfigured.False(settingCopy)
	}

	if !reflect.DeepEqual(settingCopy, setting) {
		s, err := h.settings.Update(settingCopy)
		if err != nil {
			return s, err
		}
		return s, nil
	}

	return setting, nil
}

func (h *Handler) OnVMMigrationNetworkChange(_ string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil || setting.Name != settings.VMMigrationNetworkSettingName {
		return setting, nil
	}

	if setting.Annotations == nil {
		if setting.Value == "" {
			// initialization case, don't update status, just skip it.
			return setting, nil
		}
		setting.Annotations = make(map[string]string)
	}

	if h.checkIsSameHashValue(setting) {
		return setting, nil
	}

	logrus.Infof("vm migration network change: %s", setting.Value)

	kubevirt, err := h.kubevirtCache.Get(util.HarvesterSystemNamespaceName, util.KubeVirtObjectName)
	if err != nil {
		return h.setConfiguredCondition(setting, false, "GetKubeVirtError", fmt.Sprintf("get kubevirt error %v", err), setting.Annotations[VMMigrationNetworkNadAnnotation])
	}
	kubevirtCopy := kubevirt.DeepCopy()
	if kubevirtCopy.Spec.Configuration.MigrationConfiguration == nil {
		kubevirtCopy.Spec.Configuration.MigrationConfiguration = &kubevirtv1.MigrationConfiguration{}
	}

	oldNadAnnotation := setting.Annotations[VMMigrationNetworkNadAnnotation]
	removeOldNad := true

	var nadAnnotation string
	if setting.Value != "" {
		nad, err := h.findOrCreateNad(setting)
		if err != nil {
			return h.setConfiguredCondition(setting, false, "CreateNadError", fmt.Sprintf("create nad error %v", err), oldNadAnnotation)
		}
		nadAnnotation = fmt.Sprintf("%s/%s", nad.Namespace, nad.Name)
		if oldNadAnnotation == nadAnnotation {
			removeOldNad = false
		}

		kubevirtCopy.Spec.Configuration.MigrationConfiguration.Network = &nad.Name
	} else {
		kubevirtCopy.Spec.Configuration.MigrationConfiguration.Network = nil
	}

	if !reflect.DeepEqual(kubevirt.Spec.Configuration, kubevirtCopy.Spec.Configuration) {
		if _, err = h.kubevirts.Update(kubevirtCopy); err != nil {
			return h.setConfiguredCondition(setting, false, "UpdateKubeVirtError", fmt.Sprintf("update kubevirt error %v", err), oldNadAnnotation)
		}
	}

	if removeOldNad {
		oldNadNamespaceAndName := strings.Split(oldNadAnnotation, "/")
		if len(oldNadNamespaceAndName) == 2 {
			if err = h.removeOldNad(oldNadNamespaceAndName[0], oldNadNamespaceAndName[1]); err != nil {
				return h.setConfiguredCondition(setting, false, "RemoveOldNadError", fmt.Sprintf("remove old nad error %v", err), oldNadAnnotation)
			}
		}
	}

	return h.setConfiguredCondition(setting, true, "", "", nadAnnotation)
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
	savedHash := setting.Annotations[VMMigrationNetworkHashLabel]
	return currentHash == savedHash
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
			GenerateName: VMMigrationNetworkNetAttachDefPrefix,
			Namespace:    util.HarvesterSystemNamespaceName,
		},
	}
	nad.Annotations = map[string]string{
		VMMigrationNetworkPrefix: "true",
	}
	nad.Labels = map[string]string{
		VMMigrationNetworkHashLabel: h.sha1(setting.Value),
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
	nads, err := h.networkAttachmentDefinitions.List(util.HarvesterSystemNamespaceName, metav1.ListOptions{
		LabelSelector: labels.Set{
			VMMigrationNetworkHashLabel: h.sha1(setting.Value),
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
			"sha1":     h.sha1(setting.Value),
			"count":    len(nads.Items),
			"nad.name": nads.Items[0].Name,
		}).Info("found more than one match nad")
	}

	return &nads.Items[0], nil
}

func (h *Handler) removeOldNad(namespace, name string) error {
	if _, err := h.networkAttachmentDefinitionsCache.Get(namespace, name); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		logrus.WithError(err).WithFields(logrus.Fields{
			"nad.namespace": namespace,
			"nad.name":      name,
		}).Warn("NAD not found")
		return err
	}

	if err := h.networkAttachmentDefinitions.Delete(namespace, name, &metav1.DeleteOptions{}); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"nad.namespace": namespace,
			"nad.name":      name,
		}).Warn("cannot delete nad")
		return err
	}
	return nil
}
