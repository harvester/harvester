package types

import (
	"os"

	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	LonghornManagerDaemonSetName = "longhorn-manager"
	LonghornManagerContainerName = LonghornManagerDaemonSetName
	LonghornUIDeploymentName     = "longhorn-ui"

	DriverDeployerName = "longhorn-driver-deployer"
	CSIAttacherName    = "csi-attacher"
	CSIProvisionerName = "csi-provisioner"
	CSIResizerName     = "csi-resizer"
	CSISnapshotterName = "csi-snapshotter"
	CSIPluginName      = "longhorn-csi-plugin"
)

// AddGoCoverDirToPod adds GOCOVERDIR env and host path volume to a pod.
// It's used to collect coverage data from a pod.
func AddGoCoverDirToPod(pod *corev1.Pod) {
	if pod == nil || len(pod.Spec.Containers) == 0 {
		return
	}

	goCoverDir := os.Getenv("GOCOVERDIR")
	if goCoverDir == "" {
		return
	}

	pod.Spec.Containers[0].Env = append(
		pod.Spec.Containers[0].Env,
		corev1.EnvVar{Name: "GOCOVERDIR", Value: goCoverDir},
	)
	pod.Spec.Containers[0].VolumeMounts = append(
		pod.Spec.Containers[0].VolumeMounts,
		corev1.VolumeMount{Name: "go-cover-dir", MountPath: goCoverDir},
	)
	hostPathType := corev1.HostPathDirectoryOrCreate
	pod.Spec.Volumes = append(
		pod.Spec.Volumes,
		corev1.Volume{
			Name: "go-cover-dir",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: goCoverDir,
					Type: &hostPathType,
				},
			},
		},
	)
}

// AddGoCoverDirToDaemonSet adds GOCOVERDIR env and host path volume to a daemonset.
// It's used to collect coverage data from a daemonset.
func AddGoCoverDirToDaemonSet(daemonset *appsv1.DaemonSet) {
	if daemonset == nil || len(daemonset.Spec.Template.Spec.Containers) == 0 {
		return
	}

	goCoverDir := os.Getenv("GOCOVERDIR")
	if goCoverDir == "" {
		return
	}

	daemonset.Spec.Template.Spec.Containers[0].Env = append(
		daemonset.Spec.Template.Spec.Containers[0].Env,
		corev1.EnvVar{Name: "GOCOVERDIR", Value: goCoverDir},
	)
	daemonset.Spec.Template.Spec.Containers[0].VolumeMounts = append(
		daemonset.Spec.Template.Spec.Containers[0].VolumeMounts,
		corev1.VolumeMount{Name: "go-cover-dir", MountPath: goCoverDir},
	)
	hostPathType := corev1.HostPathDirectoryOrCreate
	daemonset.Spec.Template.Spec.Volumes = append(
		daemonset.Spec.Template.Spec.Volumes,
		corev1.Volume{
			Name: "go-cover-dir",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: goCoverDir,
					Type: &hostPathType,
				},
			},
		},
	)
}

func UpdateDaemonSetTemplateBasedOnStorageNetwork(daemonSet *appsv1.DaemonSet, storageNetwork *longhorn.Setting, isStorageNetworkForRWXVolumeEnabled bool) {
	if daemonSet == nil {
		return
	}

	logger := logrus.WithField("daemonSet", daemonSet.Name)
	logger.Infof("Updating DaemonSet template for storage network %v", storageNetwork)

	isContainerNetworkNamespace := IsStorageNetworkForRWXVolume(storageNetwork, isStorageNetworkForRWXVolumeEnabled)

	updateAnnotation := func() {
		annotKey := string(CNIAnnotationNetworks)
		annotValue := ""
		if isContainerNetworkNamespace {
			annotValue = CreateCniAnnotationFromSetting(storageNetwork)
		}

		logger.WithFields(logrus.Fields{
			"oldValue": daemonSet.Spec.Template.Annotations[annotKey],
			"newValue": annotValue,
		}).Debugf("Updating template %v annotation", annotKey)

		if daemonSet.Spec.Template.Annotations == nil {
			daemonSet.Spec.Template.Annotations = make(map[string]string)
		}

		daemonSet.Spec.Template.Annotations[annotKey] = annotValue

		if annotValue == "" {
			delete(daemonSet.Spec.Template.Annotations, annotKey)
		}
	}

	updateAnnotation()
}
