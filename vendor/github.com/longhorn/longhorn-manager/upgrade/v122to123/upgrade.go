package v122to123

import (
	"context"
	"reflect"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/longhorn/longhorn-manager/engineapi"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	"github.com/longhorn/longhorn-manager/types"
)

const (
	upgradeLogPrefix = "upgrade from v1.2.2 to v1.2.3: "
)

func UpgradeResources(namespace string, lhClient *lhclientset.Clientset) (err error) {
	if err := upgradeBackups(namespace, lhClient); err != nil {
		return err
	}
	if err := upgradeEngines(namespace, lhClient); err != nil {
		return err
	}
	return nil
}

func upgradeBackups(namespace string, lhClient *lhclientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade backups failed")
	}()

	// Copy backupStatus from engine CRs to backup CRs
	backups, err := lhClient.LonghornV1beta1().Backups(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	// Loop all the backup CRs
	for _, backup := range backups.Items {
		// Get volume name from label
		volumeName, exist := backup.Labels[types.LonghornLabelBackupVolume]
		if !exist {
			continue
		}

		// Get the corresponding volume's engine
		engines, err := lhClient.LonghornV1beta1().Engines(namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: types.LonghornLabelVolume + "=" + volumeName,
		})
		if err != nil {
			return err
		}

		// No engine CR found
		var engine longhorn.Engine
		switch len(engines.Items) {
		case 0:
			continue
		case 1:
			engine = engines.Items[0]
		default:
			// If more than 2 engines found, use the current volume's engine
			v, err := lhClient.LonghornV1beta1().Volumes(namespace).Get(context.TODO(), volumeName, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					// Cannot found the correspoding volume
					continue
				}
				return err
			}

			for _, e := range engines.Items {
				if e.Spec.NodeID == v.Status.CurrentNodeID &&
					e.Spec.DesireState == longhorn.InstanceStateRunning &&
					e.Status.CurrentState == longhorn.InstanceStateRunning {
					engine = e
					break
				}
			}
		}

		// No corresponding backupStatus inside engine CR
		backupStatus, exist := engine.Status.BackupStatus[backup.Name]
		if !exist {
			continue
		}

		existingBackup := backup.DeepCopy()

		backup.Status.Progress = backupStatus.Progress
		backup.Status.URL = backupStatus.BackupURL
		backup.Status.Error = backupStatus.Error
		backup.Status.SnapshotName = backupStatus.SnapshotName
		backup.Status.State = engineapi.ConvertEngineBackupState(backupStatus.State)
		backup.Status.ReplicaAddress = backupStatus.ReplicaAddress

		if reflect.DeepEqual(existingBackup.Status, backup.Status) {
			continue
		}
		if _, err = lhClient.LonghornV1beta1().Backups(namespace).UpdateStatus(context.TODO(), &backup, metav1.UpdateOptions{}); err != nil && !apierrors.IsConflict(errors.Cause(err)) {
			return err
		}
	}
	return nil
}

func upgradeEngines(namespace string, lhClient *lhclientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade engines failed")
	}()

	// Do the field update separately to avoid messing up.

	if err := checkAndRemoveEngineBackupStatus(namespace, lhClient); err != nil {
		return err
	}

	if err := checkAndUpdateEngineActiveState(namespace, lhClient); err != nil {
		return err
	}

	return nil
}

func checkAndRemoveEngineBackupStatus(namespace string, lhClient *lhclientset.Clientset) error {
	engines, err := lhClient.LonghornV1beta1().Engines(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, engine := range engines.Items {
		existingEngine := engine.DeepCopy()

		engine.Status.BackupStatus = nil

		if reflect.DeepEqual(existingEngine.Status, engine.Status) {
			continue
		}
		if _, err := lhClient.LonghornV1beta1().Engines(namespace).UpdateStatus(context.TODO(), &engine, metav1.UpdateOptions{}); err != nil && !apierrors.IsConflict(errors.Cause(err)) {
			return err
		}
	}

	return nil
}

func checkAndUpdateEngineActiveState(namespace string, lhClient *lhclientset.Clientset) error {
	engines, err := lhClient.LonghornV1beta1().Engines(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	volumeEngineMap := map[string][]*longhorn.Engine{}
	for i := range engines.Items {
		e := &engines.Items[i]
		if e.Spec.VolumeName == "" {
			// Cannot do anything in the upgrade path if there is really an orphan engine CR.
			continue
		}
		volumeEngineMap[e.Spec.VolumeName] = append(volumeEngineMap[e.Spec.VolumeName], e)
	}

	for volumeName, engineList := range volumeEngineMap {
		skip := false
		for _, e := range engineList {
			if e.Spec.Active {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		var currentEngine *longhorn.Engine
		if len(engineList) == 1 {
			currentEngine = engineList[0]
		} else {
			v, err := lhClient.LonghornV1beta1().Volumes(namespace).Get(context.TODO(), volumeName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			for i := range engineList {
				if (v.Spec.NodeID != "" && v.Spec.NodeID == engineList[i].Spec.NodeID) ||
					(v.Status.CurrentNodeID != "" && v.Status.CurrentNodeID == engineList[i].Spec.NodeID) ||
					(v.Status.PendingNodeID != "" && v.Status.PendingNodeID == engineList[i].Spec.NodeID) {
					currentEngine = engineList[i]
					break
				}
			}
		}
		if currentEngine == nil {
			logrus.Errorf("failed to get the current engine for volume %v during upgrade, will ignore it and continue", volumeName)
			continue
		}
		currentEngine.Spec.Active = true
		if _, err := lhClient.LonghornV1beta1().Engines(namespace).Update(context.TODO(), currentEngine, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}

	return nil
}
