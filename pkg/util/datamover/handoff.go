package datamover

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

const (
	TransferState        = "harvesterhci.io/restore-transfer-state"
	StagingName          = "harvesterhci.io/restore-staging-name"
	StagingUID           = "harvesterhci.io/restore-staging-uid"
	DestinationUID       = "harvesterhci.io/restore-destination-uid"
	DestinationName      = "harvesterhci.io/restore-destination-name"
	DestinationNamespace = "harvesterhci.io/restore-destination-namespace"
	OriginalPolicy       = "harvesterhci.io/restore-reclaim-policy"
	TransferJob          = "harvesterhci.io/restore-transfer-job"
	copied               = "copied"
	complete             = "complete"
)

type Volumes struct{ Client kubernetes.Interface }

// ResumeHandoff continues a recorded PV handoff before any Job can be recreated.
func (v Volumes) ResumeHandoff(
	parent metav1.Object, stage string, desired *corev1.PersistentVolumeClaim,
) (bool, bool, error) {
	pv, err := v.findTransfer(parent, stage)
	if err != nil {
		return pv != nil, false, err
	}
	if pv == nil {
		return false, false, nil
	}
	if err := validateTransfer(parent, pv, desired); err != nil {
		return true, false, err
	}
	err = v.advanceHandoff(parent, pv, desired)
	return true, err == nil, err
}

func (v Volumes) findTransfer(parent metav1.Object, stage string) (*corev1.PersistentVolume, error) {
	volumes, err := v.Client.CoreV1().PersistentVolumes().List(context.Background(), metav1.ListOptions{
		LabelSelector: joblifecycle.Selector(parent, RestoreKind),
	})
	if err != nil {
		return nil, err
	}
	var found *corev1.PersistentVolume
	for i := range volumes.Items {
		pv := &volumes.Items[i]
		if pv.Annotations[StagingName] != stage {
			continue
		}
		if found != nil {
			return found, fmt.Errorf("multiple PVs record the same restore transfer")
		}
		found = pv
	}
	return found, nil
}

func validateTransfer(
	parent metav1.Object, pv *corev1.PersistentVolume, desired *corev1.PersistentVolumeClaim,
) error {
	if err := joblifecycle.Validate(parent, RestoreKind, pv.ObjectMeta); err != nil {
		return err
	}
	if pv.Annotations[DestinationName] != desired.Name ||
		pv.Annotations[DestinationNamespace] != desired.Namespace {
		return fmt.Errorf("restore checkpoint has a different destination")
	}
	return nil
}

// BeginHandoff records successful transfer and Retain atomically before releasing storage.
func (v Volumes) BeginHandoff(
	parent metav1.Object, stage, job string, desired *corev1.PersistentVolumeClaim,
) error {
	pvc, err := v.stagingClaim(parent, stage)
	if err != nil {
		return err
	}
	pv, err := v.stagingVolume(pvc)
	if err != nil {
		return err
	}
	joblifecycle.Track(&pv.ObjectMeta, parent, RestoreKind)
	pv.Annotations[TransferState] = copied
	pv.Annotations[StagingName] = stage
	pv.Annotations[StagingUID] = string(pvc.UID)
	pv.Annotations[DestinationName] = desired.Name
	pv.Annotations[DestinationNamespace] = desired.Namespace
	pv.Annotations[OriginalPolicy] = string(pv.Spec.PersistentVolumeReclaimPolicy)
	pv.Annotations[TransferJob] = job
	pv.Spec.PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimRetain
	return v.updateVolume(pv)
}

func (v Volumes) stagingClaim(parent metav1.Object, name string) (*corev1.PersistentVolumeClaim, error) {
	pvc, err := trackedClaim(v.Client, parent, RestoreKind, name)
	if err != nil {
		return nil, err
	}
	if pvc.Spec.VolumeName == "" || pvc.Status.Phase != corev1.ClaimBound {
		return nil, ErrPending
	}
	return pvc, nil
}

func (v Volumes) stagingVolume(pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolume, error) {
	pv, err := v.Client.CoreV1().PersistentVolumes().Get(
		context.Background(), pvc.Spec.VolumeName, metav1.GetOptions{},
	)
	if err != nil {
		return nil, err
	}
	if !claimMatches(pv.Spec.ClaimRef, pvc) {
		return nil, fmt.Errorf("staging PV belongs to another claim")
	}
	if pv.DeletionTimestamp != nil || pvc.DeletionTimestamp != nil {
		return nil, fmt.Errorf("restore staging storage is being deleted")
	}
	if len(pv.OwnerReferences) != 0 {
		return nil, fmt.Errorf("staging PV has an unexpected owner")
	}
	if pv.Annotations[TransferState] != "" {
		return nil, fmt.Errorf("staging PV already records a transfer")
	}
	return pv, nil
}

// updateVolume waits for a later reconciliation to observe the persisted state.
func (v Volumes) updateVolume(pv *corev1.PersistentVolume) error {
	_, err := v.Client.CoreV1().PersistentVolumes().Update(context.Background(), pv, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return ErrPending
}

func (v Volumes) advanceHandoff(
	parent metav1.Object, pv *corev1.PersistentVolume, desired *corev1.PersistentVolumeClaim,
) error {
	if pv.DeletionTimestamp != nil {
		return fmt.Errorf("restore PV is being deleted")
	}
	state := pv.Annotations[TransferState]
	if state != copied && state != complete {
		return fmt.Errorf("invalid restore checkpoint")
	}
	destination, err := v.destination(parent, pv, desired)
	if err != nil {
		return err
	}
	if claimMatches(pv.Spec.ClaimRef, destination) {
		return v.finishDestination(pv, destination)
	}
	return v.rebind(parent, pv, destination)
}

func (v Volumes) finishDestination(
	pv *corev1.PersistentVolume, destination *corev1.PersistentVolumeClaim,
) error {
	if pv.Annotations[DestinationUID] != string(destination.UID) {
		return fmt.Errorf("restore destination identity changed")
	}
	return v.finish(pv, destination)
}

func validateHandoff(pv *corev1.PersistentVolume) error {
	if pv.Annotations[TransferState] == complete {
		return fmt.Errorf("completed restore PV lost its destination binding")
	}
	if !assignedToStage(pv) {
		return fmt.Errorf("restore PV has an unexpected claim reference")
	}
	if pv.Spec.PersistentVolumeReclaimPolicy != corev1.PersistentVolumeReclaimRetain {
		return fmt.Errorf("restore PV must retain data during handoff")
	}
	return nil
}

func (v Volumes) rebind(
	parent metav1.Object, pv *corev1.PersistentVolume, destination *corev1.PersistentVolumeClaim,
) error {
	if err := validateHandoff(pv); err != nil {
		return err
	}
	if err := StopJob(v.Client, parent, RestoreKind, pv.Annotations[TransferJob]); err != nil {
		return err
	}
	if err := Detached(v.Client, pv.Name); err != nil {
		return err
	}
	if err := v.releaseStage(parent, pv); err != nil {
		return err
	}
	pv.Annotations[DestinationUID] = string(destination.UID)
	pv.Spec.ClaimRef = &corev1.ObjectReference{
		APIVersion: "v1", Kind: "PersistentVolumeClaim",
		Namespace: destination.Namespace, Name: destination.Name, UID: destination.UID,
	}
	return v.updateVolume(pv)
}

func (v Volumes) destination(
	parent metav1.Object, pv *corev1.PersistentVolume, template *corev1.PersistentVolumeClaim,
) (*corev1.PersistentVolumeClaim, error) {
	desired := destinationClaim(parent, pv.Name, template)
	claims := v.Client.CoreV1().PersistentVolumeClaims(desired.Namespace)
	current, err := claims.Get(context.Background(), desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return v.createDestination(pv, desired)
	}
	if err != nil {
		return nil, err
	}
	if err := validateDestination(parent, current, desired); err != nil {
		return nil, err
	}
	return current, nil
}

func destinationClaim(
	parent metav1.Object, volume string, template *corev1.PersistentVolumeClaim,
) *corev1.PersistentVolumeClaim {
	desired := template.DeepCopy()
	desired.Spec.VolumeName = volume
	desired.Spec.DataSource = nil
	desired.Spec.DataSourceRef = nil
	joblifecycle.Track(&desired.ObjectMeta, parent, RestoreKind)
	return desired
}

func validateDestination(parent metav1.Object, current, desired *corev1.PersistentVolumeClaim) error {
	if err := ValidateClaim(parent, RestoreKind, current, desired); err != nil {
		return err
	}
	if current.Spec.VolumeName != desired.Spec.VolumeName ||
		current.Spec.DataSource != nil || current.Spec.DataSourceRef != nil {
		return fmt.Errorf("destination PVC has a different source")
	}
	return nil
}

func (v Volumes) createDestination(
	pv *corev1.PersistentVolume, desired *corev1.PersistentVolumeClaim,
) (*corev1.PersistentVolumeClaim, error) {
	ref := pv.Spec.ClaimRef
	assigned := ref != nil && ref.Namespace == desired.Namespace && ref.Name == desired.Name
	if pv.Annotations[TransferState] == complete || assigned {
		return nil, fmt.Errorf("restore destination disappeared after ownership transfer")
	}
	claims := v.Client.CoreV1().PersistentVolumeClaims(desired.Namespace)
	if _, err := claims.Create(context.Background(), desired, metav1.CreateOptions{}); err != nil {
		return nil, err
	}
	return nil, ErrPending
}

func (v Volumes) releaseStage(parent metav1.Object, pv *corev1.PersistentVolume) error {
	claims := v.Client.CoreV1().PersistentVolumeClaims(joblifecycle.Namespace)
	pvc, err := claims.Get(context.Background(), pv.Annotations[StagingName], metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if string(pvc.UID) != pv.Annotations[StagingUID] || pvc.Spec.VolumeName != pv.Name {
		return fmt.Errorf("staging claim has been replaced")
	}
	if err := Unused(v.Client, pvc.Namespace, pvc.Name); err != nil {
		return err
	}
	return DeleteClaim(v.Client, parent, RestoreKind, pvc.Name)
}

func (v Volumes) finish(pv *corev1.PersistentVolume, destination *corev1.PersistentVolumeClaim) error {
	if pv.Status.Phase != corev1.VolumeBound || destination.Status.Phase != corev1.ClaimBound {
		return ErrPending
	}
	policy, err := originalPolicy(pv)
	if err != nil {
		return err
	}
	if pv.Annotations[TransferState] == complete && pv.Spec.PersistentVolumeReclaimPolicy == policy {
		return nil
	}
	pv.Spec.PersistentVolumeReclaimPolicy = policy
	pv.Annotations[TransferState] = complete
	_, err = v.Client.CoreV1().PersistentVolumes().Update(context.Background(), pv, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return ErrPending
}

func originalPolicy(pv *corev1.PersistentVolume) (corev1.PersistentVolumeReclaimPolicy, error) {
	policy := corev1.PersistentVolumeReclaimPolicy(pv.Annotations[OriginalPolicy])
	if policy != corev1.PersistentVolumeReclaimDelete && policy != corev1.PersistentVolumeReclaimRetain {
		return "", fmt.Errorf("restore PV has an invalid original reclaim policy")
	}
	return policy, nil
}

func claimMatches(ref *corev1.ObjectReference, pvc *corev1.PersistentVolumeClaim) bool {
	return ref != nil && pvc.UID != "" && ref.UID == pvc.UID && ref.Name == pvc.Name && ref.Namespace == pvc.Namespace
}

// Cleanup preserves volumes whose ownership has crossed to the destination.
func (v Volumes) Cleanup(parent metav1.Object) error {
	if err := v.cleanupVolumes(parent); err != nil {
		return err
	}
	return v.cleanupClaims(parent)
}

func (v Volumes) cleanupVolumes(parent metav1.Object) error {
	volumes, err := v.Client.CoreV1().PersistentVolumes().List(context.Background(), metav1.ListOptions{
		LabelSelector: joblifecycle.Selector(parent, RestoreKind),
	})
	if err != nil {
		return err
	}
	for i := range volumes.Items {
		if err := v.cancelVolume(parent, &volumes.Items[i]); err != nil {
			return err
		}
	}
	return nil
}

func (v Volumes) cleanupClaims(parent metav1.Object) error {
	claims := v.Client.CoreV1().PersistentVolumeClaims(joblifecycle.Namespace)
	list, err := claims.List(context.Background(), metav1.ListOptions{
		LabelSelector: joblifecycle.Selector(parent, RestoreKind),
	})
	if err != nil {
		return err
	}
	for _, pvc := range list.Items {
		if err := DeleteClaim(v.Client, parent, RestoreKind, pvc.Name); err != nil {
			return err
		}
	}
	return nil
}

func (v Volumes) cancelVolume(parent metav1.Object, pv *corev1.PersistentVolume) error {
	if err := joblifecycle.Validate(parent, RestoreKind, pv.ObjectMeta); err != nil {
		return err
	}
	if assignedToDestination(pv) {
		return v.keepVolume(pv)
	}
	if err := validateCancellation(pv); err != nil {
		return err
	}
	if err := Detached(v.Client, pv.Name); err != nil {
		return err
	}
	if err := v.cancelDestination(parent, pv); err != nil {
		return err
	}
	if err := v.releaseStage(parent, pv); err != nil {
		return err
	}
	return v.deleteVolume(pv)
}

func assignedToDestination(pv *corev1.PersistentVolume) bool {
	ref := pv.Spec.ClaimRef
	return ref != nil && ref.UID != "" &&
		ref.Namespace == pv.Annotations[DestinationNamespace] &&
		ref.Name == pv.Annotations[DestinationName] &&
		string(ref.UID) == pv.Annotations[DestinationUID]
}

func assignedToStage(pv *corev1.PersistentVolume) bool {
	ref := pv.Spec.ClaimRef
	return ref != nil && ref.Namespace == joblifecycle.Namespace &&
		ref.Name == pv.Annotations[StagingName] && string(ref.UID) == pv.Annotations[StagingUID]
}

func validateCancellation(pv *corev1.PersistentVolume) error {
	if pv.Annotations[TransferState] == complete {
		return fmt.Errorf("completed restore has an unexpected binding")
	}
	if !assignedToStage(pv) {
		return fmt.Errorf("cancelled restore has an unexpected binding")
	}
	return nil
}

func (v Volumes) keepVolume(pv *corev1.PersistentVolume) error {
	policy, err := originalPolicy(pv)
	if err != nil {
		return err
	}
	if pv.Spec.PersistentVolumeReclaimPolicy == policy {
		return nil
	}
	pv.Spec.PersistentVolumeReclaimPolicy = policy
	_, err = v.Client.CoreV1().PersistentVolumes().Update(context.Background(), pv, metav1.UpdateOptions{})
	return err
}

// A pending destination is safe to remove only while the PV still names the staging claim.
func (v Volumes) cancelDestination(parent metav1.Object, pv *corev1.PersistentVolume) error {
	claims := v.Client.CoreV1().PersistentVolumeClaims(pv.Annotations[DestinationNamespace])
	dest, err := claims.Get(context.Background(), pv.Annotations[DestinationName], metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := joblifecycle.Validate(parent, RestoreKind, dest.ObjectMeta); err != nil {
		return err
	}
	if dest.Spec.VolumeName != pv.Name || dest.Status.Phase == corev1.ClaimBound {
		return fmt.Errorf("cancelled restore destination is already in use")
	}
	return deleteClaim(v.Client, dest)
}

func (v Volumes) deleteVolume(pv *corev1.PersistentVolume) error {
	if err := v.prepareDeletion(pv); err != nil {
		return err
	}
	err := v.Client.CoreV1().PersistentVolumes().Delete(context.Background(), pv.Name, metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &pv.UID},
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return ErrPending
}

// prepareDeletion persists the reclaim policy before a later reconcile deletes the PV.
func (v Volumes) prepareDeletion(pv *corev1.PersistentVolume) error {
	policy, err := originalPolicy(pv)
	if err != nil {
		return err
	}
	if pv.Spec.PersistentVolumeReclaimPolicy == policy {
		return nil
	}
	pv.Spec.PersistentVolumeReclaimPolicy = policy
	return v.updateVolume(pv)
}
