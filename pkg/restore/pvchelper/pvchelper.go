package pvchelper

import (
	"fmt"
	"strings"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/harvester/harvester/pkg/restore/engine"
)

const (
	volumeSnapshotKind = "VolumeSnapshot"
)

// BuildPVCFromSnapshot creates a PVC spec from a VolumeSnapshot
func BuildPVCFromSnapshot(
	namespace string,
	pvcName string,
	vsName string,
	labels map[string]string,
	annotations map[string]string,
	pvcSpec corev1.PersistentVolumeClaimSpec,
) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pvcName,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: pvcSpec.AccessModes,
			DataSource: &corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(snapshotv1.SchemeGroupVersion.Group),
				Kind:     volumeSnapshotKind,
				Name:     vsName,
			},
			Resources:        pvcSpec.Resources,
			StorageClassName: pvcSpec.StorageClassName,
			VolumeMode:       pvcSpec.VolumeMode,
		},
	}
}

// CheckPVCStatus validates the PVC status and returns appropriate errors
func CheckPVCStatus(pvc *corev1.PersistentVolumeClaim) error {
	if pvc.Status.Phase == corev1.ClaimPending {
		return engine.ErrRetryLater
	}

	if pvc.Status.Phase != corev1.ClaimBound {
		return fmt.Errorf("PVC %s/%s in status %q", pvc.Namespace, pvc.Name, pvc.Status.Phase)
	}

	return nil
}

// BuildRestoreAnnotations creates annotations map for restored PVC, filtering out unwanted annotations
func BuildRestoreAnnotations(sourceAnnotations map[string]string, restoreName string, restoreAnnotationKey string) map[string]string {
	annotations := make(map[string]string)

	for key, value := range sourceAnnotations {
		if !ShouldSkipAnnotation(key) {
			annotations[key] = value
		}
	}

	annotations[restoreAnnotationKey] = restoreName
	return annotations
}

// BuildRestoreLabels copies source PVC labels but strips CDI ownership markers.
// Without this, CDI sees the destination PVC as one of its own and races its
// populator/clone against our engine's writes, corrupting the restored data.
func BuildRestoreLabels(sourceLabels map[string]string) map[string]string {
	labels := make(map[string]string)
	for key, value := range sourceLabels {
		if !ShouldSkipLabel(key, value) {
			labels[key] = value
		}
	}
	return labels
}

// ShouldSkipAnnotation checks if an annotation should be filtered out
func ShouldSkipAnnotation(key string) bool {
	// `cdi.kubevirt.io/*` annotations re-attach CDI to the destination PVC,
	// which then runs its own populator clone in parallel with our engine.
	skipPrefixes := []string{"pv.kubernetes.io", "cdi.kubevirt.io"}
	for _, prefix := range skipPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// ShouldSkipLabel checks if a label should be filtered out. Targets the marker
// labels CDI uses to claim ownership of a PVC.
func ShouldSkipLabel(key, value string) bool {
	if strings.HasPrefix(key, "cdi.kubevirt.io") {
		return true
	}
	cdiOwnership := map[string]string{
		"app":                          "containerized-data-importer",
		"app.kubernetes.io/component":  "storage",
		"app.kubernetes.io/managed-by": "cdi-controller",
	}
	if want, ok := cdiOwnership[key]; ok && want == value {
		return true
	}
	return false
}
