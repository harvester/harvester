package pvcbackup

import (
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/volumeremotebackup/common"
	"github.com/harvester/harvester/pkg/volumeremotebackup/driver"
)

const (
	remoteBackupKindName  = "VolumeRemoteBackup"
	remoteRestoreKindName = "VolumeRemoteRestore"
)

var remoteBackupKind = harvesterv1.SchemeGroupVersion.WithKind(remoteBackupKindName)
var remoteRestoreKind = harvesterv1.SchemeGroupVersion.WithKind(remoteRestoreKindName)

func (rbh *remoteBackupHandler) OnChanged(_ string, vrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error) {
	if vrb == nil || vrb.DeletionTimestamp != nil {
		return vrb, nil
	}

	op, ok := rbh.operations[rbh.bo.GetType(vrb)]
	if !ok {
		logrus.Errorf("BackupOperation not found for type %s", rbh.bo.GetType(vrb))
		return vrb, nil
	}

	ready, err := op.Readiness(vrb)
	if err != nil {
		return rbh.handleReadinessError(vrb, op, err)
	}

	if ready {
		return rbh.handleReadyBackup(vrb)
	}

	rbh.vrbController.EnqueueAfter(rbh.bo.GetNamespace(vrb), rbh.bo.GetName(vrb), 5*time.Second)
	return vrb, nil
}

func (rbh *remoteBackupHandler) handleReadinessError(
	vrb *harvesterv1.VolumeRemoteBackup,
	op driver.BackupOperation,
	err error,
) (*harvesterv1.VolumeRemoteBackup, error) {
	if common.IsRetryLater(err) {
		logrus.Infof("VolumeRemoteBackup %s/%s not ready, retrying in 5 seconds", rbh.bo.GetNamespace(vrb), rbh.bo.GetName(vrb))
		rbh.vrbController.EnqueueAfter(rbh.bo.GetNamespace(vrb), rbh.bo.GetName(vrb), 5*time.Second)
		return vrb, nil
	}

	if apierrors.IsNotFound(err) {
		logrus.Infof("Related resource not found for VolumeRemoteBackup %s/%s", rbh.bo.GetNamespace(vrb), rbh.bo.GetName(vrb))
		return rbh.handleResourceNotFound(vrb, op)
	}

	vrb, setErr := rbh.bo.SetError(vrb, err)
	if setErr != nil {
		return vrb, setErr
	}

	return vrb, err
}

func (rbh *remoteBackupHandler) handleResourceNotFound(
	vrb *harvesterv1.VolumeRemoteBackup,
	op driver.BackupOperation,
) (*harvesterv1.VolumeRemoteBackup, error) {
	if err := op.Create(vrb); err != nil {
		return vrb, err
	}
	logrus.Infof("Successfully created resource for PVCBackup %s/%s", rbh.bo.GetNamespace(vrb), rbh.bo.GetName(vrb))
	return rbh.bo.SetProcessing(vrb)
}

func (rbh *remoteBackupHandler) handleReadyBackup(vrb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error) {
	logrus.Infof("Handling ready backup for PVCBackup %s/%s", rbh.bo.GetNamespace(vrb), rbh.bo.GetName(vrb))

	return rbh.bo.Finalize(vrb)
}

func (rbh *remoteBackupHandler) OnRemove(_ string, pb *harvesterv1.VolumeRemoteBackup) (*harvesterv1.VolumeRemoteBackup, error) {
	if pb == nil {
		return nil, nil
	}

	op, ok := rbh.operations[rbh.bo.GetType(pb)]
	if !ok {
		return pb, nil
	}

	if err := op.Delete(pb); err != nil {
		return pb, err
	}

	logrus.Infof("Successfully deleted resources PVCBackup %s/%s", rbh.bo.GetNamespace(pb), rbh.bo.GetName(pb))
	return pb, nil
}

func (rrh *remoteRestoreHandler) OnChanged(_ string, vrr *harvesterv1.VolumeRemoteRestore) (*harvesterv1.VolumeRemoteRestore, error) {
	if vrr == nil || vrr.DeletionTimestamp != nil {
		return vrr, nil
	}

	op, ok := rrh.operations[rrh.ro.GetType(vrr)]
	if !ok {
		logrus.Errorf("RestoreOperation not found for type %s", rrh.ro.GetType(vrr))
		return vrr, nil
	}

	ready, err := op.Readiness(vrr)
	if err != nil {
		return rrh.handleReadinessError(vrr, op, err)
	}

	if ready {
		return rrh.handleReadyRestore(vrr)
	}

	rrh.vrrController.EnqueueAfter(rrh.ro.GetNamespace(vrr), rrh.ro.GetName(vrr), 5*time.Second)
	return vrr, nil
}

func (rrh *remoteRestoreHandler) handleReadinessError(
	vrr *harvesterv1.VolumeRemoteRestore,
	op driver.RestoreOperation,
	err error,
) (*harvesterv1.VolumeRemoteRestore, error) {
	if common.IsRetryLater(err) {
		logrus.Infof("VolumeRemoteRestore %s/%s not ready, retrying in 5 seconds", rrh.ro.GetNamespace(vrr), rrh.ro.GetName(vrr))
		rrh.vrrController.EnqueueAfter(rrh.ro.GetNamespace(vrr), rrh.ro.GetName(vrr), 5*time.Second)
		return vrr, nil
	}

	if apierrors.IsNotFound(err) {
		logrus.Infof("Related resource not found for VolumeRemoteRestore %s/%s", rrh.ro.GetNamespace(vrr), rrh.ro.GetName(vrr))
		return rrh.handleResourceNotFound(vrr, op)
	}

	pr, SetErr := rrh.ro.SetError(vrr, err)
	if SetErr != nil {
		return pr, SetErr
	}

	return pr, err
}

func (rrh *remoteRestoreHandler) handleResourceNotFound(
	vrr *harvesterv1.VolumeRemoteRestore,
	op driver.RestoreOperation,
) (*harvesterv1.VolumeRemoteRestore, error) {
	if err := op.Create(vrr); err != nil {
		return vrr, err
	}
	logrus.Infof("Successfully created resource for PVCRestore %s/%s", rrh.ro.GetNamespace(vrr), rrh.ro.GetName(vrr))
	return rrh.ro.SetProcessing(vrr)
}

func (rrh *remoteRestoreHandler) handleReadyRestore(vrr *harvesterv1.VolumeRemoteRestore) (*harvesterv1.VolumeRemoteRestore, error) {
	logrus.Infof("Handling ready restore for PVCRestore %s/%s", rrh.ro.GetNamespace(vrr), rrh.ro.GetName(vrr))

	updatedPr, err := rrh.ro.SetReady(vrr)
	if err != nil {
		return vrr, err
	}

	updatedPr, err = rrh.ro.SetSuccess(updatedPr, true)
	if err != nil {
		return vrr, err
	}

	return updatedPr, nil
}

func (rrh *remoteRestoreHandler) OnRemove(_ string, vrr *harvesterv1.VolumeRemoteRestore) (*harvesterv1.VolumeRemoteRestore, error) {
	if vrr == nil {
		return nil, nil
	}

	op, ok := rrh.operations[rrh.ro.GetType(vrr)]
	if !ok {
		return vrr, nil
	}

	if err := op.Delete(vrr); err != nil {
		return vrr, err
	}

	logrus.Infof("Successfully deleted resources PVCRestore %s/%s", rrh.ro.GetNamespace(vrr), rrh.ro.GetName(vrr))
	return vrr, nil
}

// tryResolveAndEnqueueByRef is a generic helper that tries to resolve and enqueue by owner reference
func tryResolveAndEnqueueByRef(
	resolvers []ownerRefResolver,
	resourceType, resourceNamespace, resourceName string,
	controllerRef *metav1.OwnerReference,
) {
	for _, resolver := range resolvers {
		namespace, name, found := resolver.resolveRef(resourceNamespace, controllerRef)
		if !found {
			continue
		}

		logrus.Infof("Enqueuing %s %s/%s due to %s %s/%s change",
			resolver.getResourceType(), namespace, name, resourceType, resourceNamespace, resourceName)
		resolver.enqueue(namespace, name)
		break
	}
}

// tryResolveAndEnqueueByAnnotation is a generic helper that tries to resolve and enqueue by annotation
func tryResolveAndEnqueueByAnnotation(
	resolvers []ownerRefResolver,
	resourceType, resourceNamespace, resourceName string,
	annotations map[string]string,
) {
	for _, resolver := range resolvers {
		namespace, name, found := resolver.resolveAnnotation(resourceNamespace, annotations)
		if !found {
			continue
		}

		logrus.Infof("Enqueuing %s %s/%s due to %s %s/%s change (via annotation)",
			resolver.getResourceType(), namespace, name, resourceType, resourceNamespace, resourceName)
		resolver.enqueue(namespace, name)
		break
	}
}

func (vsh *volumeSnapshotHandler) tryResolveAndEnqueueByRef(vsNamespace, vsName string, controllerRef *metav1.OwnerReference) {
	tryResolveAndEnqueueByRef(vsh.resolvers, "VolumeSnapshot", vsNamespace, vsName, controllerRef)
}

func (vsh *volumeSnapshotHandler) tryResolveAndEnqueueByAnnotation(vsNamespace, vsName string, annotations map[string]string) {
	tryResolveAndEnqueueByAnnotation(vsh.resolvers, "VolumeSnapshot", vsNamespace, vsName, annotations)
}

func (vsh *volumeSnapshotHandler) onChanged(_ string, vs *snapshotv1.VolumeSnapshot) (*snapshotv1.VolumeSnapshot, error) {
	if vs == nil || vs.DeletionTimestamp != nil {
		return nil, nil
	}

	// Try to resolve via owner reference
	if controllerRef := metav1.GetControllerOf(vs); controllerRef != nil {
		vsh.tryResolveAndEnqueueByRef(vs.Namespace, vs.Name, controllerRef)
	}

	// Try to resolve via annotation
	if len(vs.Annotations) > 0 {
		vsh.tryResolveAndEnqueueByAnnotation(vs.Namespace, vs.Name, vs.Annotations)
	}

	return vs, nil
}

func (ph *pvcHandler) tryResolveAndEnqueueByRef(pvcNamespace, pvcName string, controllerRef *metav1.OwnerReference) {
	tryResolveAndEnqueueByRef(ph.resolvers, "PVC", pvcNamespace, pvcName, controllerRef)
}

func (ph *pvcHandler) tryResolveAndEnqueueByAnnotation(pvcNamespace, pvcName string, annotations map[string]string) {
	tryResolveAndEnqueueByAnnotation(ph.resolvers, "PVC", pvcNamespace, pvcName, annotations)
}

func (ph *pvcHandler) onChanged(_ string, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	if pvc == nil || pvc.DeletionTimestamp != nil {
		return nil, nil
	}

	// Try to resolve via owner reference
	if controllerRef := metav1.GetControllerOf(pvc); controllerRef != nil {
		ph.tryResolveAndEnqueueByRef(pvc.Namespace, pvc.Name, controllerRef)
	}

	// Try to resolve via annotation
	if len(pvc.Annotations) > 0 {
		ph.tryResolveAndEnqueueByAnnotation(pvc.Namespace, pvc.Name, pvc.Annotations)
	}

	return pvc, nil
}
