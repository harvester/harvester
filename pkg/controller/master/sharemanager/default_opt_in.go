package sharemanager

import (
	longhornv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const defaultShareManagerIface = "lhnet2"

func (h *Handler) OnShareManagerDefaultOptIn(_ string, m *longhornv1beta2.ShareManager) (*longhornv1beta2.ShareManager, error) {
	if m == nil || m.DeletionTimestamp != nil || m.Namespace != util.LonghornSystemNamespaceName {
		return m, nil
	}

	configured, err := h.isShareManagerNetworkConfigured()
	if err != nil || !configured {
		return m, err
	}

	mCpy := m.DeepCopy()
	if mCpy.Annotations == nil {
		mCpy.Annotations = map[string]string{}
	}

	mCpy.Annotations[util.ShareManagerStaticIPAnno] = "true"
	mCpy.Annotations[util.ShareManagerIfaceAnno] = defaultShareManagerIface

	if m.Annotations[util.ShareManagerStaticIPAnno] == "true" &&
		m.Annotations[util.ShareManagerIfaceAnno] == defaultShareManagerIface {
		return m, nil
	}

	return h.shareManagers.Update(mCpy)
}

func (h *Handler) OnRWXNetworkDefaultOptIn(_ string, setting *harvesterv1beta1.Setting) (*harvesterv1beta1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil {
		return setting, nil
	}
	if setting.Name != settings.RWXNetworkSettingName {
		return setting, nil
	}

	configured, err := h.isShareManagerNetworkConfigured()
	if err != nil || !configured {
		return setting, err
	}

	return setting, h.enqueueShareManagers()
}

func (h *Handler) enqueueShareManagers() error {
	if h.shareManagerCache == nil || h.shareManagerController == nil {
		return nil
	}

	shareManagers, err := h.shareManagerCache.List(util.LonghornSystemNamespaceName, labels.Everything())
	if err != nil {
		return err
	}
	for _, manager := range shareManagers {
		h.shareManagerController.Enqueue(manager.Namespace, manager.Name)
	}

	return nil
}
