package backup

// NOTE: Harvester VM backup & restore is referenced from the Kubevirt's VM snapshot & restore,
// currently, we have decided to use custom VM backup and restore controllers because of the following issues:
// 1. live VM snapshot/backup should be supported, but it is prohibited on the Kubevirt side.
// 2. restore a VM backup to a new VM should be supported.
import (
	"context"
	"fmt"
	"reflect"
	"strings"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	"github.com/longhorn/longhorn-manager/types"
	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	kubevirtv1 "kubevirt.io/client-go/api/v1"

	harvesterapiv1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	ctlharvesterv1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	ctlkubevirtv1 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllonghornv1 "github.com/rancher/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	ctlsnapshotv1 "github.com/rancher/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1beta1"
	"github.com/rancher/harvester/pkg/settings"
)

const (
	backupContentControllerName = "harvester-vm-backup-content-controller"

	volumeSnapshotMissingEvent = "VolumeSnapshotMissing"
	volumeSnapshotCreateEvent  = "SuccessfulVolumeSnapshotCreate"

	vmBackupContentKindName = "VirtualMachineBackupContent"
	LonghornSystemNameSpace = "longhorn-system"
)

var vmBackupContentKind = harvesterapiv1.SchemeGroupVersion.WithKind(vmBackupContentKindName)

// RegisterContent register the vmbackupcontent and volumesnapshot controller
func RegisterContent(ctx context.Context, management *config.Management, opts config.Options) error {
	vmBackupContents := management.HarvesterFactory.Harvester().V1alpha1().VirtualMachineBackupContent()
	snapshots := management.SnapshotFactory.Snapshot().V1beta1().VolumeSnapshot()
	snapshotClass := management.SnapshotFactory.Snapshot().V1beta1().VolumeSnapshotClass()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	volumes := management.LonghornFactory.Longhorn().V1beta1().Volume()
	pvcs := management.CoreFactory.Core().V1().PersistentVolumeClaim()

	vmBackupController := &ContentHandler{
		snapshots:                 snapshots,
		snapshotCache:             snapshots.Cache(),
		vmBackupContents:          vmBackupContents,
		vmBackupContentCache:      vmBackupContents.Cache(),
		vmBackupContentController: vmBackupContents,
		vms:                       vms,
		vmsCache:                  vms.Cache(),
		volumeCache:               volumes.Cache(),
		volumes:                   volumes,
		snapshotClassCache:        snapshotClass.Cache(),
		pvcCache:                  pvcs.Cache(),
		recorder:                  management.NewRecorder(backupContentControllerName, "", ""),
	}

	vmBackupContents.OnChange(ctx, backupContentControllerName, vmBackupController.ContentOnChanged)
	snapshots.OnChange(ctx, backupContentControllerName, vmBackupController.updateVolumeSnapshotChanged)
	return nil
}

type ContentHandler struct {
	snapshots                 ctlsnapshotv1.VolumeSnapshotClient
	snapshotCache             ctlsnapshotv1.VolumeSnapshotCache
	vmBackupContents          ctlharvesterv1.VirtualMachineBackupContentClient
	vmBackupContentCache      ctlharvesterv1.VirtualMachineBackupContentCache
	vmBackupContentController ctlharvesterv1.VirtualMachineBackupContentController
	vms                       ctlkubevirtv1.VirtualMachineClient
	vmsCache                  ctlkubevirtv1.VirtualMachineCache
	volumeCache               ctllonghornv1.VolumeCache
	volumes                   ctllonghornv1.VolumeClient
	snapshotClassCache        ctlsnapshotv1.VolumeSnapshotClassCache
	pvcCache                  ctlcorev1.PersistentVolumeClaimCache
	recorder                  record.EventRecorder
}

// ContentOnChanged handles vm backup content on change and create volumesnapshot if not exist
func (h *ContentHandler) ContentOnChanged(key string, content *harvesterapiv1.VirtualMachineBackupContent) (*harvesterapiv1.VirtualMachineBackupContent, error) {
	if content == nil || content.DeletionTimestamp != nil {
		return nil, nil
	}

	if vmBackupContentReady(content) {
		return nil, nil
	}

	// check if the VM is off and volumes are detached, if yes mount it before creating the volume snapshot
	vmCpy := content.Spec.Source.DeepCopy()
	vm, err := h.vmsCache.Get(vmCpy.Namespace, vmCpy.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Errorf("failed to find backup vm %s", vmCpy.Name)
			return nil, nil
		}
		return nil, err
	}

	if !vm.Status.Ready || !vm.Status.Created {
		if err := h.reconcileLonghornVolumes(vm); err != nil {
			return content, err
		}
	}

	volBackupStatus := make([]harvesterapiv1.VolumeBackupStatus, 0, len(content.Spec.VolumeBackups))
	var deletedSnapshots, skippedSnapshots []string

	var contentReady = isContentReady(content)
	var contentError = isContentError(content)

	// create CSI volume snapshots
	for _, volumeBackup := range content.Spec.VolumeBackups {
		if volumeBackup.Name == nil {
			continue
		}

		var vsName = *volumeBackup.Name

		volumeSnapshot, err := h.getVolumeSnapshot(content.Namespace, vsName)
		if err != nil {
			return nil, err
		}

		if volumeSnapshot == nil {
			// check if snapshot was deleted
			if contentReady {
				logrus.Warningf("VolumeSnapshot %s no longer exists", vsName)
				h.recorder.Eventf(
					content,
					corev1.EventTypeWarning,
					volumeSnapshotMissingEvent,
					"VolumeSnapshot %s no longer exists",
					vsName,
				)
				deletedSnapshots = append(deletedSnapshots, vsName)
				continue
			}

			if contentError {
				logrus.Infof("Not creating snapshot %s because content in error state", vsName)
				skippedSnapshots = append(skippedSnapshots, vsName)
				continue
			}

			volumeSnapshot, err = h.createVolumeSnapshot(content, volumeBackup)
			if err != nil {
				return nil, err
			}
		}

		vss := harvesterapiv1.VolumeBackupStatus{
			VolumeBackupName: volumeSnapshot.Name,
		}

		if volumeSnapshot.Status != nil {
			vss.ReadyToUse = volumeSnapshot.Status.ReadyToUse
			vss.CreationTime = volumeSnapshot.Status.CreationTime
			vss.Error = translateError(volumeSnapshot.Status.Error)
		}

		volBackupStatus = append(volBackupStatus, vss)
	}

	var ready = true
	var errorMessage = ""
	contentCpy := content.DeepCopy()
	if contentCpy.Status == nil {
		contentCpy.Status = &harvesterapiv1.VirtualMachineBackupContentStatus{}
	}

	if len(deletedSnapshots) > 0 {
		ready = false
		errorMessage = fmt.Sprintf("volumeSnapshots (%s) missing", strings.Join(deletedSnapshots, ","))
	} else if len(skippedSnapshots) > 0 {
		ready = false
		errorMessage = fmt.Sprintf("volumeSnapshots (%s) skipped because in error state", strings.Join(skippedSnapshots, ","))
	} else {
		for _, vss := range volBackupStatus {
			if vss.ReadyToUse == nil || !*vss.ReadyToUse {
				ready = false
			}

			if vss.Error != nil {
				errorMessage = "VolumeSnapshot in error state"
				break
			}
		}
	}

	if ready && (contentCpy.Status.ReadyToUse == nil || !*contentCpy.Status.ReadyToUse) {
		contentCpy.Status.CreationTime = currentTime()
	}

	// check if need to update the error status
	if errorMessage != "" && (contentCpy.Status.Error == nil || contentCpy.Status.Error.Message == nil || *contentCpy.Status.Error.Message != errorMessage) {
		contentCpy.Status.Error = &harvesterapiv1.Error{
			Time:    currentTime(),
			Message: &errorMessage,
		}
	}

	contentCpy.Status.ReadyToUse = &ready
	contentCpy.Status.VolumeBackupStatus = volBackupStatus

	if !reflect.DeepEqual(content, contentCpy) {
		return h.vmBackupContents.Update(contentCpy)
	}

	return nil, nil
}

func (h *ContentHandler) getVolumeSnapshot(namespace, name string) (*snapshotv1.VolumeSnapshot, error) {
	snapshot, err := h.snapshotCache.Get(namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return snapshot, nil
}

func (h *ContentHandler) createVolumeSnapshot(content *harvesterapiv1.VirtualMachineBackupContent, volumeBackup harvesterapiv1.VolumeBackup) (*snapshotv1.VolumeSnapshot, error) {
	logrus.Debugf("attempting to create VolumeSnapshot %s", *volumeBackup.Name)

	sc, err := h.snapshotClassCache.Get(settings.VolumeSnapshotClass.Get())
	if err != nil {
		return nil, fmt.Errorf("%s/%s VolumeSnapshot requested but no storage class, err: %s",
			content.Namespace, volumeBackup.PersistentVolumeClaim.Name, err.Error())
	}

	snapshot := &snapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *volumeBackup.Name,
			Namespace: content.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         harvesterapiv1.SchemeGroupVersion.String(),
					Kind:               vmBackupContentKindName,
					Name:               content.Name,
					UID:                content.UID,
					Controller:         pointer.BoolPtr(true),
					BlockOwnerDeletion: pointer.BoolPtr(true),
				},
			},
		},
		Spec: snapshotv1.VolumeSnapshotSpec{
			Source: snapshotv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: &volumeBackup.PersistentVolumeClaim.Name,
			},
			VolumeSnapshotClassName: pointer.StringPtr(sc.Name),
		},
	}

	volumeSnapshot, err := h.snapshots.Create(snapshot)
	if err != nil {
		return nil, err
	}

	h.recorder.Eventf(
		content,
		corev1.EventTypeNormal,
		volumeSnapshotCreateEvent,
		"Successfully created VolumeSnapshot %s",
		snapshot.Name,
	)

	return volumeSnapshot, nil
}

func (h *ContentHandler) updateVolumeSnapshotChanged(key string, snapshot *snapshotv1.VolumeSnapshot) (*snapshotv1.VolumeSnapshot, error) {
	if snapshot == nil || snapshot.DeletionTimestamp != nil {
		return nil, nil
	}

	controllerRef := metav1.GetControllerOf(snapshot)

	// If it has a ControllerRef, that's all that matters.
	if controllerRef != nil {
		content := h.resolveVolSnapshotRef(snapshot.Namespace, controllerRef)
		if content == nil {
			return nil, nil
		}
		h.vmBackupContentController.Enqueue(content.Namespace, content.Name)
	}
	return nil, nil
}

// resolveVolSnapshotRef returns the controller referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching controller
// of the correct Kind.
func (h *ContentHandler) resolveVolSnapshotRef(namespace string, controllerRef *metav1.OwnerReference) *harvesterapiv1.VirtualMachineBackupContent {
	// We can't look up by UID, so look up by Name and then verify UID.
	// Don't even try to look up by Name if it's the wrong Kind.
	if controllerRef.Kind != vmBackupContentKind.Kind {
		return nil
	}
	content, err := h.vmBackupContentCache.Get(namespace, controllerRef.Name)
	if err != nil {
		return nil
	}
	if content.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	return content
}

func (h *ContentHandler) reconcileLonghornVolumes(vm *kubevirtv1.VirtualMachine) error {
	for _, vol := range vm.Spec.Template.Spec.Volumes {
		name := ""
		if vol.DataVolume != nil {
			name = vol.DataVolume.Name
		} else if vol.PersistentVolumeClaim != nil {
			name = vol.PersistentVolumeClaim.ClaimName
		} else {
			continue
		}

		pvc, err := h.pvcCache.Get(vm.Namespace, name)
		if err != nil {
			return fmt.Errorf("failed to get pvc %s/%s, error: %s", name, vm.Namespace, err.Error())
		}

		volume, err := h.volumeCache.Get(LonghornSystemNameSpace, pvc.Spec.VolumeName)
		if err != nil {
			return fmt.Errorf("failed to get volume %s/%s, error: %s", name, vm.Namespace, err.Error())
		}

		volCpy := volume.DeepCopy()
		if volume.Status.State == types.VolumeStateDetached || volume.Status.State == types.VolumeStateDetaching {
			volCpy.Spec.NodeID = volume.Status.OwnerID
		}

		if !reflect.DeepEqual(volCpy, volume) {
			logrus.Infof("mount detached volume %s to the node %s", volCpy.Name, volCpy.Spec.NodeID)
			if _, err = h.volumes.Update(volCpy); err != nil {
				return err
			}
		}
	}
	return nil
}
