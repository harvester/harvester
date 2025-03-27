package backup

// Harvester VM backup & restore controllers helps to manage the VM backup & restore by leveraging
// the VolumeSnapshot functionality of Kubernetes CSI drivers with built-in storage driver longhorn.
// Currently, the following features are supported:
// 1. support VM live & offline backup to the supported backupTarget(i.e, nfs_v4 or s3 storage server).
// 2. restore a backup to a new VM or replacing it with the existing VM is supported.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	// Although we don't use following drivers directly, we need to import them to register drivers.
	// NFS Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/nfs/nfs.go#L47-L51
	// S3 Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/s3/s3.go#L33-L37
	_ "github.com/longhorn/backupstore/nfs" //nolint
	_ "github.com/longhorn/backupstore/s3"  //nolint
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/scheme"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllonghornv2 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	backupControllerName         = "harvester-vm-backup-controller"
	snapshotControllerName       = "volume-snapshot-controller"
	longhornBackupControllerName = "longhorn-backup-controller"
	pvcControllerName            = "harvester-pvc-controller"
	vmBackupKindName             = "VirtualMachineBackup"

	volumeSnapshotCreateEvent = "VolumeSnapshotCreated"

	backupTargetAnnotation       = "backup.harvesterhci.io/backup-target"
	backupBucketNameAnnotation   = "backup.harvesterhci.io/bucket-name"
	backupBucketRegionAnnotation = "backup.harvesterhci.io/bucket-region"

	defaultFreezeDuration = 1 * time.Second
	snapRevise            = "-snap-revise"
)

var vmBackupKind = harvesterv1.SchemeGroupVersion.WithKind(vmBackupKindName)

// RegisterBackup register the vmBackup and volumeSnapshot controller
func RegisterBackup(ctx context.Context, management *config.Management, _ config.Options) error {
	vmBackups := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
	pv := management.CoreFactory.Core().V1().PersistentVolume()
	pvc := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	secrets := management.CoreFactory.Core().V1().Secret()
	storageClasses := management.StorageFactory.Storage().V1().StorageClass()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	vmis := management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	lhbackups := management.LonghornFactory.Longhorn().V1beta2().Backup()
	volumes := management.LonghornFactory.Longhorn().V1beta2().Volume()
	snapshots := management.SnapshotFactory.Snapshot().V1().VolumeSnapshot()
	snapshotContents := management.SnapshotFactory.Snapshot().V1().VolumeSnapshotContent()
	snapshotClass := management.SnapshotFactory.Snapshot().V1().VolumeSnapshotClass()

	virtSubsrcConfig := rest.CopyConfig(management.RestConfig)
	virtSubsrcConfig.GroupVersion = &k8sschema.GroupVersion{Group: "subresources.kubevirt.io", Version: "v1"}
	virtSubsrcConfig.APIPath = "/apis"
	virtSubsrcConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	virtSubresourceClient, err := rest.RESTClientFor(virtSubsrcConfig)
	if err != nil {
		return err
	}

	vmBackupController := &Handler{
		vmBackups:                 vmBackups,
		vmBackupController:        vmBackups,
		vmBackupCache:             vmBackups.Cache(),
		pvCache:                   pv.Cache(),
		pvcCache:                  pvc.Cache(),
		secretCache:               secrets.Cache(),
		storageClassCache:         storageClasses.Cache(),
		vms:                       vms,
		vmsCache:                  vms.Cache(),
		vmis:                      vmis,
		vmisCache:                 vmis.Cache(),
		lhbackupCache:             lhbackups.Cache(),
		volumeCache:               volumes.Cache(),
		snapshots:                 snapshots,
		snapshotCache:             snapshots.Cache(),
		snapshotContents:          snapshotContents,
		snapshotContentCache:      snapshotContents.Cache(),
		snapshotClassCache:        snapshotClass.Cache(),
		virtSubresourceRestClient: virtSubresourceClient,
		recorder:                  management.NewRecorder(backupControllerName, "", ""),
	}

	vmBackups.OnChange(ctx, backupControllerName, vmBackupController.OnBackupChange)
	vmBackups.OnRemove(ctx, backupControllerName, vmBackupController.OnBackupRemove)
	snapshots.OnChange(ctx, snapshotControllerName, vmBackupController.updateVolumeSnapshotChanged)
	lhbackups.OnChange(ctx, longhornBackupControllerName, vmBackupController.OnLHBackupChanged)
	return nil
}

type Handler struct {
	vmBackups                 ctlharvesterv1.VirtualMachineBackupClient
	vmBackupCache             ctlharvesterv1.VirtualMachineBackupCache
	vmBackupController        ctlharvesterv1.VirtualMachineBackupController
	vms                       ctlkubevirtv1.VirtualMachineClient
	vmsCache                  ctlkubevirtv1.VirtualMachineCache
	vmis                      ctlkubevirtv1.VirtualMachineInstanceClient
	vmisCache                 ctlkubevirtv1.VirtualMachineInstanceCache
	pvCache                   ctlcorev1.PersistentVolumeCache
	pvcCache                  ctlcorev1.PersistentVolumeClaimCache
	secretCache               ctlcorev1.SecretCache
	storageClassCache         ctlstoragev1.StorageClassCache
	lhbackupCache             ctllonghornv2.BackupCache
	volumeCache               ctllonghornv2.VolumeCache
	snapshots                 ctlsnapshotv1.VolumeSnapshotClient
	snapshotCache             ctlsnapshotv1.VolumeSnapshotCache
	snapshotContents          ctlsnapshotv1.VolumeSnapshotContentClient
	snapshotContentCache      ctlsnapshotv1.VolumeSnapshotContentCache
	snapshotClassCache        ctlsnapshotv1.VolumeSnapshotClassCache
	virtSubresourceRestClient rest.Interface
	recorder                  record.EventRecorder
}

// OnBackupChange handles vm backup object on change and reconcile vm backup status
func (h *Handler) OnBackupChange(_ string, vmBackup *harvesterv1.VirtualMachineBackup) (*harvesterv1.VirtualMachineBackup, error) {
	if vmBackup == nil || vmBackup.DeletionTimestamp != nil {
		return nil, nil
	}

	if IsBackupReady(vmBackup) {
		return nil, h.handleBackupReady(vmBackup)
	}

	logrus.Debugf("OnBackupChange: vmBackup name:%s", vmBackup.Name)
	// set vmBackup init status
	if isBackupMissingStatus(vmBackup) {
		// We cannot get VM outside this block, because we also can sync VMBackup from remote target.
		// A VMBackup without status is a new VMBackup, so it must have sourceVM.
		sourceVM, err := h.getBackupSource(vmBackup)
		if err != nil {
			return nil, h.setStatusError(vmBackup, err)
		}

		if err = h.initBackup(vmBackup, sourceVM); err != nil {
			return nil, h.setStatusError(vmBackup, err)
		}

		return nil, nil
	}

	// TODO, make sure status is initialized, and "Lock" the source VM by adding a finalizer and setting snapshotInProgress in status

	_, csiDriverVolumeSnapshotClassMap, err := h.getCSIDriverMap(vmBackup)
	if err != nil {
		return nil, h.setStatusError(vmBackup, err)
	}

	// create volume snapshots if not exist
	if err := h.reconcileVolumeSnapshots(vmBackup, csiDriverVolumeSnapshotClassMap); err != nil {
		return nil, h.setStatusError(vmBackup, err)
	}

	// reconcile backup status of volume backups, validate if those volumeSnapshots are ready to use
	if err := h.updateConditions(vmBackup); err != nil {
		return nil, err
	}

	return nil, nil
}

// OnBackupRemove remove remote vm backup metadata
func (h *Handler) OnBackupRemove(_ string, vmBackup *harvesterv1.VirtualMachineBackup) (*harvesterv1.VirtualMachineBackup, error) {
	if vmBackup == nil || vmBackup.Status == nil || vmBackup.Status.BackupTarget == nil {
		return nil, nil
	}

	target, err := settings.DecodeBackupTarget(settings.BackupTargetSet.Get())
	if err != nil {
		return nil, err
	}

	if !target.IsDefaultBackupTarget() {
		if err := h.deleteVMBackupMetadata(vmBackup, target); err != nil {
			return nil, err
		}
	}

	// Since VolumeSnapshot and VolumeSnapshotContent has finalizers,
	// when we delete VM Backup and its backup target is not same as current backup target,
	// VolumeSnapshot and VolumeSnapshotContent may not be deleted immediately.
	// We should force delete them to avoid that users re-config backup target back and associated LH Backup may be deleted.
	if !util.IsBackupTargetSame(vmBackup.Status.BackupTarget, target) {
		if err := h.forceDeleteVolumeSnapshotAndContent(vmBackup.Namespace, vmBackup.Status.VolumeBackups); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (h *Handler) getBackupSource(vmBackup *harvesterv1.VirtualMachineBackup) (*kubevirtv1.VirtualMachine, error) {
	var (
		sourceVM *kubevirtv1.VirtualMachine
		err      error
	)

	switch vmBackup.Spec.Source.Kind {
	case kubevirtv1.VirtualMachineGroupVersionKind.Kind:
		sourceVM, err = h.vmsCache.Get(vmBackup.Namespace, vmBackup.Spec.Source.Name)
	default:
		err = fmt.Errorf("unsupported source: %+v", vmBackup.Spec.Source)
	}
	if err != nil {
		return nil, err
	} else if sourceVM.DeletionTimestamp != nil {
		return nil, fmt.Errorf("vm %s/%s is being deleted", vmBackup.Namespace, vmBackup.Spec.Source.Name)
	}

	return sourceVM, nil
}

func (h *Handler) getBackupSourceInstance(vmBackup *harvesterv1.VirtualMachineBackup) (*kubevirtv1.VirtualMachineInstance, error) {
	if vmBackup.Spec.Source.Kind != kubevirtv1.VirtualMachineGroupVersionKind.Kind {
		return nil, fmt.Errorf("unsupported source: %+v", vmBackup.Spec.Source)
	}

	sourceVMI, err := h.vmisCache.Get(vmBackup.Namespace, vmBackup.Spec.Source.Name)
	if err != nil {
		return nil, err
	}
	if sourceVMI.DeletionTimestamp != nil {
		return nil, fmt.Errorf("vm %s/%s is being deleted", vmBackup.Namespace, vmBackup.Spec.Source.Name)
	}

	return sourceVMI, nil
}

func (h *Handler) tryFreezeFS(backup *harvesterv1.VirtualMachineBackup) error {
	backup, err := h.withFreezeFSAnnotation(backup)
	if err != nil {
		return err
	}

	freezeText := backup.Annotations[util.AnnotationSnapshotFreezeFS]
	freeze, err := strconv.ParseBool(freezeText)
	if err != nil {
		return fmt.Errorf("annotation %q=%q, should be \"true\" or \"false\"", util.AnnotationSnapshotFreezeFS, freezeText)
	}

	if !freeze {
		return nil
	}

	sourceVMI, err := h.getBackupSourceInstance(backup)
	if apierrors.IsNotFound(err) {
		return errors.New("virtual machine must be running for qemu-guest-agent-assisted snapshot/backup")
	}
	if err != nil {
		return err
	}

	err = h.freezeFS(context.Background(), sourceVMI, defaultFreezeDuration)
	if err != nil {
		logrus.WithError(err).Warn("qemu-guest-agent fs-freeze")
		return errors.New("failed to freeze file system")
	}

	return nil
}

func (h *Handler) withFreezeFSAnnotation(backup *harvesterv1.VirtualMachineBackup) (*harvesterv1.VirtualMachineBackup, error) {
	sourceVMI, err := h.getBackupSourceInstance(backup)
	if apierrors.IsNotFound(err) {
		return h.saveFreezeFSAnnotation(backup, false)
	}
	if err != nil {
		return backup, err
	}

	hasAgent := false

	for _, condition := range sourceVMI.Status.Conditions {
		if condition.Type == kubevirtv1.VirtualMachineInstanceAgentConnected {
			hasAgent = condition.Status == corev1.ConditionTrue
			break
		}
	}

	return h.saveFreezeFSAnnotation(backup, hasAgent)
}

func (h *Handler) saveFreezeFSAnnotation(backup *harvesterv1.VirtualMachineBackup, value bool) (*harvesterv1.VirtualMachineBackup, error) {
	backupCopy := backup.DeepCopy()

	if backupCopy.Annotations == nil {
		backupCopy.Annotations = make(map[string]string)
	}

	backupCopy.Annotations[util.AnnotationSnapshotFreezeFS] = strconv.FormatBool(value)

	return h.vmBackups.Update(backupCopy)
}

func (h *Handler) freezeFS(ctx context.Context, vmi *kubevirtv1.VirtualMachineInstance, timeout time.Duration) error {
	body, err := json.Marshal(kubevirtv1.FreezeUnfreezeTimeout{UnfreezeTimeout: &metav1.Duration{
		Duration: timeout,
	}})
	if err != nil {
		return err
	}

	res := h.virtSubresourceRestClient.Put().
		Namespace(vmi.Namespace).
		Resource("virtualmachineinstances").
		Name(vmi.Name).
		SubResource("freeze").
		Body(body).
		Do(ctx)
	return res.Error()
}

// getVolumeBackups helps to build a list of VolumeBackup upon the volume list of backup VM
func (h *Handler) getVolumeBackups(backup *harvesterv1.VirtualMachineBackup, vm *kubevirtv1.VirtualMachine) ([]harvesterv1.VolumeBackup, error) {
	sourceVolumes := vm.Spec.Template.Spec.Volumes
	var volumeBackups = make([]harvesterv1.VolumeBackup, 0, len(sourceVolumes))

	for volumeName, pvcName := range volumeToPVCMappings(sourceVolumes) {
		pvc, err := h.getBackupPVC(vm.Namespace, pvcName)
		if err != nil {
			return nil, err
		}

		pv, err := h.pvCache.Get(pvc.Spec.VolumeName)
		if err != nil {
			return nil, err
		}

		if pv.Spec.PersistentVolumeSource.CSI == nil {
			return nil, fmt.Errorf("PV %s is not from CSI driver, cannot take a %s", pv.Name, backup.Spec.Type)
		}

		storageCapacity := pv.Spec.Capacity[corev1.ResourceStorage]
		volumeSize := storageCapacity.Value()

		volumeBackupName := fmt.Sprintf("%s-volume-%s", backup.Name, pvcName)

		vb := harvesterv1.VolumeBackup{
			Name:          &volumeBackupName,
			VolumeName:    volumeName,
			CSIDriverName: pv.Spec.PersistentVolumeSource.CSI.Driver,
			PersistentVolumeClaim: harvesterv1.PersistentVolumeClaimSourceSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:        pvc.ObjectMeta.Name,
					Namespace:   pvc.ObjectMeta.Namespace,
					Labels:      pvc.Labels,
					Annotations: pvc.Annotations,
				},
				Spec: pvc.Spec,
			},
			ReadyToUse: pointer.BoolPtr(false),
			VolumeSize: volumeSize,
		}

		volumeBackups = append(volumeBackups, vb)
	}

	return volumeBackups, nil
}

// getCSIDriverMap retrieves VolumeSnapshotClassName for each csi driver
func (h *Handler) getCSIDriverMap(backup *harvesterv1.VirtualMachineBackup) (map[string]string, map[string]snapshotv1.VolumeSnapshotClass, error) {
	csiDriverConfig := map[string]settings.CSIDriverInfo{}
	if err := json.Unmarshal([]byte(settings.CSIDriverConfig.GetDefault()), &csiDriverConfig); err != nil {
		return nil, nil, fmt.Errorf("unmarshal failed, error: %w, value: %s", err, settings.CSIDriverConfig.GetDefault())
	}
	tmpDriverConfig := map[string]settings.CSIDriverInfo{}
	if err := json.Unmarshal([]byte(settings.CSIDriverConfig.Get()), &tmpDriverConfig); err != nil {
		return nil, nil, fmt.Errorf("unmarshal failed, error: %w, value: %s", err, settings.CSIDriverConfig.Get())
	}

	for key, val := range tmpDriverConfig {
		if _, ok := csiDriverConfig[key]; !ok {
			csiDriverConfig[key] = val
		}
	}

	csiDriverVolumeSnapshotClassNameMap := map[string]string{}
	csiDriverVolumeSnapshotClassMap := map[string]snapshotv1.VolumeSnapshotClass{}
	for _, volumeBackup := range backup.Status.VolumeBackups {
		csiDriverName := volumeBackup.CSIDriverName
		if _, ok := csiDriverVolumeSnapshotClassNameMap[csiDriverName]; ok {
			continue
		}

		driverInfo, ok := csiDriverConfig[csiDriverName]
		if !ok {
			return nil, nil, fmt.Errorf("can't find CSI driver %s in setting CSIDriverInfo", csiDriverName)
		}
		volumeSnapshotClassName := ""
		switch backup.Spec.Type {
		case harvesterv1.Backup:
			volumeSnapshotClassName = driverInfo.BackupVolumeSnapshotClassName
		case harvesterv1.Snapshot:
			volumeSnapshotClassName = driverInfo.VolumeSnapshotClassName
		}
		volumeSnapshotClass, err := h.snapshotClassCache.Get(volumeSnapshotClassName)
		if err != nil {
			return nil, nil, fmt.Errorf("can't find volumeSnapshotClass %s for CSI driver %s", volumeSnapshotClassName, csiDriverName)
		}
		csiDriverVolumeSnapshotClassNameMap[csiDriverName] = volumeSnapshotClassName
		csiDriverVolumeSnapshotClassMap[csiDriverName] = *volumeSnapshotClass
	}

	return csiDriverVolumeSnapshotClassNameMap, csiDriverVolumeSnapshotClassMap, nil
}

// getSecretBackups helps to build a list of SecretBackup upon the secrets used by the backup VM
func (h *Handler) getSecretBackups(vm *kubevirtv1.VirtualMachine) ([]harvesterv1.SecretBackup, error) {
	secretRefs := []*corev1.LocalObjectReference{}

	for _, credential := range vm.Spec.Template.Spec.AccessCredentials {
		if sshPublicKey := credential.SSHPublicKey; sshPublicKey != nil && sshPublicKey.Source.Secret != nil {
			secretRefs = append(secretRefs, &corev1.LocalObjectReference{
				Name: sshPublicKey.Source.Secret.SecretName,
			})
		}
		if userPassword := credential.UserPassword; userPassword != nil && userPassword.Source.Secret != nil {
			secretRefs = append(secretRefs, &corev1.LocalObjectReference{
				Name: userPassword.Source.Secret.SecretName,
			})
		}
	}

	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.CloudInitNoCloud != nil && volume.CloudInitNoCloud.UserDataSecretRef != nil {
			secretRefs = append(secretRefs, volume.CloudInitNoCloud.UserDataSecretRef)
		}
		if volume.CloudInitNoCloud != nil && volume.CloudInitNoCloud.NetworkDataSecretRef != nil {
			secretRefs = append(secretRefs, volume.CloudInitNoCloud.NetworkDataSecretRef)
		}
	}

	secretBackups := []harvesterv1.SecretBackup{}
	secretBackupMap := map[string]bool{}
	for _, secretRef := range secretRefs {
		// users may put SecretRefs in a same secret, so we only keep one
		secretFullName := fmt.Sprintf("%s/%s", vm.Namespace, secretRef.Name)
		if v, ok := secretBackupMap[secretFullName]; ok && v {
			continue
		}

		secretBackup, err := h.getSecretBackupFromSecret(vm.Namespace, secretRef.Name)
		if err != nil {
			return nil, err
		}
		secretBackupMap[secretFullName] = true
		secretBackups = append(secretBackups, *secretBackup)
	}

	return secretBackups, nil
}

func (h *Handler) getSecretBackupFromSecret(namespace, name string) (*harvesterv1.SecretBackup, error) {
	secret, err := h.secretCache.Get(namespace, name)
	if err != nil {
		return nil, err
	}

	// Remove empty string. If there is empty string in secret, we will encounter error.
	// ref: https://github.com/harvester/harvester/issues/1536
	data := secret.DeepCopy().Data
	for k, v := range secret.Data {
		if len(v) == 0 {
			delete(data, k)
		}
	}

	return &harvesterv1.SecretBackup{Name: secret.Name, Data: data}, nil
}

// initBackup initialize VM backup status and annotation
func (h *Handler) initBackup(backup *harvesterv1.VirtualMachineBackup, vm *kubevirtv1.VirtualMachine) error {
	var err error
	backupCpy := backup.DeepCopy()
	backupCpy.Status = &harvesterv1.VirtualMachineBackupStatus{
		ReadyToUse: pointer.BoolPtr(false),
		SourceUID:  &vm.UID,
		SourceSpec: &harvesterv1.VirtualMachineSourceSpec{
			ObjectMeta: metav1.ObjectMeta{
				Name:        vm.ObjectMeta.Name,
				Namespace:   vm.ObjectMeta.Namespace,
				Annotations: vm.ObjectMeta.Annotations,
				Labels:      vm.ObjectMeta.Labels,
			},
			Spec: vm.Spec,
		},
	}

	if backupCpy.Status.VolumeBackups, err = h.getVolumeBackups(backup, vm); err != nil {
		return err
	}

	if backupCpy.Status.SecretBackups, err = h.getSecretBackups(vm); err != nil {
		return err
	}

	if backupCpy.Status.CSIDriverVolumeSnapshotClassNames, _, err = h.getCSIDriverMap(backupCpy); err != nil {
		return err
	}

	if backup.Spec.Type == harvesterv1.Backup {
		target, err := settings.DecodeBackupTarget(settings.BackupTargetSet.Get())
		if err != nil {
			return err
		}

		backupCpy.Status.BackupTarget = &harvesterv1.BackupTarget{
			Endpoint:     target.Endpoint,
			BucketName:   target.BucketName,
			BucketRegion: target.BucketRegion,
		}
	}

	if _, err := h.vmBackups.Update(backupCpy); err != nil {
		return err
	}
	return nil
}

func (h *Handler) getBackupPVC(namespace, name string) (*corev1.PersistentVolumeClaim, error) {
	pvc, err := h.pvcCache.Get(namespace, name)
	if err != nil {
		return nil, err
	}

	if pvc.Spec.VolumeName == "" {
		return nil, fmt.Errorf("unbound PVC %s/%s", pvc.Namespace, pvc.Name)
	}

	if pvc.Spec.StorageClassName == nil {
		return nil, fmt.Errorf("no storage class for PVC %s/%s", pvc.Namespace, pvc.Name)
	}

	return pvc, nil
}

// We only needs to freeze the fs if the LH backup doesn't exists,
// this can be a `bak` type VolumeSnapshot from or a pure VolumeSnapshot
func (h *Handler) freezeFsIfNeeded(vmBackup *harvesterv1.VirtualMachineBackup, vb harvesterv1.VolumeBackup) error {
	if vb.LonghornBackupName != nil {
		return nil
	}

	return h.tryFreezeFS(vmBackup)
}

// reconcileVolumeSnapshots create volume snapshot if not exist.
// For vm backup from a existent VM, we create volume snapshot from pvc.
// For vm backup from syncing vm backup metadata, we create volume snapshot from volume snapshot content.
func (h *Handler) reconcileVolumeSnapshots(vmBackup *harvesterv1.VirtualMachineBackup, csiDriverVolumeSnapshotClassMap map[string]snapshotv1.VolumeSnapshotClass) error {
	vmBackupCpy := vmBackup.DeepCopy()
	for i, volumeBackup := range vmBackupCpy.Status.VolumeBackups {
		if volumeBackup.Name == nil {
			continue
		}

		snapshotName := *volumeBackup.Name
		volumeSnapshot, err := h.getVolumeSnapshot(vmBackupCpy.Namespace, snapshotName)
		if err != nil {
			return err
		}

		if volumeSnapshot != nil && volumeSnapshot.DeletionTimestamp != nil {
			logrus.WithFields(logrus.Fields{
				"VolumeSnapshotNamespace": volumeSnapshot.Namespace,
				"VolumeSnapshotName":      volumeSnapshot.Name,
				"VMBackupNamespace":       vmBackup.Namespace,
				"VMBackupName":            vmBackup.Name,
			}).Info("volumeSnapshot is deleted, requeue vm backup again")
			h.vmBackupController.EnqueueAfter(vmBackup.Namespace, vmBackup.Name, 5*time.Second)
			return nil
		}

		if volumeSnapshot == nil {
			if err := h.freezeFsIfNeeded(vmBackupCpy, volumeBackup); err != nil {
				return err
			}

			volumeSnapshotClass := csiDriverVolumeSnapshotClassMap[volumeBackup.CSIDriverName]
			volumeSnapshot, err = h.createVolumeSnapshot(vmBackupCpy, volumeBackup, &volumeSnapshotClass)
			if err != nil {
				logrus.Debugf("create volumeSnapshot %s/%s error: %v", vmBackupCpy.Namespace, snapshotName, err)
				return err
			}
		}

		if volumeSnapshot.Status != nil {
			vmBackupCpy.Status.VolumeBackups[i].ReadyToUse = volumeSnapshot.Status.ReadyToUse
			vmBackupCpy.Status.VolumeBackups[i].CreationTime = volumeSnapshot.Status.CreationTime
			vmBackupCpy.Status.VolumeBackups[i].Error = translateError(volumeSnapshot.Status.Error)
		}

	}

	if !reflect.DeepEqual(vmBackup.Status, vmBackupCpy.Status) {
		if _, err := h.vmBackups.Update(vmBackupCpy); err != nil {
			return err
		}
	}

	return nil
}

func (h *Handler) getVolumeSnapshot(namespace, name string) (*snapshotv1.VolumeSnapshot, error) {
	snapshot, err := h.snapshotCache.Get(namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return snapshot, nil
}

func (h *Handler) createVolumeSnapshot(vmBackup *harvesterv1.VirtualMachineBackup, volumeBackup harvesterv1.VolumeBackup, volumeSnapshotClass *snapshotv1.VolumeSnapshotClass) (*snapshotv1.VolumeSnapshot, error) {
	var (
		sourceStorageClassName string
		sourceImageID          string
		sourceProvisioner      string
	)

	volumeSnapshotSource := snapshotv1.VolumeSnapshotSource{}
	// If LonghornBackupName exists, it means the VM Backup has associated LH Backup.
	// In this case, we should create volume snapshot from LH Backup instead of from current PVC.
	if volumeBackup.LonghornBackupName != nil {
		volumeSnapshotContent, err := h.createVolumeSnapshotContent(vmBackup, volumeBackup, volumeSnapshotClass)
		if err != nil {
			return nil, err
		}
		volumeSnapshotSource.VolumeSnapshotContentName = &volumeSnapshotContent.Name
	} else {
		pvc, err := h.pvcCache.Get(volumeBackup.PersistentVolumeClaim.ObjectMeta.Namespace, volumeBackup.PersistentVolumeClaim.ObjectMeta.Name)
		if err != nil {
			return nil, err
		}
		sourceStorageClassName = *pvc.Spec.StorageClassName
		sourceImageID = pvc.Annotations[util.AnnotationImageID]
		sourceProvisioner = util.GetProvisionedPVCProvisioner(pvc, h.storageClassCache)
		volumeSnapshotSource.PersistentVolumeClaimName = &volumeBackup.PersistentVolumeClaim.ObjectMeta.Name
	}

	snapshot := &snapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *volumeBackup.Name,
			Namespace: vmBackup.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: harvesterv1.SchemeGroupVersion.String(),
					Kind:       vmBackupKind.Kind,
					Name:       vmBackup.Name,
					UID:        vmBackup.UID,
					Controller: pointer.BoolPtr(true),
				},
			},
			Annotations: map[string]string{},
		},
		Spec: snapshotv1.VolumeSnapshotSpec{
			Source:                  volumeSnapshotSource,
			VolumeSnapshotClassName: pointer.StringPtr(volumeSnapshotClass.Name),
		},
	}

	if sourceStorageClassName != "" {
		snapshot.Annotations[util.AnnotationStorageClassName] = sourceStorageClassName
	}
	if sourceImageID != "" {
		snapshot.Annotations[util.AnnotationImageID] = sourceImageID
	}
	if sourceProvisioner != "" {
		snapshot.Annotations[util.AnnotationStorageProvisioner] = sourceProvisioner
	}

	logrus.WithFields(logrus.Fields{
		"namespace": vmBackup.Namespace,
		"name":      *volumeBackup.Name,
	}).Info("creating VolumeSnapshot")
	volumeSnapshot, err := h.snapshots.Create(snapshot)
	if err != nil {
		return nil, err
	}

	h.recorder.Eventf(
		vmBackup,
		corev1.EventTypeNormal,
		volumeSnapshotCreateEvent,
		"Successfully created VolumeSnapshot %s/%s",
		vmBackup.Namespace,
		snapshot.Name,
	)

	return volumeSnapshot, nil
}

func (h *Handler) createVolumeSnapshotContent(
	vmBackup *harvesterv1.VirtualMachineBackup,
	volumeBackup harvesterv1.VolumeBackup,
	snapshotClass *snapshotv1.VolumeSnapshotClass,
) (*snapshotv1.VolumeSnapshotContent, error) {
	logrus.Debugf("attempting to create VolumeSnapshotContent %s", getVolumeSnapshotContentName(volumeBackup))
	snapshotContent, err := h.snapshotContentCache.Get(getVolumeSnapshotContentName(volumeBackup))
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	} else if err == nil {
		return snapshotContent, nil
	}

	lhBackup, err := h.lhbackupCache.Get(util.LonghornSystemNamespaceName, *volumeBackup.LonghornBackupName)
	if err != nil {
		return nil, err
	}

	if lhBackup.Status.VolumeName == "" {
		return nil, fmt.Errorf("lhbackup %s is not populated for vmbackup %s/%s's volume %s",
			lhBackup.Name, vmBackup.Namespace, vmBackup.Name, *volumeBackup.Name)
	}

	// Ref: https://longhorn.io/docs/1.2.3/snapshots-and-backups/csi-snapshot-support/restore-a-backup-via-csi/#restore-a-backup-that-has-no-associated-volumesnapshot
	snapshotHandle := fmt.Sprintf("bak://%s/%s", lhBackup.Status.VolumeName, lhBackup.Name)

	logrus.WithFields(logrus.Fields{
		"name": getVolumeSnapshotContentName(volumeBackup),
	}).Info("creating VolumeSnapshotContent")
	return h.snapshotContents.Create(&snapshotv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getVolumeSnapshotContentName(volumeBackup),
			Namespace: vmBackup.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: harvesterv1.SchemeGroupVersion.String(),
					Kind:       vmBackupKind.Kind,
					Name:       vmBackup.Name,
					UID:        vmBackup.UID,
					Controller: pointer.BoolPtr(true),
				},
			},
		},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			Driver:         "driver.longhorn.io",
			DeletionPolicy: snapshotv1.VolumeSnapshotContentDelete,
			Source: snapshotv1.VolumeSnapshotContentSource{
				SnapshotHandle: pointer.StringPtr(snapshotHandle),
			},
			VolumeSnapshotClassName: pointer.StringPtr(snapshotClass.Name),
			VolumeSnapshotRef: corev1.ObjectReference{
				Name:      *volumeBackup.Name,
				Namespace: vmBackup.Namespace,
			},
		},
	})
}

func (h *Handler) setStatusError(vmBackup *harvesterv1.VirtualMachineBackup, err error) error {
	vmBackupCpy := vmBackup.DeepCopy()
	if vmBackupCpy.Status == nil {
		vmBackupCpy.Status = &harvesterv1.VirtualMachineBackupStatus{}
	}

	vmBackupCpy.Status.Error = &harvesterv1.Error{
		Time:    currentTime(),
		Message: pointer.StringPtr(err.Error()),
	}
	updateBackupCondition(vmBackupCpy, newProgressingCondition(corev1.ConditionFalse, "Error", err.Error()))
	updateBackupCondition(vmBackupCpy, newReadyCondition(corev1.ConditionFalse, "", "Not Ready"))

	if _, updateErr := h.vmBackups.Update(vmBackupCpy); updateErr != nil {
		return updateErr
	}
	return err
}

func (h *Handler) deleteVMBackupMetadata(vmBackup *harvesterv1.VirtualMachineBackup, target *settings.BackupTarget) error {
	var err error
	if target == nil {
		if target, err = settings.DecodeBackupTarget(settings.BackupTargetSet.Get()); err != nil {
			return err
		}
	}

	// when backup target has been reset to default, skip following
	if target.IsDefaultBackupTarget() {
		logrus.Debugf("vmBackup delete:%s, backup target is default, skip", vmBackup.Name)
		return nil
	}

	if !util.IsBackupTargetSame(vmBackup.Status.BackupTarget, target) {
		return nil
	}

	bsDriver, err := util.GetBackupStoreDriver(h.secretCache, target)
	if err != nil {
		return err
	}

	destURL := getVMBackupMetadataFilePath(vmBackup.Namespace, vmBackup.Name)
	if exist := bsDriver.FileExists(destURL); exist {
		logrus.Debugf("delete vm backup metadata %s/%s in backup target %s", vmBackup.Namespace, vmBackup.Name, target.Type)
		return bsDriver.Remove(destURL)
	}

	return nil
}

func (h *Handler) uploadVMBackupMetadata(vmBackup *harvesterv1.VirtualMachineBackup) error {
	// if users don't update VMBackup CRD, we may lose backup target data.
	if vmBackup.Status.BackupTarget == nil {
		return fmt.Errorf("no backup target in vmbackup.status")
	}

	target, err := settings.DecodeBackupTarget(settings.BackupTargetSet.Get())
	if err != nil {
		return err
	}

	// when current backup target is default, skip following steps
	// if backup target is default, IsBackupTargetSame is true when vmBackup.Status.BackupTarget is also default value
	if target.IsDefaultBackupTarget() {
		return nil
	}

	if !util.IsBackupTargetSame(vmBackup.Status.BackupTarget, target) {
		return nil
	}

	bsDriver, err := util.GetBackupStoreDriver(h.secretCache, target)
	if err != nil {
		return err
	}

	vmBackupMetadata := &VirtualMachineBackupMetadata{
		Name:          vmBackup.Name,
		Namespace:     vmBackup.Namespace,
		BackupSpec:    vmBackup.Spec,
		VMSourceSpec:  vmBackup.Status.SourceSpec,
		VolumeBackups: sanitizeVolumeBackups(vmBackup.Status.VolumeBackups),
		SecretBackups: vmBackup.Status.SecretBackups,
	}
	if vmBackup.Namespace == "" {
		vmBackupMetadata.Namespace = metav1.NamespaceDefault
	}

	j, err := json.Marshal(vmBackupMetadata)
	if err != nil {
		return err
	}

	shouldUpload := true
	destURL := getVMBackupMetadataFilePath(vmBackup.Namespace, vmBackup.Name)
	if bsDriver.FileExists(destURL) {
		if remoteVMBackupMetadata, err := loadBackupMetadataInBackupTarget(destURL, bsDriver); err != nil {
			return err
		} else if reflect.DeepEqual(vmBackupMetadata, remoteVMBackupMetadata) {
			shouldUpload = false
		}
	}

	if shouldUpload {
		logrus.Debugf("upload vm backup metadata %s/%s to backup target %s", vmBackup.Namespace, vmBackup.Name, target.Type)
		if err := bsDriver.Write(destURL, bytes.NewReader(j)); err != nil {
			return err
		}
	}

	vmBackupCopy := vmBackup.DeepCopy()
	updateBackupCondition(vmBackupCopy, harvesterv1.Condition{
		Type:               harvesterv1.BackupConditionMetadataReady,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: currentTime().Format(time.RFC3339),
	})
	if !reflect.DeepEqual(vmBackup.Status, vmBackupCopy.Status) {
		_, err = h.vmBackups.Update(vmBackupCopy)
		return err
	}
	return nil
}

func sanitizeVolumeBackups(volumeBackups []harvesterv1.VolumeBackup) []harvesterv1.VolumeBackup {
	for i := 0; i < len(volumeBackups); i++ {
		volumeBackups[i].ReadyToUse = nil
		volumeBackups[i].CreationTime = nil
		volumeBackups[i].Error = nil
	}
	return volumeBackups
}

func volumeToPVCMappings(volumes []kubevirtv1.Volume) map[string]string {
	pvcs := map[string]string{}

	for _, volume := range volumes {
		var pvcName string

		if volume.PersistentVolumeClaim != nil {
			pvcName = volume.PersistentVolumeClaim.ClaimName
		} else {
			continue
		}

		pvcs[volume.Name] = pvcName
	}

	return pvcs
}

func (h *Handler) forceDeleteVolumeSnapshotAndContent(namespace string, volumeBackups []harvesterv1.VolumeBackup) error {
	for _, volumeBackup := range volumeBackups {
		volumeSnapshot, err := h.getVolumeSnapshot(namespace, *volumeBackup.Name)
		if err != nil {
			return err
		}
		if volumeSnapshot == nil {
			continue
		}

		// remove finalizers in VolumeSnapshot and force delete it
		volumeSnapshotCpy := volumeSnapshot.DeepCopy()
		volumeSnapshot.Finalizers = []string{}
		logrus.Debugf("remove finalizers in volume snapshot %s/%s", namespace, *volumeBackup.Name)
		if _, err := h.snapshots.Update(volumeSnapshotCpy); err != nil {
			return err
		}
		logrus.Debugf("delete volume snapshot %s/%s", namespace, *volumeBackup.Name)
		if err := h.snapshots.Delete(namespace, *volumeBackup.Name, metav1.NewDeleteOptions(0)); err != nil {
			return err
		}

		if volumeSnapshot.Status == nil || volumeSnapshot.Status.BoundVolumeSnapshotContentName == nil {
			continue
		}
		volumeSnapshotContet, err := h.snapshotContentCache.Get(*volumeSnapshot.Status.BoundVolumeSnapshotContentName)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			continue
		}

		// remove finalizers in VolumeSnapshotContent and force delete it
		volumeSnapshotContetCpy := volumeSnapshotContet.DeepCopy()
		volumeSnapshotContetCpy.Finalizers = []string{}
		logrus.Debugf("remove finalizers in volume snapshot content %s", *volumeSnapshot.Status.BoundVolumeSnapshotContentName)
		if _, err := h.snapshotContents.Update(volumeSnapshotContetCpy); err != nil {
			return err
		}
		logrus.Debugf("delete volume snapshot content %s", *volumeSnapshot.Status.BoundVolumeSnapshotContentName)
		if err := h.snapshotContents.Delete(*volumeSnapshot.Status.BoundVolumeSnapshotContentName, metav1.NewDeleteOptions(0)); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) configureBackupTargetOnStatus(vmBackup *harvesterv1.VirtualMachineBackup) (*harvesterv1.VirtualMachineBackup, error) {
	if !isBackupTargetOnAnnotation(vmBackup) {
		return vmBackup, nil
	}

	logrus.Debugf("configure backup target from annotation to status for vm backup %s/%s", vmBackup.Namespace, vmBackup.Name)
	vmBackupCpy := vmBackup.DeepCopy()
	vmBackupCpy.Status.BackupTarget = &harvesterv1.BackupTarget{
		Endpoint:     vmBackup.Annotations[backupTargetAnnotation],
		BucketName:   vmBackup.Annotations[backupBucketNameAnnotation],
		BucketRegion: vmBackup.Annotations[backupBucketRegionAnnotation],
	}
	return h.vmBackups.Update(vmBackupCpy)
}

func (h *Handler) handleBackupReady(vmBackup *harvesterv1.VirtualMachineBackup) error {
	// We add CSIDriverVolumeSnapshotClassNameMap since v1.1.0.
	// For backport to v1.0.x, we construct the map from VolumeBackups.
	var err error
	if vmBackup, err = h.configureCSIDriverVolumeSnapshotClassNames(vmBackup); err != nil {
		return err
	}

	// only backup type needs to configure backup target and upload metadata
	if vmBackup.Spec.Type == harvesterv1.Snapshot {
		return nil
	}

	// We've changed backup target information to status since v1.0.0.
	// For backport to v0.3.0, we move backup target information from annotation to status.
	if vmBackup, err = h.configureBackupTargetOnStatus(vmBackup); err != nil {
		return err
	}

	// generate vm backup metadata and upload to backup target
	return h.uploadVMBackupMetadata(vmBackup)
}

func (h *Handler) configureCSIDriverVolumeSnapshotClassNames(vmBackup *harvesterv1.VirtualMachineBackup) (*harvesterv1.VirtualMachineBackup, error) {
	if len(vmBackup.Status.CSIDriverVolumeSnapshotClassNames) != 0 {
		return vmBackup, nil
	}

	logrus.Debugf("configure csiDriverVolumeSnapshotClassNames from volumeBackups for vm backup %s/%s", vmBackup.Namespace, vmBackup.Name)
	var err error
	vmBackupCpy := vmBackup.DeepCopy()
	if vmBackupCpy.Status.CSIDriverVolumeSnapshotClassNames, _, err = h.getCSIDriverMap(vmBackup); err != nil {
		return nil, err
	}
	return h.vmBackups.Update(vmBackupCpy)
}
