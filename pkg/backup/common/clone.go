package common

import (
	"fmt"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// ValidateClonePVC checks the backup identity and snapshot source before reuse
// or deletion. A matching name alone does not establish ownership.
func ValidateClonePVC(
	pvc *corev1.PersistentVolumeClaim,
	owner metav1.OwnerReference,
	snapshotName string,
) error {
	if err := checkOwner(pvc, owner); err != nil {
		return err
	}
	return checkSource(pvc, snapshotName)
}

func checkOwner(pvc *corev1.PersistentVolumeClaim, owner metav1.OwnerReference) error {
	controller := metav1.GetControllerOfNoCopy(pvc)
	if owner.UID == "" || controller == nil || controller.UID != owner.UID ||
		controller.APIVersion != owner.APIVersion || controller.Kind != owner.Kind ||
		controller.Name != owner.Name {
		return fmt.Errorf("PVC %s/%s is not controlled by %s %s with UID %s",
			pvc.Namespace, pvc.Name, owner.Kind, owner.Name, owner.UID)
	}
	return nil
}

func checkSource(pvc *corev1.PersistentVolumeClaim, snapshotName string) error {
	source := pvc.Spec.DataSource
	if snapshotName == "" || source == nil ||
		ptr.Deref(source.APIGroup, "") != snapshotv1.GroupName ||
		source.Kind != "VolumeSnapshot" || source.Name != snapshotName {
		return fmt.Errorf("PVC %s/%s does not reference VolumeSnapshot %s",
			pvc.Namespace, pvc.Name, snapshotName)
	}
	// Kubernetes can populate DataSourceRef from DataSource. When present, both
	// references must identify the same snapshot in the PVC's namespace.
	ref := pvc.Spec.DataSourceRef
	if ref == nil {
		return nil
	}
	if ptr.Deref(ref.APIGroup, "") != snapshotv1.GroupName ||
		ref.Kind != "VolumeSnapshot" || ref.Name != snapshotName {
		return fmt.Errorf("PVC %s/%s has a conflicting dataSourceRef",
			pvc.Namespace, pvc.Name)
	}
	namespace := ptr.Deref(ref.Namespace, "")
	if namespace != "" && namespace != pvc.Namespace {
		return fmt.Errorf("PVC %s/%s has a conflicting dataSourceRef",
			pvc.Namespace, pvc.Name)
	}
	return nil
}

// DeleteClonePVC uses a live lookup and UID precondition to avoid deleting an
// unrelated PVC, including one that replaces the clone after validation.
func DeleteClonePVC(
	client ctlcorev1.PersistentVolumeClaimClient,
	namespace, name string,
	owner metav1.OwnerReference,
	snapshotName string,
) error {
	pvc, err := client.Get(namespace, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := ValidateClonePVC(pvc, owner, snapshotName); err != nil {
		return err
	}
	if pvc.UID == "" {
		return fmt.Errorf("PVC %s/%s has no UID", namespace, name)
	}
	err = client.Delete(namespace, name, &metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &pvc.UID},
	})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
