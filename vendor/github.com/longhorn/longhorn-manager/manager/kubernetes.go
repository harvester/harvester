package manager

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/types"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func (m *VolumeManager) PVCreate(name, pvName, fsType, secretNamespace, secretName, storageClassName string) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to create PV for volume %v", name)
	}()

	v, err = m.ds.GetVolume(name)
	if err != nil {
		return nil, err
	}

	if v.Status.KubernetesStatus.PVName != "" {
		return v, fmt.Errorf("volume already had PV %v", v.Status.KubernetesStatus.PVName)
	}

	if pvName == "" {
		pvName = v.Name
	}

	if storageClassName == "" {
		if backupVolumeName := v.Labels[types.LonghornLabelBackupVolume]; backupVolumeName != "" {
			backupVolume, _ := m.ds.GetBackupVolumeRO(backupVolumeName)
			if backupVolume != nil {
				storageClassName = backupVolume.Status.StorageClassName
			}
		}
	}

	if storageClassName == "" {
		storageClassName, err = m.ds.GetSettingValueExisted(types.SettingNameDefaultLonghornStaticStorageClass)
		if err != nil {
			return nil, fmt.Errorf("failed to get longhorn default static storage class name for PV %v creation: %v", pvName, err)
		}
	}

	if fsType == "" {
		fsType = "ext4"
	}

	pv := datastore.NewPVManifestForVolume(v, pvName, storageClassName, fsType)
	if v.Spec.Encrypted {
		if secretName == "" {
			secretName = "longhorn-crypto"
		}

		if secretNamespace == "" {
			secretNamespace = "longhorn-system"
		}

		secretRef := &corev1.SecretReference{
			Name:      secretName,
			Namespace: secretNamespace,
		}
		pv.Spec.CSI.NodeStageSecretRef = secretRef
		pv.Spec.CSI.NodePublishSecretRef = secretRef
	}

	pv, err = m.ds.CreatePersistentVolume(pv)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Created PV for volume %v: %+v", v.Name, v.Spec)
	return v, nil
}

func (m *VolumeManager) PVCCreate(name, namespace, pvcName string) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to create PVC for volume %v", name)
	}()

	v, err = m.ds.GetVolume(name)
	if err != nil {
		return nil, err
	}
	ks := v.Status.KubernetesStatus

	if ks.LastPVCRefAt == "" && ks.PVCName != "" {
		return v, fmt.Errorf("volume already had PVC %v", ks.PVCName)
	}
	if pvcName == "" {
		pvcName = v.Name
	}

	var pvFound bool
	for i := 0; i < datastore.KubeStatusPollCount; i++ {
		v, err = m.ds.GetVolume(name)
		if err != nil {
			return nil, err
		}
		ks = v.Status.KubernetesStatus
		if v.Status.KubernetesStatus.PVName != "" &&
			(ks.PVStatus == string(corev1.VolumeAvailable) || ks.PVStatus == string(corev1.VolumeReleased)) {
			pvFound = true
			break
		}
		time.Sleep(datastore.KubeStatusPollInterval)
	}
	if !pvFound {
		return nil, fmt.Errorf("cannot found PV %v or the PV status %v is invalid for PVC creation", ks.PVName, ks.PVStatus)
	}

	pv, err := m.ds.GetPersistentVolume(ks.PVName)
	if err != nil {
		return nil, err
	}
	// cleanup ClaimRef of PV. Otherwise the existing PV cannot be reused.
	if pv.Spec.ClaimRef != nil {
		pv.Spec.ClaimRef = nil
		pv, err = m.ds.UpdatePersistentVolume(pv)
		if err != nil {
			return nil, err
		}
	}

	pvc := datastore.NewPVCManifestForVolume(v, ks.PVName, namespace, pvcName, pv.Spec.StorageClassName)
	pvc, err = m.ds.CreatePersistentVolumeClaim(namespace, pvc)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Created PVC for volume %v: %+v", v.Name, v.Spec)
	return v, nil
}

func (m *VolumeManager) GetDaemonSetRO(name string) (*appsv1.DaemonSet, error) {
	return m.ds.GetDaemonSet(name)
}
