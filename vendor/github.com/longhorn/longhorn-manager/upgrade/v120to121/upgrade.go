package v120to121

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/longhorn/longhorn-manager/types"

	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
)

const (
	upgradeLogPrefix = "upgrade from v1.2.0 to v1.2.1: "
)

func UpgradeResources(namespace string, lhClient *lhclientset.Clientset) (err error) {
	if err := upgradeSettings(namespace, lhClient); err != nil {
		return err
	}
	return nil
}

func upgradeSettings(namespace string, lhClient *lhclientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade settings failed")
	}()

	// Skip the upgrade if this deprecated setting is unavailable.
	disableReplicaRebuildSetting, err := lhClient.LonghornV1beta2().Settings(namespace).Get(context.TODO(), string(types.SettingNameDisableReplicaRebuild), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	disabled, err := strconv.ParseBool(disableReplicaRebuildSetting.Value)
	if err != nil {
		logrus.Warnf(upgradeLogPrefix+"Failed to parse boolean value for setting %v, will skip checking it, error: %v", types.SettingNameDisableReplicaRebuild, err)
		return nil
	}
	if !disabled {
		return nil
	}

	// The rebuilding is already disabled. This new setting should be consistent with it.
	concurrentRebuildLimitSetting, err := lhClient.LonghornV1beta2().Settings(namespace).Get(context.TODO(), string(types.SettingNameConcurrentReplicaRebuildPerNodeLimit), metav1.GetOptions{})
	if err != nil {
		return err
	}
	concurrentRebuildLimit, err := strconv.ParseInt(concurrentRebuildLimitSetting.Value, 10, 32)
	if err != nil {
		return err
	}
	if concurrentRebuildLimit != 0 {
		concurrentRebuildLimitSetting.Value = "0"
		if _, err := lhClient.LonghornV1beta2().Settings(namespace).Update(context.TODO(), concurrentRebuildLimitSetting, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}

	// This old/deprecated setting can be unset or cleaned up now.
	disableReplicaRebuildSetting.Value = "false"
	if _, err := lhClient.LonghornV1beta2().Settings(namespace).Update(context.TODO(), disableReplicaRebuildSetting, metav1.UpdateOptions{}); err != nil {
		return err
	}

	return nil
}
