package v13xto140

import (
	"context"

	"github.com/pkg/errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
)

const (
	upgradeLogPrefix = "upgrade from v1.3.x to v1.4.0: "
)

func UpgradeResources(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset) error {
	if err := upgradeVolumes(namespace, lhClient); err != nil {
		return err
	}
	return nil
}

func upgradeVolumes(namespace string, lhClient *lhclientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade volume failed")
	}()

	volumes, err := lhClient.LonghornV1beta2().Volumes(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to list all existing Longhorn volumes during the volume upgrade")
	}

	requireUpdate := false
	for _, v := range volumes.Items {
		requireUpdate = false
		if v.Spec.SnapshotDataIntegrity == "" {
			v.Spec.SnapshotDataIntegrity = longhorn.SnapshotDataIntegrityIgnored
			requireUpdate = true
		}
		if v.Spec.RestoreVolumeRecurringJob == "" {
			v.Spec.RestoreVolumeRecurringJob = longhorn.RestoreVolumeRecurringJobDefault
			requireUpdate = true
		}
		if v.Spec.UnmapMarkSnapChainRemoved == "" {
			v.Spec.UnmapMarkSnapChainRemoved = longhorn.UnmapMarkSnapChainRemovedIgnored
			requireUpdate = true
		}

		if !requireUpdate {
			continue
		}
		if _, err = lhClient.LonghornV1beta2().Volumes(namespace).Update(context.TODO(), &v, metav1.UpdateOptions{}); err != nil {
			return errors.Wrapf(err, "failed to update volume %v", v.Name)
		}
	}

	return nil
}
