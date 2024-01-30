package manager

import (
	"fmt"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/resource"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	"github.com/longhorn/longhorn-manager/scheduler"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

type VolumeManager struct {
	ds        *datastore.DataStore
	scheduler *scheduler.ReplicaScheduler

	currentNodeID string

	proxyConnCounter util.Counter
}

func NewVolumeManager(currentNodeID string, ds *datastore.DataStore, proxyConnCounter util.Counter) *VolumeManager {
	return &VolumeManager{
		ds:        ds,
		scheduler: scheduler.NewReplicaScheduler(ds),

		currentNodeID: currentNodeID,

		proxyConnCounter: proxyConnCounter,
	}
}

func (m *VolumeManager) GetCurrentNodeID() string {
	return m.currentNodeID
}

func (m *VolumeManager) Node2APIAddress(nodeID string) (string, error) {
	nodeIPMap, err := m.ds.GetManagerNodeIPMap()
	if err != nil {
		return "", err
	}
	ip, exists := nodeIPMap[nodeID]
	if !exists {
		return "", fmt.Errorf("cannot find longhorn manager on node %v", nodeID)
	}
	return types.GetAPIServerAddressFromIP(ip), nil
}

func (m *VolumeManager) List() (map[string]*longhorn.Volume, error) {
	return m.ds.ListVolumes()
}

func (m *VolumeManager) ListSorted() ([]*longhorn.Volume, error) {
	volumeMap, err := m.List()
	if err != nil {
		return []*longhorn.Volume{}, err
	}

	volumes := make([]*longhorn.Volume, len(volumeMap))
	volumeNames, err := util.SortKeys(volumeMap)
	if err != nil {
		return []*longhorn.Volume{}, err
	}
	for i, volumeName := range volumeNames {
		volumes[i] = volumeMap[volumeName]
	}
	return volumes, nil
}

func (m *VolumeManager) Get(vName string) (*longhorn.Volume, error) {
	return m.ds.GetVolume(vName)
}

func (m *VolumeManager) GetEngines(vName string) (map[string]*longhorn.Engine, error) {
	return m.ds.ListVolumeEngines(vName)
}

func (m *VolumeManager) GetEnginesSorted(vName string) ([]*longhorn.Engine, error) {
	engineMap, err := m.ds.ListVolumeEngines(vName)
	if err != nil {
		return []*longhorn.Engine{}, err
	}

	engines := make([]*longhorn.Engine, len(engineMap))
	engineNames, err := util.SortKeys(engineMap)
	if err != nil {
		return []*longhorn.Engine{}, err
	}
	for i, engineName := range engineNames {
		engines[i] = engineMap[engineName]
	}
	return engines, nil
}

func (m *VolumeManager) GetReplicas(vName string) (map[string]*longhorn.Replica, error) {
	return m.ds.ListVolumeReplicas(vName)
}

func (m *VolumeManager) GetReplicasSorted(vName string) ([]*longhorn.Replica, error) {
	replicaMap, err := m.ds.ListVolumeReplicas(vName)
	if err != nil {
		return []*longhorn.Replica{}, err
	}

	replicas := make([]*longhorn.Replica, len(replicaMap))
	replicaNames, err := util.SortKeys(replicaMap)
	if err != nil {
		return []*longhorn.Replica{}, err
	}
	for i, replicaName := range replicaNames {
		replicas[i] = replicaMap[replicaName]
	}
	return replicas, nil
}

func (m *VolumeManager) Create(name string, spec *longhorn.VolumeSpec, recurringJobSelector []longhorn.VolumeRecurringJob) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to create volume %v", name)
		if err != nil {
			logrus.Errorf("manager: unable to create volume %v: %+v: %v", name, spec, err)
		}
	}()

	labels := map[string]string{}
	for _, job := range recurringJobSelector {
		labelType := types.LonghornLabelRecurringJob
		if job.IsGroup {
			labelType = types.LonghornLabelRecurringJobGroup
		}
		key := types.GetRecurringJobLabelKey(labelType, job.Name)
		labels[key] = types.LonghornLabelValueEnabled
	}

	if spec.DataSource != "" {
		if err := m.verifyDataSourceForVolumeCreation(spec.DataSource, spec.Size); err != nil {
			return nil, err
		}
	}

	v = &longhorn.Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: longhorn.VolumeSpec{
			Size:                        spec.Size,
			AccessMode:                  spec.AccessMode,
			Migratable:                  spec.Migratable,
			Encrypted:                   spec.Encrypted,
			Frontend:                    spec.Frontend,
			EngineImage:                 "",
			FromBackup:                  spec.FromBackup,
			RestoreVolumeRecurringJob:   spec.RestoreVolumeRecurringJob,
			DataSource:                  spec.DataSource,
			NumberOfReplicas:            spec.NumberOfReplicas,
			ReplicaAutoBalance:          spec.ReplicaAutoBalance,
			DataLocality:                spec.DataLocality,
			StaleReplicaTimeout:         spec.StaleReplicaTimeout,
			BackingImage:                spec.BackingImage,
			Standby:                     spec.Standby,
			DiskSelector:                spec.DiskSelector,
			NodeSelector:                spec.NodeSelector,
			RevisionCounterDisabled:     spec.RevisionCounterDisabled,
			SnapshotDataIntegrity:       spec.SnapshotDataIntegrity,
			BackupCompressionMethod:     spec.BackupCompressionMethod,
			UnmapMarkSnapChainRemoved:   spec.UnmapMarkSnapChainRemoved,
			ReplicaSoftAntiAffinity:     spec.ReplicaSoftAntiAffinity,
			ReplicaZoneSoftAntiAffinity: spec.ReplicaZoneSoftAntiAffinity,
			BackendStoreDriver:          spec.BackendStoreDriver,
			OfflineReplicaRebuilding:    spec.OfflineReplicaRebuilding,
		},
	}

	v, err = m.ds.CreateVolume(v)
	if err != nil {
		return nil, err
	}
	logrus.Infof("Created volume %v: %+v", v.Name, v.Spec)
	return v, nil
}

func (m *VolumeManager) Delete(name string) error {
	if err := m.ds.DeleteVolume(name); err != nil {
		return err
	}
	logrus.Infof("Deleted volume %v", name)
	return nil
}

func (m *VolumeManager) Attach(name, nodeID string, disableFrontend bool, attachedBy, attacherType, attachmentID string) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to attach volume %v to %v", name, nodeID)
	}()

	node, err := m.ds.GetNode(nodeID)
	if err != nil {
		return nil, err
	}
	readyCondition := types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeReady)
	if readyCondition.Status != longhorn.ConditionStatusTrue {
		return nil, fmt.Errorf("node %v is not ready, couldn't attach volume %v to it", node.Name, name)
	}

	v, err = m.ds.GetVolume(name)
	if err != nil {
		return nil, err
	}

	if isReady, err := m.ds.CheckEngineImageReadyOnAtLeastOneVolumeReplica(v.Spec.EngineImage, v.Name, node.Name, v.Spec.DataLocality); !isReady {
		if err != nil {
			return nil, errors.Wrapf(err, "cannot attach volume %v with image %v", v.Name, v.Spec.EngineImage)
		}
		return nil, fmt.Errorf("cannot attach volume %v because the engine image %v is not deployed on at least one of the the replicas' nodes or the node that the volume is going to attach to", v.Name, v.Spec.EngineImage)
	}

	restoreCondition := types.GetCondition(v.Status.Conditions, longhorn.VolumeConditionTypeRestore)
	if restoreCondition.Status == longhorn.ConditionStatusTrue {
		return nil, fmt.Errorf("volume %v is restoring data", name)
	}

	if v.Status.RestoreRequired {
		return nil, fmt.Errorf("volume %v is pending restoring", name)
	}

	if v.Spec.MigrationNodeID == node.Name {
		logrus.Infof("Volume %v is already migrating to node %v from node %v", v.Name, node.Name, v.Spec.NodeID)
		return v, nil
	}

	// TODO: special case for attachment by UI
	if attacherType == "" {
		attacherType = string(longhorn.AttacherTypeLonghornAPI)
	}

	va, err := m.ds.GetLHVolumeAttachmentByVolumeName(v.Name)
	if err != nil {
		return nil, err
	}

	va.Spec.AttachmentTickets[attachmentID] = &longhorn.AttachmentTicket{
		ID: attachmentID,
		// TODO: validate attacher type
		Type:   longhorn.AttacherType(attacherType),
		NodeID: node.Name,
		Parameters: map[string]string{
			longhorn.AttachmentParameterDisableFrontend: strconv.FormatBool(disableFrontend),
			longhorn.AttachmentParameterLastAttachedBy:  attachedBy,
		},
	}

	if _, err := m.ds.UpdateLHVolumeAttachment(va); err != nil {
		return nil, err
	}

	return v, nil
}

// Detach will handle regular detachment as well as cleaning up attachment Ticket created by upgrade path
func (m *VolumeManager) Detach(name, attachmentID, hostID string, forceDetach bool) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to detach volume %v", name)
	}()

	v, err = m.ds.GetVolume(name)
	if err != nil {
		return nil, err
	}

	if v.Status.IsStandby {
		return nil, fmt.Errorf("cannot detach standby volume %v", v.Name)
	}

	va, err := m.ds.GetLHVolumeAttachmentByVolumeName(v.Name)
	if err != nil {
		return nil, err
	}

	// if force detach, detach from all nodes by clearing the volumeattachment spec
	if forceDetach {
		va.Spec.AttachmentTickets = make(map[string]*longhorn.AttachmentTicket)
		if _, err := m.ds.UpdateLHVolumeAttachment(va); err != nil {
			return nil, err
		}
		return v, nil
	}

	delete(va.Spec.AttachmentTickets, attachmentID)

	if _, err := m.ds.UpdateLHVolumeAttachment(va); err != nil {
		return nil, err
	}

	return v, nil
}

func (m *VolumeManager) isVolumeAvailableOnNode(volume, node string) bool {
	es, _ := m.ds.ListVolumeEngines(volume)
	for _, e := range es {
		if e.Spec.NodeID != node {
			continue
		}
		if e.DeletionTimestamp != nil {
			continue
		}
		if e.Spec.DesireState != longhorn.InstanceStateRunning || e.Status.CurrentState != longhorn.InstanceStateRunning {
			continue
		}
		hasAvailableReplica := false
		for _, mode := range e.Status.ReplicaModeMap {
			hasAvailableReplica = hasAvailableReplica || mode == longhorn.ReplicaModeRW
		}
		if !hasAvailableReplica {
			continue
		}
		return true
	}

	return false
}

func (m *VolumeManager) Salvage(volumeName string, replicaNames []string) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to salvage volume %v", volumeName)
	}()

	v, err = m.ds.GetVolume(volumeName)
	if err != nil {
		return nil, err
	}
	if v.Status.State != longhorn.VolumeStateDetached {
		return nil, fmt.Errorf("invalid volume state to salvage: %v", v.Status.State)
	}
	if v.Status.Robustness != longhorn.VolumeRobustnessFaulted {
		return nil, fmt.Errorf("invalid robustness state to salvage: %v", v.Status.Robustness)
	}
	v.Spec.NodeID = ""
	v, err = m.ds.UpdateVolume(v)
	if err != nil {
		return nil, err
	}

	for _, name := range replicaNames {
		r, err := m.ds.GetReplica(name)
		if err != nil {
			return nil, err
		}
		if r.Spec.VolumeName != v.Name {
			return nil, fmt.Errorf("replica %v doesn't belong to volume %v", r.Name, v.Name)
		}
		isDownOrDeleted, err := m.ds.IsNodeDownOrDeleted(r.Spec.NodeID)
		if err != nil {
			return nil, fmt.Errorf("failed to check if the related node %v is still running for replica %v", r.Spec.NodeID, name)
		}
		if isDownOrDeleted {
			return nil, fmt.Errorf("unable to check if the related node %v is down or deleted for replica %v", r.Spec.NodeID, name)
		}
		node, err := m.ds.GetNode(r.Spec.NodeID)
		if err != nil {
			return nil, fmt.Errorf("failed to get the related node %v for replica %v", r.Spec.NodeID, name)
		}
		diskSchedulable := false
		for _, diskStatus := range node.Status.DiskStatus {
			if diskStatus.DiskUUID == r.Spec.DiskID {
				if types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeSchedulable).Status == longhorn.ConditionStatusTrue {
					diskSchedulable = true
					break
				}
			}
		}
		if !diskSchedulable {
			return nil, fmt.Errorf("disk with UUID %v on node %v is unschedulable for replica %v", r.Spec.DiskID, r.Spec.NodeID, name)
		}
		if r.Spec.FailedAt == "" {
			// already updated, ignore it for idempotency
			continue
		}
		r.Spec.FailedAt = ""
		if _, err := m.ds.UpdateReplica(r); err != nil {
			return nil, err
		}
	}

	logrus.Infof("Salvaged replica %+v for volume %v", replicaNames, v.Name)
	return v, nil
}

func (m *VolumeManager) Activate(volumeName string, frontend string) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to activate volume %v", volumeName)
	}()

	v, err = m.ds.GetVolume(volumeName)
	if err != nil {
		return nil, err
	}

	if !v.Status.IsStandby {
		return nil, fmt.Errorf("volume %v is already in active mode", v.Name)
	}
	if !v.Spec.Standby {
		return nil, fmt.Errorf("volume %v is being activated", v.Name)
	}

	if frontend != string(longhorn.VolumeFrontendBlockDev) && frontend != string(longhorn.VolumeFrontendISCSI) {
		return nil, fmt.Errorf("invalid frontend %v", frontend)
	}

	v, err = m.ds.GetVolume(volumeName)
	if err != nil {
		return nil, err
	}

	// Trigger a backup volume update to get the latest backup
	// and will confirm recovery completion in volume state reconciliation
	if err := m.triggerBackupVolumeToSync(v); err != nil {
		return nil, err
	}

	v.Spec.Frontend = longhorn.VolumeFrontend(frontend)
	v.Spec.Standby = false
	v, err = m.ds.UpdateVolume(v)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Activating volume %v with frontend %v", v.Name, frontend)
	return v, nil
}

func (m *VolumeManager) triggerBackupVolumeToSync(volume *longhorn.Volume) error {
	backupVolumeName, isExist := volume.Labels[types.LonghornLabelBackupVolume]
	if !isExist || backupVolumeName == "" {
		return errors.Errorf("cannot find the backup volume label for volume: %v", volume.Name)
	}

	backupVolume, err := m.ds.GetBackupVolume(backupVolumeName)
	if err != nil {
		return errors.Wrapf(err, "failed to get backup volume: %v", backupVolumeName)
	}
	requestSyncTime := metav1.Time{Time: time.Now().UTC()}
	backupVolume.Spec.SyncRequestedAt = requestSyncTime
	if _, err = m.ds.UpdateBackupVolume(backupVolume); err != nil {
		return errors.Wrapf(err, "failed to update backup volume: %v", backupVolumeName)
	}

	return nil
}

func (m *VolumeManager) Expand(volumeName string, size int64) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to expand volume %v", volumeName)
	}()

	v, err = m.ds.GetVolume(volumeName)
	if err != nil {
		return nil, err
	}

	if v.Status.State != longhorn.VolumeStateDetached && v.Status.State != longhorn.VolumeStateAttached {
		return nil, fmt.Errorf("invalid volume state to expand: %v", v.Status.State)
	}

	if types.GetCondition(v.Status.Conditions, longhorn.VolumeConditionTypeScheduled).Status != longhorn.ConditionStatusTrue {
		return nil, fmt.Errorf("cannot expand volume before replica scheduling success")
	}

	size = util.RoundUpSize(size)

	kubernetesStatus := &v.Status.KubernetesStatus
	if kubernetesStatus.PVCName != "" && kubernetesStatus.LastPVCRefAt == "" {
		waitForPVCExpansion, size, err := m.checkAndExpandPVC(kubernetesStatus.Namespace, kubernetesStatus.PVCName, size)
		if err != nil {
			return nil, err
		}

		// PVC Expand call
		if waitForPVCExpansion {
			return v, nil
		}

		logrus.Infof("CSI plugin call to expand volume %v to size %v", v.Name, size)
	}

	if v.Spec.Size >= size {
		logrus.Infof("Volume %v expansion is not necessary since current size %v >= %v", v.Name, v.Spec.Size, size)
		return v, nil
	}

	if _, err := m.scheduler.CheckReplicasSizeExpansion(v, v.Spec.Size, size); err != nil {
		return nil, err
	}

	previousSize := v.Spec.Size
	v.Spec.Size = size

	v, err = m.ds.UpdateVolume(v)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Expanding volume %v from %v to %v requested", v.Name, previousSize, size)

	return v, nil
}

func (m *VolumeManager) checkAndExpandPVC(namespace string, pvcName string, size int64) (waitForPVCExpansion bool, s int64, err error) {
	pvc, err := m.ds.GetPersistentVolumeClaim(namespace, pvcName)
	if err != nil {
		return false, -1, err
	}

	// TODO: Should check for pvc.Spec.Resources.Requests.Storage() here, once upgrade API to v0.18.x.
	pvcSpecValue, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if !ok {
		return false, -1, fmt.Errorf("cannot get request storage")
	}

	requestedSize := resource.MustParse(strconv.FormatInt(size, 10))

	if pvcSpecValue.Cmp(requestedSize) < 0 {
		pvc.Spec.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: requestedSize,
			},
		}

		logrus.Infof("Persistent Volume Claim %v expand to %v requested", pvcName, requestedSize)
		_, err = m.ds.UpdatePersistentVolumeClaim(namespace, pvc)
		if err != nil {
			return false, -1, err
		}

		// return and CSI plugin call this API later for Longhorn volume expansion
		return true, 0, nil
	}

	// all other case belong to the CSI plugin call
	if pvcSpecValue.Cmp(requestedSize) > 0 {
		size = util.RoundUpSize(pvcSpecValue.Value())
	}
	return false, size, nil
}

func (m *VolumeManager) CancelExpansion(volumeName string) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to cancel expansion for volume %v", volumeName)
	}()

	v, err = m.ds.GetVolume(volumeName)
	if err != nil {
		return nil, err
	}
	if !v.Status.ExpansionRequired {
		return nil, fmt.Errorf("volume expansion is not started")
	}
	if v.Status.IsStandby {
		return nil, fmt.Errorf("canceling expansion for standby volume is not supported")
	}

	var engine *longhorn.Engine
	es, err := m.ds.ListVolumeEngines(v.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to list engines for volume %v: %v", v.Name, err)
	}
	if len(es) != 1 {
		return nil, fmt.Errorf("found more than 1 engines for volume %v", v.Name)
	}
	for _, e := range es {
		engine = e
	}

	if engine.Status.IsExpanding {
		return nil, fmt.Errorf("the engine expansion is in progress")
	}
	if engine.Status.CurrentSize == v.Spec.Size {
		return nil, fmt.Errorf("the engine expansion is already complete")
	}

	previousSize := v.Spec.Size
	v.Spec.Size = engine.Status.CurrentSize
	v, err = m.ds.UpdateVolume(v)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Canceling volume %v expansion from %v to %v requested", v.Name, previousSize, v.Spec.Size)
	return v, nil
}

func (m *VolumeManager) TrimFilesystem(name string) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to trim filesystem for volume %v", name)
	}()

	v, err = m.ds.GetVolume(name)
	if err != nil {
		return nil, err
	}
	if v.Status.State != longhorn.VolumeStateAttached {
		return nil, fmt.Errorf("volume is not attached")
	}
	if v.Status.FrontendDisabled {
		return nil, fmt.Errorf("volume frontend is disabled")
	}

	if v.Spec.AccessMode == longhorn.AccessModeReadWriteMany {
		return v, m.trimRWXVolumeFilesystem(name, v.Spec.Encrypted)
	}
	return v, m.trimNonRWXVolumeFilesystem(name, v.Spec.Encrypted)
}

func (m *VolumeManager) trimNonRWXVolumeFilesystem(volumeName string, encryptedDevice bool) error {
	return util.TrimFilesystem(volumeName, encryptedDevice)
}

func (m *VolumeManager) trimRWXVolumeFilesystem(volumeName string, encryptedDevice bool) error {
	sm, err := m.ds.GetShareManager(volumeName)
	if err != nil {
		return errors.Wrapf(err, "failed to get share manager for trimming volume %v", volumeName)
	}
	pod, err := m.ds.GetPodRO(sm.Namespace, types.GetShareManagerPodNameFromShareManagerName(sm.Name))
	if err != nil {
		return errors.Wrapf(err, "failed to get share manager pod for trimming volume %v in namespae", volumeName)
	}

	if sm.Status.State != longhorn.ShareManagerStateRunning {
		return fmt.Errorf("share manager %v is not running", sm.Name)
	}

	client, err := engineapi.NewShareManagerClient(sm, pod)
	if err != nil {
		return errors.Wrapf(err, "failed to launch gRPC client for share manager before trimming volume %v", volumeName)
	}
	defer client.Close()

	return client.FilesystemTrim(encryptedDevice)
}

func (m *VolumeManager) AddVolumeRecurringJob(volumeName string, name string, isGroup bool) (volumeRecurringJob map[string]*longhorn.VolumeRecurringJob, err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to add volume recurring jobs for %v", volumeName)
	}()

	volume, err := m.ds.GetVolume(volumeName)
	if err != nil {
		return nil, err
	}

	key := types.GetRecurringJobLabelKeyByType(name, isGroup)
	volume, err = m.ds.AddRecurringJobLabelToVolume(volume, key)
	if err != nil {
		return nil, err
	}

	volumeRecurringJob, err = m.ListVolumeRecurringJob(volume.Name)
	if err != nil {
		return nil, err
	}
	return volumeRecurringJob, nil
}

func (m *VolumeManager) ListVolumeRecurringJob(volumeName string) (map[string]*longhorn.VolumeRecurringJob, error) {
	var err error
	defer func() {
		err = errors.Wrapf(err, "failed to list volume recurring jobs for %v", volumeName)
	}()

	v, err := m.ds.GetVolume(volumeName)
	if err != nil {
		return nil, err
	}
	return datastore.MarshalLabelToVolumeRecurringJob(v.Labels), nil
}

func (m *VolumeManager) DeleteVolumeRecurringJob(volumeName string, name string, isGroup bool) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to delete recurring job %v from Volume %v", name, volumeName)
	}()

	volume, err := m.ds.GetVolumeRO(volumeName)
	if err != nil {
		return nil, err
	}

	key := types.GetRecurringJobLabelKeyByType(name, isGroup)
	return m.ds.RemoveRecurringJobLabelFromVolume(volume, key)
}

func (m *VolumeManager) DeleteReplica(volumeName, replicaName string) error {
	healthyReplica := ""
	rs, err := m.ds.ListVolumeReplicas(volumeName)
	if err != nil {
		return err
	}
	if _, exists := rs[replicaName]; !exists {
		return fmt.Errorf("cannot find replica %v of volume %v", replicaName, volumeName)
	}
	for _, r := range rs {
		if r.Name == replicaName {
			continue
		}
		if !datastore.IsAvailableHealthyReplica(r) {
			continue
		}
		if r.Status.EvictionRequested {
			continue
		}
		healthyReplica = r.Name
		break
	}
	if healthyReplica == "" {
		return fmt.Errorf("no other healthy replica available, cannot delete replica %v since it may still contain data for recovery", replicaName)
	}
	if err := m.ds.DeleteReplica(replicaName); err != nil {
		return err
	}
	logrus.Infof("Deleted replica %v of volume %v, there is still at least one available healthy replica %v", replicaName, volumeName, healthyReplica)
	return nil
}

func (m *VolumeManager) GetManagerNodeIPMap() (map[string]string, error) {
	podList, err := m.ds.ListManagerPods()
	if err != nil {
		return nil, err
	}

	nodeIPMap := map[string]string{}
	for _, pod := range podList {
		if nodeIPMap[pod.Spec.NodeName] != "" {
			return nil, fmt.Errorf("multiple managers on the node %v", pod.Spec.NodeName)
		}
		nodeIPMap[pod.Spec.NodeName] = pod.Status.PodIP
	}
	return nodeIPMap, nil
}

func (m *VolumeManager) EngineUpgrade(volumeName, image string) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "cannot upgrade engine for volume %v using image %v", volumeName, image)
	}()

	// Only allow to upgrade to the default engine image if the setting `Automatically upgrade volumes' engine to the default engine image` is enabled
	concurrentAutomaticEngineUpgradePerNodeLimit, err := m.ds.GetSettingAsInt(types.SettingNameConcurrentAutomaticEngineUpgradePerNodeLimit)
	if err != nil {
		return nil, err
	}
	if concurrentAutomaticEngineUpgradePerNodeLimit > 0 {
		defaultEngineImage, err := m.ds.GetSettingValueExisted(types.SettingNameDefaultEngineImage)
		if err != nil {
			return nil, err
		}
		if image != defaultEngineImage {
			return nil, fmt.Errorf("upgrading to %v is not allowed. "+
				"Only allow to upgrade to the default engine image %v because the setting "+
				"`Concurrent Automatic Engine Upgrade Per Node Limit` is greater than 0",
				image, defaultEngineImage)
		}
	}

	v, err = m.ds.GetVolume(volumeName)
	if err != nil {
		return nil, err
	}

	if v.Spec.EngineImage == image {
		return nil, fmt.Errorf("upgrading in process for volume %v engine image from %v to %v already",
			v.Name, v.Status.CurrentImage, v.Spec.EngineImage)
	}

	if v.Spec.EngineImage != v.Status.CurrentImage && image != v.Status.CurrentImage {
		return nil, fmt.Errorf("upgrading in process for volume %v engine image from %v to %v, cannot upgrade to another engine image",
			v.Name, v.Status.CurrentImage, v.Spec.EngineImage)
	}

	if isReady, err := m.ds.CheckEngineImageReadyOnAllVolumeReplicas(image, v.Name, v.Status.CurrentNodeID); !isReady {
		if err != nil {
			return nil, fmt.Errorf("cannot upgrade engine image for volume %v from image %v to image %v: %v", v.Name, v.Spec.EngineImage, image, err)
		}
		return nil, fmt.Errorf("cannot upgrade engine image for volume %v from image %v to image %v because the engine image %v is not deployed on the replicas' nodes or the node that the volume is attached to", v.Name, v.Spec.EngineImage, image, image)
	}

	if isReady, err := m.ds.CheckEngineImageReadyOnAllVolumeReplicas(v.Status.CurrentImage, v.Name, v.Status.CurrentNodeID); !isReady {
		if err != nil {
			return nil, fmt.Errorf("cannot upgrade engine image for volume %v from image %v to image %v: %v", v.Name, v.Spec.EngineImage, image, err)
		}
		return nil, fmt.Errorf("cannot upgrade engine image for volume %v from image %v to image %v because the volume's current engine image %v is not deployed on the replicas' nodes or the node that the volume is attached to", v.Name, v.Spec.EngineImage, image, v.Status.CurrentImage)
	}

	if v.Spec.MigrationNodeID != "" {
		return nil, fmt.Errorf("cannot upgrade during migration")
	}

	// Note: Rebuild is not supported for old DR volumes and the handling of a degraded DR volume live upgrade will get stuck.
	//  Hence if you modify this part, the live upgrade should be prevented in API level for all old DR volumes.
	if v.Status.State == longhorn.VolumeStateAttached && v.Status.Robustness != longhorn.VolumeRobustnessHealthy {
		return nil, fmt.Errorf("cannot do live upgrade for a unhealthy volume %v", v.Name)
	}

	oldImage := v.Spec.EngineImage
	v.Spec.EngineImage = image

	v, err = m.ds.UpdateVolume(v)
	if err != nil {
		return nil, err
	}
	if image != v.Status.CurrentImage {
		logrus.Infof("Upgrading volume %v engine image from %v to %v", v.Name, oldImage, v.Spec.EngineImage)
	} else {
		logrus.Infof("Rolling back volume %v engine image to %v", v.Name, v.Status.CurrentImage)
	}

	return v, nil
}

func (m *VolumeManager) UpdateReplicaCount(name string, count int) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to update replica count for volume %v", name)
	}()

	v, err = m.ds.GetVolume(name)
	if err != nil {
		return nil, err
	}

	if v.Spec.NodeID == "" || v.Status.State != longhorn.VolumeStateAttached {
		return nil, fmt.Errorf("invalid volume state to update replica count%v", v.Status.State)
	}
	if v.Spec.EngineImage != v.Status.CurrentImage {
		return nil, fmt.Errorf("upgrading in process, cannot update replica count")
	}
	if v.Spec.MigrationNodeID != "" {
		return nil, fmt.Errorf("migration in process, cannot update replica count")
	}

	oldCount := v.Spec.NumberOfReplicas
	v.Spec.NumberOfReplicas = count

	v, err = m.ds.UpdateVolume(v)
	if err != nil {
		return nil, err
	}
	logrus.Infof("Updated volume %v replica count from %v to %v", v.Name, oldCount, v.Spec.NumberOfReplicas)
	return v, nil
}

func (m *VolumeManager) UpdateSnapshotDataIntegrity(name string, value string) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to update snapshot data integrity for volume %v", name)
	}()

	v, err = m.ds.GetVolume(name)
	if err != nil {
		return nil, err
	}

	oldValue := v.Spec.SnapshotDataIntegrity
	v.Spec.SnapshotDataIntegrity = longhorn.SnapshotDataIntegrity(value)

	v, err = m.ds.UpdateVolume(v)
	if err != nil {
		return nil, err
	}
	logrus.Infof("Updated volume %v snapshot data integrity from %v to %v", v.Name, oldValue, v.Spec.SnapshotDataIntegrity)
	return v, nil
}

func (m *VolumeManager) UpdateOfflineReplicaRebuilding(name string, value string) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to update offline replica rebuilding for volume %v", name)
	}()

	v, err = m.ds.GetVolume(name)
	if err != nil {
		return nil, err
	}

	oldValue := v.Spec.OfflineReplicaRebuilding
	v.Spec.OfflineReplicaRebuilding = longhorn.OfflineReplicaRebuilding(value)

	v, err = m.ds.UpdateVolume(v)
	if err != nil {
		return nil, err
	}
	logrus.Infof("Updated volume %v offline replica rebuilding from %v to %v", v.Name, oldValue, v.Spec.OfflineReplicaRebuilding)
	return v, nil
}

func (m *VolumeManager) UpdateBackupCompressionMethod(name string, value string) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to update backup compression method for volume %v", name)
	}()

	v, err = m.ds.GetVolume(name)
	if err != nil {
		return nil, err
	}

	oldValue := v.Spec.BackupCompressionMethod
	v.Spec.BackupCompressionMethod = longhorn.BackupCompressionMethod(value)

	v, err = m.ds.UpdateVolume(v)
	if err != nil {
		return nil, err
	}
	logrus.Infof("Updated volume %v backup compression method from %v to %v", v.Name, oldValue, v.Spec.BackupCompressionMethod)
	return v, nil
}

func (m *VolumeManager) UpdateReplicaAutoBalance(name string, inputSpec longhorn.ReplicaAutoBalance) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to update replica auto-balance for volume %v", name)
	}()

	v, err = m.ds.GetVolume(name)
	if err != nil {
		return nil, err
	}

	if v.Spec.ReplicaAutoBalance == inputSpec {
		logrus.Debugf("Volume %v already has replica auto-balance set to %v", v.Name, inputSpec)
		return v, nil
	}

	oldSpec := v.Spec.ReplicaAutoBalance
	v.Spec.ReplicaAutoBalance = inputSpec
	v, err = m.ds.UpdateVolume(v)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Updated volume %v replica auto-balance spec from %v to %v", v.Name, oldSpec, v.Spec.ReplicaAutoBalance)
	return v, nil
}

func (m *VolumeManager) UpdateDataLocality(name string, dataLocality longhorn.DataLocality) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to update data locality for volume %v", name)
	}()

	v, err = m.ds.GetVolume(name)
	if err != nil {
		return nil, err
	}

	if v.Spec.DataLocality == dataLocality {
		logrus.Debugf("Volume %v already has data locality %v", v.Name, dataLocality)
		return v, nil
	}

	oldDataLocality := v.Spec.DataLocality
	v.Spec.DataLocality = dataLocality
	v, err = m.ds.UpdateVolume(v)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Updated volume %v data locality from %v to %v", v.Name, oldDataLocality, v.Spec.DataLocality)
	return v, nil
}

func (m *VolumeManager) UpdateAccessMode(name string, accessMode longhorn.AccessMode) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to update access mode for volume %v", name)
	}()

	v, err = m.ds.GetVolume(name)
	if err != nil {
		return nil, err
	}

	if v.Spec.AccessMode == accessMode {
		logrus.Debugf("Volume %v already has access mode %v", v.Name, accessMode)
		return v, nil
	}

	if v.Spec.NodeID != "" || v.Status.State != longhorn.VolumeStateDetached {
		return nil, fmt.Errorf("can only update volume access mode while volume is detached")
	}

	oldAccessMode := v.Spec.AccessMode
	v.Spec.AccessMode = accessMode
	v, err = m.ds.UpdateVolume(v)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Updated volume %v access mode from %v to %v", v.Name, oldAccessMode, accessMode)
	return v, nil
}

func (m *VolumeManager) UpdateUnmapMarkSnapChainRemoved(name string, unmapMarkSnapChainRemoved longhorn.UnmapMarkSnapChainRemoved) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to update field UnmapMarkSnapChainRemoved for volume %v", name)
	}()

	v, err = m.ds.GetVolume(name)
	if err != nil {
		return nil, err
	}

	if v.Spec.UnmapMarkSnapChainRemoved == unmapMarkSnapChainRemoved {
		logrus.Debugf("Volume %v already set field UnmapMarkSnapChainRemoved to %v", v.Name, unmapMarkSnapChainRemoved)
		return v, nil
	}

	oldUnmapMarkSnapChainRemoved := v.Spec.UnmapMarkSnapChainRemoved
	v.Spec.UnmapMarkSnapChainRemoved = unmapMarkSnapChainRemoved
	v, err = m.ds.UpdateVolume(v)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Updated volume %v field UnmapMarkSnapChainRemoved from %v to %v", v.Name, oldUnmapMarkSnapChainRemoved, unmapMarkSnapChainRemoved)
	return v, nil
}

func (m *VolumeManager) UpdateReplicaSoftAntiAffinity(name string, replicaSoftAntiAffinity longhorn.ReplicaSoftAntiAffinity) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to update field ReplicaSoftAntiAffinity for volume %v", name)
	}()

	v, err = m.ds.GetVolume(name)
	if err != nil {
		return nil, err
	}

	if v.Spec.ReplicaSoftAntiAffinity == replicaSoftAntiAffinity {
		logrus.Debugf("Volume %v already set field ReplicaSoftAntiAffinity to %v", v.Name, replicaSoftAntiAffinity)
		return v, nil
	}

	oldReplicaSoftAntiAffinity := v.Spec.ReplicaSoftAntiAffinity
	v.Spec.ReplicaSoftAntiAffinity = replicaSoftAntiAffinity
	v, err = m.ds.UpdateVolume(v)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Updated volume %v field ReplicaSoftAntiAffinity from %v to %v", v.Name, oldReplicaSoftAntiAffinity, replicaSoftAntiAffinity)
	return v, nil
}

func (m *VolumeManager) UpdateReplicaZoneSoftAntiAffinity(name string, replicaZoneSoftAntiAffinity longhorn.ReplicaZoneSoftAntiAffinity) (v *longhorn.Volume, err error) {
	defer func() {
		err = errors.Wrapf(err, "unable to update field ReplicaZoneSoftAntiAffinity for volume %v", name)
	}()

	v, err = m.ds.GetVolume(name)
	if err != nil {
		return nil, err
	}

	if v.Spec.ReplicaZoneSoftAntiAffinity == replicaZoneSoftAntiAffinity {
		logrus.Debugf("Volume %v already set field ReplicaZoneSoftAntiAffinity to %v", v.Name, replicaZoneSoftAntiAffinity)
		return v, nil
	}

	oldReplicaZoneSoftAntiAffinity := v.Spec.ReplicaZoneSoftAntiAffinity
	v.Spec.ReplicaZoneSoftAntiAffinity = replicaZoneSoftAntiAffinity
	v, err = m.ds.UpdateVolume(v)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Updated volume %v field ReplicaZoneSoftAntiAffinity from %v to %v", v.Name, oldReplicaZoneSoftAntiAffinity, replicaZoneSoftAntiAffinity)
	return v, nil
}

func (m *VolumeManager) verifyDataSourceForVolumeCreation(dataSource longhorn.VolumeDataSource, requestSize int64) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to verify data source")
	}()

	if !types.IsValidVolumeDataSource(dataSource) {
		return fmt.Errorf("invalid value for data source: %v", dataSource)
	}

	if types.IsDataFromVolume(dataSource) {
		srcVolName := types.GetVolumeName(dataSource)
		srcVol, err := m.ds.GetVolume(srcVolName)
		if err != nil {
			return err
		}
		if requestSize != srcVol.Spec.Size {
			return fmt.Errorf("size of target volume (%v bytes) is different than size of source volume (%v bytes)", requestSize, srcVol.Spec.Size)
		}

		if snapName := types.GetSnapshotName(dataSource); snapName != "" {
			snapshotCR, err := m.GetSnapshotCR(snapName)
			if err != nil {
				return err
			}
			if !snapshotCR.Status.ReadyToUse {
				return fmt.Errorf("snapshot %v is not ready to use", snapshotCR.Name)
			}
		}
	}
	return nil
}
