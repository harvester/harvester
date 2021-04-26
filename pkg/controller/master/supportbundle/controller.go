package supportbundle

import (
	"fmt"

	ctlappsv1 "github.com/rancher/wrangler/pkg/generated/controllers/apps/v1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
)

// Handler generates support bundles for the cluster
type Handler struct {
	supportBundles v1beta1.SupportBundleClient
	settingCache   v1beta1.SettingCache
	nodeCache      ctlcorev1.NodeCache
	deployments    ctlappsv1.DeploymentClient
	daemonSets     ctlappsv1.DaemonSetClient
	services       ctlcorev1.ServiceClient

	manager *Manager
}

func (h *Handler) OnSupportBundleChanged(key string, sb *harvesterv1.SupportBundle) (*harvesterv1.SupportBundle, error) {
	if sb == nil || sb.DeletionTimestamp != nil {
		return sb, nil
	}

	switch sb.Status.State {
	case StateNone:
		logrus.Debugf("[%s] generating a support bundle", sb.Name)
		err := h.manager.Create(sb, settings.SupportBundleImage.Get())
		toUpdate := sb.DeepCopy()
		if err != nil {
			h.setError(toUpdate, fmt.Sprintf("fail to create manager for %s: %s", sb.Name, err))
		} else {
			h.setState(toUpdate, StateGenerating)
		}
		return h.supportBundles.Update(toUpdate)
	default:
		logrus.Debugf("[%s] noop for state %s", sb.Name, sb.Status.State)
		return sb, nil
	}
}

func (h *Handler) setError(toUpdate *harvesterv1.SupportBundle, reason string) {
	logrus.Errorf(reason)
	harvesterv1.SupportBundleInitialized.False(toUpdate)
	harvesterv1.SupportBundleInitialized.Message(toUpdate, reason)

	toUpdate.Status.State = StateError
}

func (h *Handler) setState(toUpdate *harvesterv1.SupportBundle, state string) {
	logrus.Debugf("[%s] set state to %s", toUpdate.Name, state)

	if state == StateReady {
		logrus.Debugf("[%s] set condition %s to true", toUpdate.Name, harvesterv1.SupportBundleInitialized)
		harvesterv1.SupportBundleInitialized.True(toUpdate)
	}

	toUpdate.Status.State = state
}
