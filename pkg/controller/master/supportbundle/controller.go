package supportbundle

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	catalogv1 "github.com/rancher/rancher/pkg/generated/controllers/catalog.cattle.io/v1"
	mgmtv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	ctlappsv1 "github.com/rancher/wrangler/pkg/generated/controllers/apps/v1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	settingctl "github.com/harvester/harvester/pkg/controller/master/setting"
	"github.com/harvester/harvester/pkg/controller/master/supportbundle/types"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
)

// Handler generates support bundles for the cluster
type Handler struct {
	supportBundles          v1beta1.SupportBundleClient
	supportBundleController v1beta1.SupportBundleController
	nodeCache               ctlcorev1.NodeCache
	podCache                ctlcorev1.PodCache
	deployments             ctlappsv1.DeploymentClient
	daemonSets              ctlappsv1.DaemonSetClient
	services                ctlcorev1.ServiceClient
	appCache                catalogv1.AppCache
	managedChartCache       mgmtv3.ManagedChartCache
	settings                v1beta1.SettingClient
	settingCache            v1beta1.SettingCache

	manager *Manager
}

func (h *Handler) OnSupportBundleChanged(key string, sb *harvesterv1.SupportBundle) (*harvesterv1.SupportBundle, error) {
	if sb == nil || sb.DeletionTimestamp != nil {
		return sb, nil
	}

	switch sb.Status.State {
	case types.StateNone:
		logrus.Debugf("[%s] generating a support bundle", sb.Name)

		imageStr := settings.SupportBundleImage.Get()
		if imageStr == "{}" || imageStr == "" {
			err := h.updateSupportBundleImageSetting()
			if err != nil {
				return nil, err
			}
			h.supportBundleController.Enqueue(sb.Namespace, sb.Name)
			return nil, nil
		}

		var image settings.Image
		err := json.Unmarshal([]byte(imageStr), &image)
		if err != nil {
			return h.setError(sb, fmt.Sprintf("fail to parse support bundle image for %s: %s", sb.Name, err))
		}
		logrus.Debugf("[%s] support bundle image: %+v", sb.Name, image)

		err = h.manager.Create(sb, fmt.Sprintf("%s:%s", image.Repository, image.Tag), image.ImagePullPolicy)
		if err != nil {
			return h.setError(sb, fmt.Sprintf("fail to create manager for %s: %s", sb.Name, err))
		}
		return h.setState(sb, types.StateGenerating)
	case types.StateGenerating:
		logrus.Debugf("[%s] support bundle is being generated", sb.Name)
		return h.checkManagerStatus(sb)
	default:
		logrus.Debugf("[%s] noop for state %s", sb.Name, sb.Status.State)
		return sb, nil
	}
}

func (h *Handler) updateSupportBundleImageSetting() error {
	harvesterManagedChart, err := h.managedChartCache.Get(settingctl.ManagedChartNamespace, settingctl.HarvesterManagedChartName)
	if err != nil {
		return err
	}

	app, err := h.appCache.Get(harvesterManagedChart.Spec.DefaultNamespace, harvesterManagedChart.Spec.ReleaseName)
	if err != nil {
		return err
	}

	return settingctl.UpdateSupportBundleImage(h.settings, h.settingCache, app)
}

func (h *Handler) checkManagerStatus(sb *harvesterv1.SupportBundle) (*harvesterv1.SupportBundle, error) {
	var timeout int
	var err error
	if timeoutStr := settings.SupportBundleTimeout.Get(); timeoutStr != "" {
		if timeout, err = strconv.Atoi(timeoutStr); err != nil {
			return nil, err
		}
	}

	if timeout != 0 && time.Now().After(sb.CreationTimestamp.Add(time.Duration(timeout)*time.Minute)) {
		return h.setError(sb, "fail to generate supportbundle: timeout")
	}

	managerStatus, err := h.manager.GetStatus(sb)
	if err != nil {
		logrus.Debugf("[%s] manager pod is not ready: %s", sb.Name, err)
		h.supportBundleController.EnqueueAfter(sb.Namespace, sb.Name, time.Second*3)
		return sb, nil
	}

	if managerStatus.Error {
		return h.setError(sb, managerStatus.ErrorMessage)
	}

	switch managerStatus.Progress {
	case 100:
		return h.setReady(sb, managerStatus.Filename, managerStatus.Filesize)
	default:
		if managerStatus.Progress == sb.Status.Progress {
			h.supportBundleController.EnqueueAfter(sb.Namespace, sb.Name, time.Second*5)
			return sb, nil
		}
		return h.setProgress(sb, managerStatus.Progress)
	}
}

func (h *Handler) setError(sb *harvesterv1.SupportBundle, reason string) (*harvesterv1.SupportBundle, error) {
	logrus.Errorf("[%s] set state to error: %s", sb.Name, reason)
	toUpdate := sb.DeepCopy()
	harvesterv1.SupportBundleInitialized.False(toUpdate)
	harvesterv1.SupportBundleInitialized.Message(toUpdate, reason)
	toUpdate.Status.State = types.StateError
	return h.supportBundles.Update(toUpdate)
}

func (h *Handler) setState(sb *harvesterv1.SupportBundle, state string) (*harvesterv1.SupportBundle, error) {
	logrus.Debugf("[%s] set state to %s", sb.Name, state)
	toUpdate := sb.DeepCopy()
	toUpdate.Status.State = state
	return h.supportBundles.Update(toUpdate)
}

func (h *Handler) setReady(sb *harvesterv1.SupportBundle, filename string, filesize int64) (*harvesterv1.SupportBundle, error) {
	logrus.Debugf("[%s] set state to %s", sb.Name, types.StateReady)
	toUpdate := sb.DeepCopy()
	harvesterv1.SupportBundleInitialized.True(toUpdate)
	toUpdate.Status.State = types.StateReady
	toUpdate.Status.Progress = 100
	toUpdate.Status.Filename = filename
	toUpdate.Status.Filesize = filesize
	return h.supportBundles.Update(toUpdate)
}

func (h *Handler) setProgress(sb *harvesterv1.SupportBundle, progress int) (*harvesterv1.SupportBundle, error) {
	logrus.Debugf("[%s] set progress to %d", sb.Name, progress)
	toUpdate := sb.DeepCopy()
	toUpdate.Status.Progress = progress
	return h.supportBundles.Update(toUpdate)
}
