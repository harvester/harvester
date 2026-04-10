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

func (h *Handler) reconcileDerivedNetworkResources(setting *harvesterv1beta1.Setting) (*harvesterv1beta1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil || setting.Name != settings.RWXNetworkSettingName {
		return setting, nil
	}

	currentSourceNADRef, oldSourceNADRef, err := h.getDerivedSourceNADRefs(setting)
	if err != nil {
		return setting, err
	}

	if err := h.ensureDerivedNetworkResources(currentSourceNADRef); err != nil {
		return setting, err
	}
	if err := h.removeDerivedNetworkResources(
		oldSourceNADRef,
		currentSourceNADRef,
	); err != nil {
		return setting, err
	}
	if err := h.enqueueStaticIPShareManagers(); err != nil {
		return setting, err
	}

	return setting, nil
}

func (h *Handler) OnRWXNetworkChange(_ string, setting *harvesterv1beta1.Setting) (*harvesterv1beta1.Setting, error) {
	return h.reconcileDerivedNetworkResources(setting)
}

func (h *Handler) ensureDerivedNetworkResources(sourceNADRef string) error {
	if sourceNADRef == "" || h.nads == nil || h.ipPoolUsages == nil {
		return nil
	}

	sourceNAD, err := h.getSourceNAD(sourceNADRef)
	if err != nil {
		return err
	}

	excludeCIDR, err := getSingleExcludeRange(sourceNAD)
	if err != nil {
		return err
	}
	if excludeCIDR == "" {
		return h.removeDerivedNetworkResources(sourceNADRef, "")
	}

	staticNAD, err := newStaticNAD(sourceNAD)
	if err != nil {
		return err
	}
	if err := h.ensureStaticNAD(staticNAD); err != nil {
		return err
	}
	return h.ensureExcludeIPPoolUsage(newExcludeIPPoolUsage(sourceNAD, excludeCIDR))
}

func (h *Handler) getDerivedSourceNADRefs(setting *harvesterv1beta1.Setting) (string, string, error) {
	rwxConfig, err := settings.GetRWXNetworkConfig(setting)
	if err != nil {
		return "", "", err
	}
	if rwxConfig.ShareStorageNetwork {
		current, err := h.getSharedSourceNADRef()
		if err != nil {
			return "", "", err
		}
		return current, oldDerivedSourceNADRef(setting), nil
	}

	return setting.Annotations[util.RWXNadNetworkAnnotation], setting.Annotations[util.RWXOldNadNetworkAnnotation], nil
}

func (h *Handler) getSharedSourceNADRef() (string, error) {
	storageSetting, err := h.getStorageNetworkSetting()
	if err != nil {
		return "", err
	}
	if storageSetting == nil {
		return "", nil
	}
	return storageSetting.Annotations[util.NadStorageNetworkAnnotation], nil
}

func oldDerivedSourceNADRef(setting *harvesterv1beta1.Setting) string {
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
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return setting, nil
}

func (h *Handler) removeDerivedNetworkResources(sourceNADRef, currentSourceNADRef string) error {
	if sourceNADRef == "" || sourceNADRef == currentSourceNADRef {
		return nil
	}

	nadNamespace, nadName, err := parseNADNamespacedName(sourceNADRef)
	if err != nil {
		return err
	}
	if nadNamespace == "" {
		nadNamespace = util.StorageNetworkNetAttachDefNamespace
	}

	staticNADName := staticNADNameBySourceName(nadName)
	excludePoolName := excludeIPPoolUsageNameBySource(nadNamespace, nadName)

	if err := h.nads.Delete(nadNamespace, staticNADName, &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("remove static nad error %v", err)
	}
	if err := h.ipPoolUsages.Delete(excludePoolName, &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("remove exclude ip pool usage error %v", err)
	}

	return nil
}

func (h *Handler) ensureStaticNAD(nad *networkv1.NetworkAttachmentDefinition) error {
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

func (h *Handler) ensureExcludeIPPoolUsage(pool *harvesterv1beta1.IPPoolUsage) error {
	existing, err := h.ipPoolUsageCache.Get(pool.Name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		_, err = h.ipPoolUsages.Create(pool)
		return err
	}

	if existing.Spec.CIDR == pool.Spec.CIDR && existing.Spec.ReservedIPCount == pool.Spec.ReservedIPCount {
		return nil
	}

	poolCopy := existing.DeepCopy()
	poolCopy.Spec = pool.Spec
	_, err = h.ipPoolUsages.Update(poolCopy)
	return err
}

func staticNADNameBySourceName(sourceNADName string) string {
	return sourceNADName + "-static"
}

func excludeIPPoolUsageNameBySource(namespace, sourceNADName string) string {
	return fmt.Sprintf("%s-%s-exclude", namespace, sourceNADName)
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

func newExcludeIPPoolUsage(
	sourceNAD *networkv1.NetworkAttachmentDefinition,
	excludeCIDR string,
) *harvesterv1beta1.IPPoolUsage {
	return &harvesterv1beta1.IPPoolUsage{
		ObjectMeta: metav1.ObjectMeta{
			Name: excludeIPPoolUsageName(sourceNAD),
		},
		Spec: harvesterv1beta1.IPPoolUsageSpec{
			CIDR:            excludeCIDR,
			ReservedIPCount: util.IPPoolUsageDefaultReservedIPCount,
		},
	}
}
