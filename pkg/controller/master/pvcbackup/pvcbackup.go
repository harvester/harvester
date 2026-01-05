package pvcbackup

import (
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/pvcbackup/common"
	"github.com/harvester/harvester/pkg/pvcbackup/driver"
)

const (
	pvcBackupKindName  = "PVCBackup"
	pvcRestoreKindName = "PVCRestore"
)

var pvcBackupKind = harvesterv1.SchemeGroupVersion.WithKind(pvcBackupKindName)
var pvcRestoreKind = harvesterv1.SchemeGroupVersion.WithKind(pvcRestoreKindName)

func (pbh *pvcbackupHandler) OnChanged(_ string, pb *harvesterv1.PVCBackup) (*harvesterv1.PVCBackup, error) {
	if pb == nil || pb.DeletionTimestamp != nil {
		return pb, nil
	}

	op, ok := pbh.operations[pbh.pbo.GetType(pb)]
	if !ok {
		logrus.Errorf("BackupOperation not found for type %s", pbh.pbo.GetType(pb))
		return pb, nil
	}

	ready, err := op.Readiness(pb)
	if err != nil {
		return pbh.handleReadinessError(pb, op, err)
	}

	if ready {
		return pbh.handleReadyBackup(pb)
	}

	return pb, nil
}

func (pbh *pvcbackupHandler) handleReadinessError(
	pb *harvesterv1.PVCBackup,
	op driver.BackupOperation,
	err error,
) (*harvesterv1.PVCBackup, error) {
	if common.IsRetryLater(err) {
		logrus.Infof("PVCBackup %s/%s not ready, retrying in 5 seconds", pbh.pbo.GetNamespace(pb), pbh.pbo.GetName(pb))
		pbh.pbController.EnqueueAfter(pbh.pbo.GetNamespace(pb), pbh.pbo.GetName(pb), 5*time.Second)
		return pb, nil
	}

	if apierrors.IsNotFound(err) {
		logrus.Infof("Resource not found for PVCBackup %s/%s, creating it", pbh.pbo.GetNamespace(pb), pbh.pbo.GetName(pb))
		return pbh.handleResourceNotFound(pb, op)
	}

	pb, SetErr := pbh.pbo.SetError(pb, err)
	if SetErr != nil {
		return pb, SetErr
	}

	return pb, err
}

func (pbh *pvcbackupHandler) handleResourceNotFound(
	pb *harvesterv1.PVCBackup,
	op driver.BackupOperation,
) (*harvesterv1.PVCBackup, error) {
	if err := op.Create(pb); err != nil {
		return pb, err
	}
	logrus.Infof("Successfully created resource for PVCBackup %s/%s", pbh.pbo.GetNamespace(pb), pbh.pbo.GetName(pb))
	return pbh.pbo.SetProcessing(pb)
}

func (pbh *pvcbackupHandler) handleReadyBackup(pb *harvesterv1.PVCBackup) (*harvesterv1.PVCBackup, error) {
	logrus.Infof("Handling ready backup for PVCBackup %s/%s", pbh.pbo.GetNamespace(pb), pbh.pbo.GetName(pb))

	updatedPb, err := pbh.pbo.SetReady(pb)
	if err != nil {
		return pb, err
	}

	updatedPb, err = pbh.pbo.SetSuccess(updatedPb, true)
	if err != nil {
		return pb, err
	}

	updatedPb, err = pbh.pbo.UpdateCSIProvider(updatedPb)
	if err != nil {
		return pb, err
	}

	updatedPb, err = pbh.pbo.UpdateSourceSpec(updatedPb)
	if err != nil {
		return pb, err
	}

	return updatedPb, nil
}

func (pbh *pvcbackupHandler) OnRemove(_ string, pb *harvesterv1.PVCBackup) (*harvesterv1.PVCBackup, error) {
	if pb == nil {
		return nil, nil
	}

	op, ok := pbh.operations[pbh.pbo.GetType(pb)]
	if !ok {
		return pb, nil
	}

	if err := op.Delete(pb); err != nil {
		return pb, err
	}

	logrus.Infof("Successfully deleted resources PVCBackup %s/%s", pbh.pbo.GetNamespace(pb), pbh.pbo.GetName(pb))
	return pb, nil
}

func (prh *pvcrestoreHandler) OnChanged(_ string, pr *harvesterv1.PVCRestore) (*harvesterv1.PVCRestore, error) {
	if pr == nil || pr.DeletionTimestamp != nil {
		return pr, nil
	}

	op, ok := prh.operations[prh.pro.GetType(pr)]
	if !ok {
		logrus.Errorf("RestoreOperation not found for type %s", prh.pro.GetType(pr))
		return pr, nil
	}

	ready, err := op.Readiness(pr)
	if err != nil {
		return prh.handleReadinessError(pr, op, err)
	}

	if ready {
		return prh.handleReadyRestore(pr)
	}

	return pr, nil
}

func (prh *pvcrestoreHandler) handleReadinessError(
	pr *harvesterv1.PVCRestore,
	op driver.RestoreOperation,
	err error,
) (*harvesterv1.PVCRestore, error) {
	if common.IsRetryLater(err) {
		logrus.Infof("PVCRestore %s/%s not ready, retrying in 5 seconds", prh.pro.GetNamespace(pr), prh.pro.GetName(pr))
		prh.prController.EnqueueAfter(prh.pro.GetNamespace(pr), prh.pro.GetName(pr), 5*time.Second)
		return pr, nil
	}

	if apierrors.IsNotFound(err) {
		logrus.Infof("Resource not found for PVCRestore %s/%s, creating it", prh.pro.GetNamespace(pr), prh.pro.GetName(pr))
		return prh.handleResourceNotFound(pr, op)
	}

	pr, SetErr := prh.pro.SetError(pr, err)
	if SetErr != nil {
		return pr, SetErr
	}

	return pr, err
}

func (prh *pvcrestoreHandler) handleResourceNotFound(
	pr *harvesterv1.PVCRestore,
	op driver.RestoreOperation,
) (*harvesterv1.PVCRestore, error) {
	if err := op.Create(pr); err != nil {
		return pr, err
	}
	logrus.Infof("Successfully created resource for PVCRestore %s/%s", prh.pro.GetNamespace(pr), prh.pro.GetName(pr))
	return prh.pro.SetProcessing(pr)
}

func (prh *pvcrestoreHandler) handleReadyRestore(pr *harvesterv1.PVCRestore) (*harvesterv1.PVCRestore, error) {
	logrus.Infof("Handling ready restore for PVCRestore %s/%s", prh.pro.GetNamespace(pr), prh.pro.GetName(pr))

	updatedPr, err := prh.pro.SetReady(pr)
	if err != nil {
		return pr, err
	}

	updatedPr, err = prh.pro.SetSuccess(updatedPr, true)
	if err != nil {
		return pr, err
	}

	return updatedPr, nil
}

func (prh *pvcrestoreHandler) OnRemove(_ string, pr *harvesterv1.PVCRestore) (*harvesterv1.PVCRestore, error) {
	if pr == nil {
		return nil, nil
	}

	op, ok := prh.operations[prh.pro.GetType(pr)]
	if !ok {
		return pr, nil
	}

	if err := op.Delete(pr); err != nil {
		return pr, err
	}

	logrus.Infof("Successfully deleted resources PVCRestore %s/%s", prh.pro.GetNamespace(pr), prh.pro.GetName(pr))
	return pr, nil
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
