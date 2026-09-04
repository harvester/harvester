package sharemanager

import (
	"encoding/json"
	"fmt"
	"reflect"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

func (h *Handler) OnRWXNetworkChange(_ string, setting *harvesterv1beta1.Setting) (*harvesterv1beta1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil || setting.Name != settings.RWXNetworkSettingName {
		return setting, nil
	}

	currentSrcNADRef, oldSrcNADRef, err := h.getSrcNADRefs(setting)
	if err != nil {
		return setting, err
	}

	if err := h.ensureDerivedNADs(currentSrcNADRef); err != nil {
		return setting, err
	}
	if err := h.removeDerivedNADs(
		oldSrcNADRef,
		currentSrcNADRef,
	); err != nil {
		return setting, err
	}
	if err := h.enqueueStaticIPShareManagers(); err != nil {
		return setting, err
	}

	return setting, nil
}

func (h *Handler) ensureDerivedNADs(sourceNADRef string) error {
	if sourceNADRef == "" || h.nads == nil {
		return nil
	}

	sourceNAD, err := h.getNAD(sourceNADRef)
	if err != nil {
		return err
	}

	staticNAD, err := newStaticNAD(sourceNAD)
	if err != nil {
		return err
	}

	return h.ensureNAD(staticNAD)
}

func (h *Handler) getSrcNADRefs(setting *harvesterv1beta1.Setting) (string, string, error) {
	current, err := h.getCurrentSourceNADRef(setting)
	if err != nil {
		return "", "", err
	}
	return current, oldSrcNADRef(setting), nil
}

func (h *Handler) getCurrentSourceNADRef(setting *harvesterv1beta1.Setting) (string, error) {
	if setting == nil || setting.EffectiveValue() == "" {
		return "", nil
	}

	rwxConfig, err := settings.GetRWXNetworkConfig(setting)
	if err != nil {
		return "", err
	}
	if rwxConfig.ShareStorageNetwork {
		return h.getStorageNetworkNADRef()
	}

	return setting.Annotations[util.RWXNadNetworkAnnotation], nil
}

func (h *Handler) getStorageNetworkNADRef() (string, error) {
	storageSetting, err := h.getStorageNetworkSetting()
	if err != nil {
		return "", err
	}
	if storageSetting == nil {
		return "", nil
	}
	return storageSetting.Annotations[util.NadStorageNetworkAnnotation], nil
}

func oldSrcNADRef(setting *harvesterv1beta1.Setting) string {
	if old := setting.Annotations[util.RWXOldNadNetworkAnnotation]; old != "" {
		return old
	}
	return setting.Annotations[util.RWXNadNetworkAnnotation]
}

func (h *Handler) getStorageNetworkSetting() (*harvesterv1beta1.Setting, error) {
	if h.settingsCache == nil {
		return nil, nil
	}

	setting, err := h.settingsCache.Get(settings.StorageNetworkName)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return setting, nil
}

func (h *Handler) removeDerivedNADs(oldSrcNADRef, currentSrcNADRef string) error {
	if oldSrcNADRef == "" || oldSrcNADRef == currentSrcNADRef {
		return nil
	}

	nadNamespace, nadName, err := parseNADNamespacedName(oldSrcNADRef)
	if err != nil {
		return err
	}
	if nadNamespace == "" {
		nadNamespace = util.StorageNetworkNetAttachDefNamespace
	}

	staticNADName := staticNADNameBySourceName(nadName)

	if err := h.nads.Delete(nadNamespace, staticNADName, &metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("remove static nad error %v", err)
		}
	}

	return nil
}

func (h *Handler) ensureNAD(nad *networkv1.NetworkAttachmentDefinition) error {
	existing, err := h.nadCache.Get(nad.Namespace, nad.Name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		_, err = h.nads.Create(nad)
		return err
	}

	if existing.Spec.Config == nad.Spec.Config && reflect.DeepEqual(existing.Labels, nad.Labels) {
		return nil
	}

	nadCopy := existing.DeepCopy()
	nadCopy.Labels = nad.Labels
	nadCopy.Spec = nad.Spec
	_, err = h.nads.Update(nadCopy)
	return err
}

func staticNADNameBySourceName(sourceNADName string) string {
	return sourceNADName + "-static"
}

func newStaticNAD(sourceNAD *networkv1.NetworkAttachmentDefinition) (*networkv1.NetworkAttachmentDefinition, error) {
	config := map[string]interface{}{}
	if err := json.Unmarshal([]byte(sourceNAD.Spec.Config), &config); err != nil {
		return nil, fmt.Errorf("failed to decode NAD %s/%s config: %w", sourceNAD.Namespace, sourceNAD.Name, err)
	}
	config["ipam"] = map[string]string{"type": "static"}

	labelsCopy := map[string]string{}
	for key, value := range sourceNAD.Labels {
		if key == util.HashStorageNetworkAnnotation || key == util.RWXHashNetworkAnnotation {
			continue
		}
		labelsCopy[key] = value
	}

	configBytes, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to encode static NAD config for %s/%s: %w", sourceNAD.Namespace, sourceNAD.Name, err)
	}

	return &networkv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      staticNADNameBySourceName(sourceNAD.Name),
			Namespace: sourceNAD.Namespace,
			Labels:    labelsCopy,
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: string(configBytes),
		},
	}, nil
}
