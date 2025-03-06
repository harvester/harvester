package scheduler

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	FailedReplicaMaxRetryCount = 5
)

type ReplicaScheduler struct {
	ds *datastore.DataStore

	// Required for unit testing.
	nowHandler func() time.Time
}

type Disk struct {
	longhorn.DiskSpec
	*longhorn.DiskStatus
	NodeID string
}

type DiskSchedulingInfo struct {
	DiskUUID                   string
	StorageAvailable           int64
	StorageMaximum             int64
	StorageReserved            int64
	StorageScheduled           int64
	OverProvisioningPercentage int64
	MinimalAvailablePercentage int64
}

func NewReplicaScheduler(ds *datastore.DataStore) *ReplicaScheduler {
	rcScheduler := &ReplicaScheduler{
		ds: ds,

		// Required for unit testing.
		nowHandler: time.Now,
	}
	return rcScheduler
}

// ScheduleReplica will return (nil, nil) for unschedulable replica
func (rcs *ReplicaScheduler) ScheduleReplica(replica *longhorn.Replica, replicas map[string]*longhorn.Replica, volume *longhorn.Volume) (*longhorn.Replica, util.MultiError, error) {
	// only called when replica is starting for the first time
	if replica.Spec.NodeID != "" {
		return nil, nil, fmt.Errorf("BUG: Replica %v has been scheduled to node %v", replica.Name, replica.Spec.NodeID)
	}

	// not to schedule a replica failed and unused before.
	if replica.Spec.HealthyAt == "" && replica.Spec.FailedAt != "" {
		logrus.WithField("replica", replica.Name).Warn("Failed replica is not scheduled")
		return nil, nil, nil
	}

	diskCandidates, multiError, err := rcs.FindDiskCandidates(replica, replicas, volume)
	if err != nil {
		return nil, nil, err
	}

	// there's no disk that fit for current replica
	if len(diskCandidates) == 0 {
		logrus.Errorf("There's no available disk for replica %v, size %v", replica.ObjectMeta.Name, replica.Spec.VolumeSize)
		return nil, multiError, nil
	}

	rcs.scheduleReplicaToDisk(replica, diskCandidates)

	return replica, nil, nil
}

// FindDiskCandidates identifies suitable disks on eligible nodes for the replica.
//
// Parameters:
// - replica: The replica for which to find disk candidates.
// - replicas: The map of existing replicas.
// - volume: The volume associated with the replica.
//
// Returns:
// - Map of disk candidates (disk UUID to Disk).
// - MultiError for non-fatal errors encountered.
// - Error for any fatal errors encountered.
func (rcs *ReplicaScheduler) FindDiskCandidates(replica *longhorn.Replica, replicas map[string]*longhorn.Replica, volume *longhorn.Volume) (map[string]*Disk, util.MultiError, error) {
	nodesInfo, err := rcs.getNodeInfo()
	if err != nil {
		return nil, nil, err
	}

	nodeCandidates, multiError := rcs.getNodeCandidates(nodesInfo, replica)
	if len(nodeCandidates) == 0 {
		logrus.Errorf("There's no available node for replica %v, size %v", replica.ObjectMeta.Name, replica.Spec.VolumeSize)
		return nil, multiError, nil
	}

	nodeDisksMap := map[string]map[string]struct{}{}
	for _, node := range nodeCandidates {
		disks := map[string]struct{}{}
		for diskName, diskStatus := range node.Status.DiskStatus {
			diskSpec, exists := node.Spec.Disks[diskName]
			if !exists {
				continue
			}
			if !diskSpec.AllowScheduling || diskSpec.EvictionRequested {
				continue
			}
			if types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeSchedulable).Status != longhorn.ConditionStatusTrue {
				continue
			}
			disks[diskStatus.DiskUUID] = struct{}{}
		}
		nodeDisksMap[node.Name] = disks
	}

	diskCandidates, multiError := rcs.getDiskCandidates(nodeCandidates, nodeDisksMap, replicas, volume, true, false)
	return diskCandidates, multiError, nil
}

func (rcs *ReplicaScheduler) getNodeCandidates(nodesInfo map[string]*longhorn.Node, schedulingReplica *longhorn.Replica) (nodeCandidates map[string]*longhorn.Node, multiError util.MultiError) {
	if schedulingReplica.Spec.HardNodeAffinity != "" {
		node, exist := nodesInfo[schedulingReplica.Spec.HardNodeAffinity]
		if !exist {
			return nil, util.NewMultiError(longhorn.ErrorReplicaScheduleHardNodeAffinityNotSatisfied)
		}
		nodesInfo = map[string]*longhorn.Node{}
		nodesInfo[schedulingReplica.Spec.HardNodeAffinity] = node
	}

	if len(nodesInfo) == 0 {
		return nil, util.NewMultiError(longhorn.ErrorReplicaScheduleNodeUnavailable)
	}

	nodeCandidates = map[string]*longhorn.Node{}
	for _, node := range nodesInfo {
		if types.IsDataEngineV2(schedulingReplica.Spec.DataEngine) {
			disabled, err := rcs.ds.IsV2DataEngineDisabledForNode(node.Name)
			if err != nil {
				logrus.WithError(err).Errorf("Failed to check if v2 data engine is disabled on node %v", node.Name)
				return nil, util.NewMultiError(longhorn.ErrorReplicaScheduleSchedulingFailed)
			}
			if disabled {
				continue
			}
		}

		log := logrus.WithField("node", node.Name)

		// After a node reboot, it might be listed in the nodeInfo but its InstanceManager
		// is not ready. To prevent scheduling replicas on such nodes, verify the
		// InstanceManager's readiness before including it in the candidate list.
		if isReady, err := rcs.ds.CheckInstanceManagersReadiness(schedulingReplica.Spec.DataEngine, node.Name); !isReady {
			if err != nil {
				log = log.WithError(err)
			}
			log.Debugf("Excluding node in node candidates because instance manager on node is not ready")
			continue
		}

		if isReady, err := rcs.ds.CheckDataEngineImageReadiness(schedulingReplica.Spec.Image, schedulingReplica.Spec.DataEngine, node.Name); isReady {
			nodeCandidates[node.Name] = node
		} else {
			if err != nil {
				log = log.WithError(err)
			}
			log.Debugf("Excluding node in node candidates because data engine image on node is not ready")
		}
	}

	if len(nodeCandidates) == 0 {
		return map[string]*longhorn.Node{}, util.NewMultiError(longhorn.ErrorReplicaScheduleEngineImageNotReady)
	}

	return nodeCandidates, nil
}

// getDiskCandidates returns a map of the most appropriate disks a replica can be scheduled to (assuming it can be
// scheduled at all). For example, consider a case in which there are two disks on nodes without a replica for a volume
// and two disks on nodes with a replica for the same volume. getDiskCandidates only returns the disks without a
// replica, even if the replica can legally be scheduled on all four disks.
// Some callers (e.g. CheckAndReuseFailedReplicas) do not consider a node or zone to be used if it contains a failed
// replica. ignoreFailedReplicas == true supports this use case.
func (rcs *ReplicaScheduler) getDiskCandidates(nodeInfo map[string]*longhorn.Node,
	nodeDisksMap map[string]map[string]struct{},
	replicas map[string]*longhorn.Replica,
	volume *longhorn.Volume,
	requireSchedulingCheck, ignoreFailedReplicas bool) (map[string]*Disk, util.MultiError) {
	multiError := util.NewMultiError()

	biNodeSelector := []string{}
	biDiskSelector := []string{}
	if volume.Spec.BackingImage != "" {
		bi, err := rcs.ds.GetBackingImageRO(volume.Spec.BackingImage)
		if err != nil {
			err = errors.Wrapf(err, "failed to get backing image %v", volume.Spec.BackingImage)
			multiError.Append(util.NewMultiError(err.Error()))
			return map[string]*Disk{}, multiError
		}
		biNodeSelector = bi.Spec.NodeSelector
		biDiskSelector = bi.Spec.DiskSelector
	}

	nodeSoftAntiAffinity, err := rcs.ds.GetSettingAsBool(types.SettingNameReplicaSoftAntiAffinity)
	if err != nil {
		err = errors.Wrapf(err, "failed to get %v setting", types.SettingNameReplicaSoftAntiAffinity)
		multiError.Append(util.NewMultiError(err.Error()))
		return map[string]*Disk{}, multiError
	}

	if volume.Spec.ReplicaSoftAntiAffinity != longhorn.ReplicaSoftAntiAffinityDefault &&
		volume.Spec.ReplicaSoftAntiAffinity != "" {
		nodeSoftAntiAffinity = volume.Spec.ReplicaSoftAntiAffinity == longhorn.ReplicaSoftAntiAffinityEnabled
	}

	zoneSoftAntiAffinity, err := rcs.ds.GetSettingAsBool(types.SettingNameReplicaZoneSoftAntiAffinity)
	if err != nil {
		err = errors.Wrapf(err, "failed to get %v setting", types.SettingNameReplicaZoneSoftAntiAffinity)
		multiError.Append(util.NewMultiError(err.Error()))
		return map[string]*Disk{}, multiError
	}
	if volume.Spec.ReplicaZoneSoftAntiAffinity != longhorn.ReplicaZoneSoftAntiAffinityDefault &&
		volume.Spec.ReplicaZoneSoftAntiAffinity != "" {
		zoneSoftAntiAffinity = volume.Spec.ReplicaZoneSoftAntiAffinity == longhorn.ReplicaZoneSoftAntiAffinityEnabled
	}

	diskSoftAntiAffinity, err := rcs.ds.GetSettingAsBool(types.SettingNameReplicaDiskSoftAntiAffinity)
	if err != nil {
		err = errors.Wrapf(err, "failed to get %v setting", types.SettingNameReplicaDiskSoftAntiAffinity)
		multiError.Append(util.NewMultiError(err.Error()))
		return map[string]*Disk{}, multiError
	}
	if volume.Spec.ReplicaDiskSoftAntiAffinity != longhorn.ReplicaDiskSoftAntiAffinityDefault &&
		volume.Spec.ReplicaDiskSoftAntiAffinity != "" {
		diskSoftAntiAffinity = volume.Spec.ReplicaDiskSoftAntiAffinity == longhorn.ReplicaDiskSoftAntiAffinityEnabled
	}

	creatingNewReplicasForReplenishment := false
	if volume.Status.Robustness == longhorn.VolumeRobustnessDegraded {
		timeToReplacementReplica, _, err := rcs.timeToReplacementReplica(volume)
		if err != nil {
			err = errors.Wrap(err, "failed to get time until replica replacement")
			multiError.Append(util.NewMultiError(err.Error()))
			return map[string]*Disk{}, multiError
		}
		creatingNewReplicasForReplenishment = timeToReplacementReplica == 0
	}

	getDiskCandidatesFromNodes := func(nodes map[string]*longhorn.Node) (diskCandidates map[string]*Disk, multiError util.MultiError) {
		diskCandidates = map[string]*Disk{}
		multiError = util.NewMultiError()
		for _, node := range nodes {
			diskCandidatesFromNode, errors := rcs.filterNodeDisksForReplica(node, nodeDisksMap[node.Name], replicas,
				volume, requireSchedulingCheck, biDiskSelector)
			for k, v := range diskCandidatesFromNode {
				diskCandidates[k] = v
			}
			multiError.Append(errors)
		}
		diskCandidates = filterDisksWithMatchingReplicas(diskCandidates, replicas, diskSoftAntiAffinity, ignoreFailedReplicas)
		return diskCandidates, multiError
	}

	usedNodes, usedZones, onlyEvictingNodes, onlyEvictingZones := getCurrentNodesAndZones(replicas, nodeInfo,
		ignoreFailedReplicas, creatingNewReplicasForReplenishment)

	allowEmptyNodeSelectorVolume, err := rcs.ds.GetSettingAsBool(types.SettingNameAllowEmptyNodeSelectorVolume)
	if err != nil {
		err = errors.Wrapf(err, "failed to get %v setting", types.SettingNameAllowEmptyNodeSelectorVolume)
		multiError.Append(util.NewMultiError(err.Error()))
		return map[string]*Disk{}, multiError
	}

	replicaAutoBalance := rcs.ds.GetAutoBalancedReplicasSetting(volume, &logrus.Entry{})

	unusedNodes := map[string]*longhorn.Node{}
	unusedNodesInUnusedZones := map[string]*longhorn.Node{}

	// Per https://github.com/longhorn/longhorn/issues/3076, if a replica is being evicted from one disk on a node, the
	// scheduler must be given the opportunity to schedule it to a different disk on the same node (if it meets other
	// requirements). Track nodes that are evicting all their replicas in case we can reuse one.
	unusedNodesAfterEviction := map[string]*longhorn.Node{}
	unusedNodesInUnusedZonesAfterEviction := map[string]*longhorn.Node{}

	for nodeName, node := range nodeInfo {
		// Filter Nodes. If the Nodes don't match the tags, don't bother marking them as candidates.
		if !types.IsSelectorsInTags(node.Spec.Tags, volume.Spec.NodeSelector, allowEmptyNodeSelectorVolume) {
			continue
		}
		// If the Nodes don't match the tags of the backing image of this volume,
		// don't schedule the replica on it because it will hang there
		if volume.Spec.BackingImage != "" {
			if !types.IsSelectorsInTags(node.Spec.Tags, biNodeSelector, allowEmptyNodeSelectorVolume) {
				continue
			}
		}

		if _, ok := usedNodes[nodeName]; !ok {
			unusedNodes[nodeName] = node
		} else if replicaAutoBalance == longhorn.ReplicaAutoBalanceBestEffort {
			unusedNodes[nodeName] = node
		}
		if onlyEvictingNodes[nodeName] {
			unusedNodesAfterEviction[nodeName] = node
			if onlyEvictingZones[node.Status.Zone] {
				unusedNodesInUnusedZonesAfterEviction[nodeName] = node
			} else if replicaAutoBalance == longhorn.ReplicaAutoBalanceBestEffort {
				unusedNodesInUnusedZonesAfterEviction[nodeName] = node
			}
		}
		if _, ok := usedZones[node.Status.Zone]; !ok {
			unusedNodesInUnusedZones[nodeName] = node
		}
	}

	// In all cases, we should try to use a disk on an unused node in an unused zone first. Don't bother considering
	// zoneSoftAntiAffinity and nodeSoftAntiAffinity settings if such disks are available.
	diskCandidates, errors := getDiskCandidatesFromNodes(unusedNodesInUnusedZones)
	if len(diskCandidates) > 0 {
		return diskCandidates, nil
	}
	multiError.Append(errors)

	switch {
	case !zoneSoftAntiAffinity && !nodeSoftAntiAffinity:
		fallthrough
	// Same as the above. If we cannot schedule two replicas in the same zone, we cannot schedule them on the same node.
	case !zoneSoftAntiAffinity && nodeSoftAntiAffinity:
		diskCandidates, errors = getDiskCandidatesFromNodes(unusedNodesInUnusedZonesAfterEviction)
		if len(diskCandidates) > 0 {
			return diskCandidates, nil
		}
		multiError.Append(errors)
	case zoneSoftAntiAffinity && !nodeSoftAntiAffinity:
		diskCandidates, errors = getDiskCandidatesFromNodes(unusedNodes)
		if len(diskCandidates) > 0 {
			return diskCandidates, nil
		}
		multiError.Append(errors)
		diskCandidates, errors = getDiskCandidatesFromNodes(unusedNodesAfterEviction)
		if len(diskCandidates) > 0 {
			return diskCandidates, nil
		}
		multiError.Append(errors)
	case zoneSoftAntiAffinity && nodeSoftAntiAffinity:
		diskCandidates, errors = getDiskCandidatesFromNodes(unusedNodes)
		if len(diskCandidates) > 0 {
			return diskCandidates, nil
		}
		multiError.Append(errors)
		diskCandidates, errors = getDiskCandidatesFromNodes(usedNodes)
		if len(diskCandidates) > 0 {
			return diskCandidates, nil
		}
		multiError.Append(errors)
	}
	return map[string]*Disk{}, multiError
}

func (rcs *ReplicaScheduler) filterNodeDisksForReplica(node *longhorn.Node, disks map[string]struct{}, replicas map[string]*longhorn.Replica, volume *longhorn.Volume, requireSchedulingCheck bool, biDiskSelector []string) (preferredDisks map[string]*Disk, multiError util.MultiError) {
	multiError = util.NewMultiError()
	preferredDisks = map[string]*Disk{}

	allowEmptyDiskSelectorVolume, err := rcs.ds.GetSettingAsBool(types.SettingNameAllowEmptyDiskSelectorVolume)
	if err != nil {
		err = errors.Wrapf(err, "failed to get %v setting", types.SettingNameAllowEmptyDiskSelectorVolume)
		multiError.Append(util.NewMultiError(err.Error()))
		return preferredDisks, multiError
	}

	if len(disks) == 0 {
		multiError.Append(util.NewMultiError(longhorn.ErrorReplicaScheduleDiskUnavailable))
		return preferredDisks, multiError
	}

	// find disk that fit for current replica
	for diskUUID := range disks {
		var diskName string
		var diskSpec longhorn.DiskSpec
		var diskStatus *longhorn.DiskStatus
		diskFound := false
		for diskName, diskStatus = range node.Status.DiskStatus {
			if diskStatus.DiskUUID != diskUUID {
				continue
			}
			if !requireSchedulingCheck || types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeSchedulable).Status == longhorn.ConditionStatusTrue {
				diskFound = true
				diskSpec = node.Spec.Disks[diskName]
				break
			}
		}
		if !diskFound {
			logrus.Errorf("Cannot find the spec or the status for disk %v when scheduling replica", diskUUID)
			multiError.Append(util.NewMultiError(longhorn.ErrorReplicaScheduleDiskNotFound))
			continue
		}

		if !(types.IsDataEngineV1(volume.Spec.DataEngine) && diskSpec.Type == longhorn.DiskTypeFilesystem) &&
			!(types.IsDataEngineV2(volume.Spec.DataEngine) && diskSpec.Type == longhorn.DiskTypeBlock) {
			logrus.Debugf("Volume %v is not compatible with disk %v", volume.Name, diskName)
			continue
		}

		if !datastore.IsSupportedVolumeSize(volume.Spec.DataEngine, diskStatus.FSType, volume.Spec.Size) {
			logrus.Debugf("Volume %v size %v is not compatible with the file system %v of the disk %v", volume.Name, volume.Spec.Size, diskStatus.Type, diskName)
			continue
		}

		if requireSchedulingCheck {
			info, err := rcs.GetDiskSchedulingInfo(diskSpec, diskStatus)
			if err != nil {
				logrus.Errorf("Failed to get settings when scheduling replica: %v", err)
				multiError.Append(util.NewMultiError(longhorn.ErrorReplicaScheduleSchedulingSettingsRetrieveFailed))
				return preferredDisks, multiError
			}
			scheduledReplica := diskStatus.ScheduledReplica
			// check other replicas for the same volume has been accounted on current node
			var storageScheduled int64
			for rName, r := range replicas {
				if _, ok := scheduledReplica[rName]; !ok && r.Spec.NodeID != "" && r.Spec.NodeID == node.Name {
					storageScheduled += r.Spec.VolumeSize
				}
			}
			if storageScheduled > 0 {
				info.StorageScheduled += storageScheduled
			}
			if !rcs.IsSchedulableToDisk(volume.Spec.Size, volume.Status.ActualSize, info) {
				multiError.Append(util.NewMultiError(longhorn.ErrorReplicaScheduleInsufficientStorage))
				continue
			}
		}

		// Check if the Disk's Tags are valid.
		if !types.IsSelectorsInTags(diskSpec.Tags, volume.Spec.DiskSelector, allowEmptyDiskSelectorVolume) {
			multiError.Append(util.NewMultiError(longhorn.ErrorReplicaScheduleTagsNotFulfilled))
			continue
		}

		if volume.Spec.BackingImage != "" {
			// If the disks don't match the tags of the backing image of this volume,
			// don't schedule the replica on it because it will hang there
			if !types.IsSelectorsInTags(diskSpec.Tags, biDiskSelector, allowEmptyDiskSelectorVolume) {
				multiError.Append(util.NewMultiError(longhorn.ErrorReplicaScheduleTagsNotFulfilled))
				continue
			}
		}

		suggestDisk := &Disk{
			DiskSpec:   diskSpec,
			DiskStatus: diskStatus,
			NodeID:     node.Name,
		}
		preferredDisks[diskUUID] = suggestDisk
	}

	return preferredDisks, multiError
}

// filterDiskWithMatchingReplicas returns disk that have no matching replicas when diskSoftAntiAffinity is false.
// Otherwise, it returns the input disks map.
func filterDisksWithMatchingReplicas(disks map[string]*Disk, replicas map[string]*longhorn.Replica,
	diskSoftAntiAffinity, ignoreFailedReplicas bool) map[string]*Disk {
	replicasCountPerDisk := map[string]int{}
	for _, r := range replicas {
		if r.Spec.FailedAt != "" {
			if ignoreFailedReplicas {
				continue
			}
			if !IsPotentiallyReusableReplica(r) {
				continue // This replica can never be used again, so it does not count in scheduling decisions.
			}
		}
		replicasCountPerDisk[r.Spec.DiskID]++
	}

	disksByReplicaCount := map[int]map[string]*Disk{}
	for diskUUID, disk := range disks {
		count := replicasCountPerDisk[diskUUID]
		if disksByReplicaCount[count] == nil {
			disksByReplicaCount[count] = map[string]*Disk{}
		}
		disksByReplicaCount[count][diskUUID] = disk
	}

	if len(disksByReplicaCount[0]) > 0 || !diskSoftAntiAffinity {
		return disksByReplicaCount[0]
	}

	return disks
}

func (rcs *ReplicaScheduler) getNodeInfo() (map[string]*longhorn.Node, error) {
	nodeInfo, err := rcs.ds.ListNodes()
	if err != nil {
		return nil, err
	}

	scheduledNode := map[string]*longhorn.Node{}

	for _, node := range nodeInfo {
		if node == nil || node.DeletionTimestamp != nil {
			continue
		}

		nodeReadyCondition := types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeReady)
		nodeSchedulableCondition := types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeSchedulable)

		if nodeReadyCondition.Status != longhorn.ConditionStatusTrue {
			continue
		}
		if nodeSchedulableCondition.Status != longhorn.ConditionStatusTrue {
			continue
		}
		if !node.Spec.AllowScheduling {
			continue
		}
		scheduledNode[node.Name] = node
	}

	return scheduledNode, nil
}

func (rcs *ReplicaScheduler) scheduleReplicaToDisk(replica *longhorn.Replica, diskCandidates map[string]*Disk) {
	disk := rcs.getDiskWithMostUsableStorage(diskCandidates)
	replica.Spec.NodeID = disk.NodeID
	replica.Spec.DiskID = disk.DiskUUID
	replica.Spec.DiskPath = disk.Path
	replica.Spec.DataDirectoryName = replica.Spec.VolumeName + "-" + util.RandomID()

	logrus.WithFields(logrus.Fields{
		"replica":           replica.Name,
		"disk":              replica.Spec.DiskID,
		"diskPath":          replica.Spec.DiskPath,
		"dataDirectoryName": replica.Spec.DataDirectoryName,
	}).Infof("Schedule replica to node %v", replica.Spec.NodeID)
}

// Investigate
func (rcs *ReplicaScheduler) getDiskWithMostUsableStorage(disks map[string]*Disk) *Disk {
	diskWithMostUsableStorage := &Disk{}
	for _, disk := range disks {
		diskWithMostUsableStorage = disk
		break
	}

	for _, disk := range disks {
		diskWithMostStorageSize := diskWithMostUsableStorage.StorageAvailable - diskWithMostUsableStorage.StorageReserved
		diskSize := disk.StorageAvailable - disk.StorageReserved
		if diskWithMostStorageSize > diskSize {
			continue
		}

		diskWithMostUsableStorage = disk
	}

	return diskWithMostUsableStorage
}
func filterActiveReplicas(replicas map[string]*longhorn.Replica) map[string]*longhorn.Replica {
	result := map[string]*longhorn.Replica{}
	for _, r := range replicas {
		if r.Spec.Active {
			result[r.Name] = r
		}
	}
	return result
}

func (rcs *ReplicaScheduler) CheckAndReuseFailedReplica(replicas map[string]*longhorn.Replica, volume *longhorn.Volume, hardNodeAffinity string) (*longhorn.Replica, error) {
	if types.IsDataEngineV2(volume.Spec.DataEngine) {
		V2DataEngineFastReplicaRebuilding, err := rcs.ds.GetSettingAsBool(types.SettingNameV2DataEngineFastReplicaRebuilding)
		if err != nil {
			logrus.WithError(err).Warnf("Failed to get the setting %v, will consider it as false", types.SettingDefinitionV2DataEngineFastReplicaRebuilding)
			V2DataEngineFastReplicaRebuilding = false
		}
		if !V2DataEngineFastReplicaRebuilding {
			logrus.Infof("Skip checking and reusing replicas for volume %v since setting %v is not enabled", volume.Name, types.SettingNameV2DataEngineFastReplicaRebuilding)
			return nil, nil
		}
	}

	replicas = filterActiveReplicas(replicas)

	allNodesInfo, err := rcs.getNodeInfo()
	if err != nil {
		return nil, err
	}

	availableNodesInfo := map[string]*longhorn.Node{}
	availableNodeDisksMap := map[string]map[string]struct{}{}
	reusableNodeReplicasMap := map[string][]*longhorn.Replica{}
	for _, r := range replicas {
		isReusable, err := rcs.isFailedReplicaReusable(r, volume, allNodesInfo, hardNodeAffinity)
		if err != nil {
			return nil, err
		}
		if !isReusable {
			continue
		}

		disks, exists := availableNodeDisksMap[r.Spec.NodeID]
		if exists {
			disks[r.Spec.DiskID] = struct{}{}
		} else {
			disks = map[string]struct{}{r.Spec.DiskID: {}}
		}
		availableNodesInfo[r.Spec.NodeID] = allNodesInfo[r.Spec.NodeID]
		availableNodeDisksMap[r.Spec.NodeID] = disks

		if replicas, exists := reusableNodeReplicasMap[r.Spec.NodeID]; exists {
			reusableNodeReplicasMap[r.Spec.NodeID] = append(replicas, r)
		} else {
			reusableNodeReplicasMap[r.Spec.NodeID] = []*longhorn.Replica{r}
		}
	}

	// Call getDiskCandidates with ignoreFailedReplicas == true since we want the list of candidates to include disks
	// that already contain a failed replica.
	diskCandidates, _ := rcs.getDiskCandidates(availableNodesInfo, availableNodeDisksMap, replicas, volume, false, true)

	var reusedReplica *longhorn.Replica
	for _, suggestDisk := range diskCandidates {
		for _, r := range reusableNodeReplicasMap[suggestDisk.NodeID] {
			if r.Spec.DiskID != suggestDisk.DiskUUID {
				continue
			}
			if reusedReplica == nil {
				reusedReplica = r
				continue
			}
			reusedReplica = GetLatestFailedReplica(reusedReplica, r)
		}
	}
	if reusedReplica == nil {
		logrus.Infof("Cannot find a reusable failed replicas for volume %v", volume.Name)
		return nil, nil
	}

	return reusedReplica, nil
}

// RequireNewReplica is used to check if creating new replica immediately is necessary **after a reusable failed replica is not found**.
// If creating new replica immediately is necessary, returns 0.
// Otherwise, returns the duration that the caller should recheck.
// A new replica needs to be created when:
//  1. the volume is a new volume (volume.Status.Robustness is Empty)
//  2. data locality is required (hardNodeAffinity is not Empty and volume.Status.Robustness is Healthy)
//  3. replica eviction happens (volume.Status.Robustness is Healthy)
//  4. there is no potential reusable replica
//  5. there is potential reusable replica but the replica replenishment wait interval is passed.
func (rcs *ReplicaScheduler) RequireNewReplica(replicas map[string]*longhorn.Replica, volume *longhorn.Volume, hardNodeAffinity string) time.Duration {
	if volume.Status.Robustness != longhorn.VolumeRobustnessDegraded {
		return 0
	}
	if hardNodeAffinity != "" {
		return 0
	}

	hasPotentiallyReusableReplica := false
	for _, r := range replicas {
		if IsPotentiallyReusableReplica(r) {
			hasPotentiallyReusableReplica = true
			break
		}
	}
	if !hasPotentiallyReusableReplica {
		return 0
	}

	if types.IsDataEngineV2(volume.Spec.DataEngine) {
		V2DataEngineFastReplicaRebuilding, err := rcs.ds.GetSettingAsBool(types.SettingNameV2DataEngineFastReplicaRebuilding)
		if err != nil {
			logrus.WithError(err).Warnf("Failed to get the setting %v, will consider it as false", types.SettingDefinitionV2DataEngineFastReplicaRebuilding)
			V2DataEngineFastReplicaRebuilding = false
		}
		if !V2DataEngineFastReplicaRebuilding {
			logrus.Infof("Skip checking potentially reusable replicas for volume %v since setting %v is not enabled", volume.Name, types.SettingNameV2DataEngineFastReplicaRebuilding)
			return 0
		}
	}

	timeUntilNext, timeOfNext, err := rcs.timeToReplacementReplica(volume)
	if err != nil {
		msg := "Failed to get time until replica replacement, will directly replenish a new replica"
		logrus.WithError(err).Errorf("%s", msg)
	}
	if timeUntilNext > 0 {
		// Adding another second to the checkBackDuration to avoid clock skew.
		timeUntilNext = timeUntilNext + time.Second
		logrus.Infof("Replica replenishment is delayed until %v", timeOfNext.Add(time.Second))
	}
	return timeUntilNext
}

func (rcs *ReplicaScheduler) isFailedReplicaReusable(r *longhorn.Replica, v *longhorn.Volume, nodeInfo map[string]*longhorn.Node, hardNodeAffinity string) (bool, error) {
	// All failedReusableReplicas are also potentiallyFailedReusableReplicas.
	if !IsPotentiallyReusableReplica(r) {
		return false, nil
	}

	if hardNodeAffinity != "" && r.Spec.NodeID != hardNodeAffinity {
		return false, nil
	}

	if isReady, _ := rcs.ds.CheckDataEngineImageReadiness(r.Spec.Image, r.Spec.DataEngine, r.Spec.NodeID); !isReady {
		return false, nil
	}

	allowEmptyDiskSelectorVolume, err := rcs.ds.GetSettingAsBool(types.SettingNameAllowEmptyDiskSelectorVolume)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get %v setting", types.SettingNameAllowEmptyDiskSelectorVolume)
	}

	node, exists := nodeInfo[r.Spec.NodeID]
	if !exists {
		return false, nil
	}
	diskFound := false
	for diskName, diskStatus := range node.Status.DiskStatus {
		diskSpec, ok := node.Spec.Disks[diskName]
		if !ok {
			continue
		}

		if types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeReady).Status != longhorn.ConditionStatusTrue {
			continue
		}
		if types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeSchedulable).Status != longhorn.ConditionStatusTrue {
			// We want to reuse replica on the disk that is unschedulable due to allocated space being bigger than max allocable space but the disk is not full yet
			if types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeSchedulable).Reason != longhorn.DiskConditionReasonDiskPressure {
				continue
			}
			schedulingInfo, err := rcs.GetDiskSchedulingInfo(diskSpec, diskStatus)
			if err != nil {
				logrus.Warnf("failed to GetDiskSchedulingInfo of disk %v on node %v when checking replica %v is reusable: %v", diskName, node.Name, r.Name, err)
			}
			if !rcs.isDiskNotFull(schedulingInfo) {
				continue
			}
		}
		if diskStatus.DiskUUID == r.Spec.DiskID {
			diskFound = true
			diskSpec, exists := node.Spec.Disks[diskName]
			if !exists {
				return false, nil
			}
			if !diskSpec.AllowScheduling || diskSpec.EvictionRequested {
				return false, nil
			}
			if !types.IsSelectorsInTags(diskSpec.Tags, v.Spec.DiskSelector, allowEmptyDiskSelectorVolume) {
				return false, nil
			}
		}
	}
	if !diskFound {
		return false, nil
	}

	im, err := rcs.ds.GetInstanceManagerByInstanceRO(r)
	if err != nil {
		logrus.Errorf("Failed to get instance manager when checking replica %v is reusable: %v", r.Name, err)
		return false, nil
	}
	if im.DeletionTimestamp != nil || im.Status.CurrentState != longhorn.InstanceManagerStateRunning {
		return false, nil
	}

	return true, nil
}

// IsPotentiallyReusableReplica checks if a failed replica is potentially reusable. A potentially reusable replica means
// this failed replica may be able to reuse it later but it’s not valid now due to node/disk down issue.
func IsPotentiallyReusableReplica(r *longhorn.Replica) bool {
	if r.Spec.FailedAt == "" {
		return false
	}
	if r.Spec.NodeID == "" || r.Spec.DiskID == "" {
		return false
	}
	if r.Spec.RebuildRetryCount >= FailedReplicaMaxRetryCount {
		return false
	}
	if r.Spec.EvictionRequested {
		return false
	}
	return true
}

func GetLatestFailedReplica(rs ...*longhorn.Replica) (res *longhorn.Replica) {
	if rs == nil {
		return nil
	}

	var latestFailedAt time.Time
	for _, r := range rs {
		failedAt, err := util.ParseTime(r.Spec.FailedAt)
		if err != nil {
			logrus.Errorf("Failed to check replica %v failure timestamp %v: %v", r.Name, r.Spec.FailedAt, err)
			continue
		}
		if res == nil || failedAt.After(latestFailedAt) {
			res = r
			latestFailedAt = failedAt
		}
	}
	return res
}

func (rcs *ReplicaScheduler) IsSchedulableToDisk(size int64, requiredStorage int64, info *DiskSchedulingInfo) bool {
	// StorageReserved = the space is already used by 3rd party + the space will be used by 3rd party.
	// StorageAvailable = the space can be used by 3rd party or Longhorn system.
	// There is no (direct) relationship between StorageReserved and StorageAvailable.
	return info.StorageMaximum > 0 && info.StorageAvailable > 0 &&
		info.StorageAvailable-requiredStorage > int64(float64(info.StorageMaximum)*float64(info.MinimalAvailablePercentage)/100) &&
		(size+info.StorageScheduled) <= int64(float64(info.StorageMaximum-info.StorageReserved)*float64(info.OverProvisioningPercentage)/100)
}

func (rcs *ReplicaScheduler) IsSchedulableToDiskConsiderDiskPressure(diskPressurePercentage, size, requiredStorage int64, info *DiskSchedulingInfo) bool {
	log := logrus.WithFields(logrus.Fields{
		"diskUUID":               info.DiskUUID,
		"diskPressurePercentage": diskPressurePercentage,
		"requiredStorage":        requiredStorage,
		"storageScheduled":       info.StorageScheduled,
		"storageReserved":        info.StorageReserved,
		"storageMaximum":         info.StorageMaximum,
	})

	if info.StorageMaximum <= 0 {
		log.Warnf("StorageMaximum is %v, skip evaluating new disk usage", info.StorageMaximum)
		return false
	}

	newDiskUsagePercentage := (requiredStorage + info.StorageScheduled + info.StorageReserved) * 100 / info.StorageMaximum
	log.Debugf("Evaluated new disk usage percentage after scheduling replica: %v%%", newDiskUsagePercentage)

	return rcs.IsSchedulableToDisk(size, requiredStorage, info) &&
		newDiskUsagePercentage < int64(diskPressurePercentage)
}

// IsDiskUnderPressure checks if the disk is under pressure based the provided
// threshold percentage.
func (rcs *ReplicaScheduler) IsDiskUnderPressure(diskPressurePercentage int64, info *DiskSchedulingInfo) bool {
	storageUnusedPercentage := int64(0)
	storageUnused := info.StorageAvailable - info.StorageReserved
	if storageUnused > 0 && info.StorageMaximum > 0 {
		storageUnusedPercentage = storageUnused * 100 / info.StorageMaximum
	}
	return storageUnusedPercentage < 100-int64(diskPressurePercentage)
}

// FilterNodesSchedulableForVolume filters nodes that are schedulable for a given volume based on the disk space.
func (rcs *ReplicaScheduler) FilterNodesSchedulableForVolume(nodes map[string]*longhorn.Node, volume *longhorn.Volume) map[string]*longhorn.Node {
	filteredNodes := map[string]*longhorn.Node{}
	for _, node := range nodes {
		isSchedulable := false

		for diskName, diskStatus := range node.Status.DiskStatus {
			diskSpec, exists := node.Spec.Disks[diskName]
			if !exists {
				continue
			}

			diskInfo, err := rcs.GetDiskSchedulingInfo(diskSpec, diskStatus)
			if err != nil {
				logrus.WithError(err).Debugf("Failed to get disk scheduling info for disk %v on node %v", diskName, node.Name)
				continue
			}

			if rcs.IsSchedulableToDisk(volume.Spec.Size, volume.Status.ActualSize, diskInfo) {
				isSchedulable = true
				break
			}
		}

		if isSchedulable {
			logrus.Tracef("Found node %v schedulable for volume %v", node.Name, volume.Name)
			filteredNodes[node.Name] = node
		}
	}

	if len(filteredNodes) == 0 {
		logrus.Debugf("Found no nodes schedulable for volume %v", volume.Name)
	}
	return filteredNodes
}

func (rcs *ReplicaScheduler) isDiskNotFull(info *DiskSchedulingInfo) bool {
	// StorageAvailable = the space can be used by 3rd party or Longhorn system.
	return info.StorageMaximum > 0 && info.StorageAvailable > 0 &&
		info.StorageAvailable > int64(float64(info.StorageMaximum)*float64(info.MinimalAvailablePercentage)/100)
}

func (rcs *ReplicaScheduler) GetDiskSchedulingInfo(disk longhorn.DiskSpec, diskStatus *longhorn.DiskStatus) (*DiskSchedulingInfo, error) {
	// get StorageOverProvisioningPercentage and StorageMinimalAvailablePercentage settings
	overProvisioningPercentage, err := rcs.ds.GetSettingAsInt(types.SettingNameStorageOverProvisioningPercentage)
	if err != nil {
		return nil, err
	}
	minimalAvailablePercentage, err := rcs.ds.GetSettingAsInt(types.SettingNameStorageMinimalAvailablePercentage)
	if err != nil {
		return nil, err
	}
	info := &DiskSchedulingInfo{
		DiskUUID:                   diskStatus.DiskUUID,
		StorageAvailable:           diskStatus.StorageAvailable,
		StorageScheduled:           diskStatus.StorageScheduled,
		StorageReserved:            disk.StorageReserved,
		StorageMaximum:             diskStatus.StorageMaximum,
		OverProvisioningPercentage: overProvisioningPercentage,
		MinimalAvailablePercentage: minimalAvailablePercentage,
	}
	return info, nil
}

func (rcs *ReplicaScheduler) CheckReplicasSizeExpansion(v *longhorn.Volume, oldSize, newSize int64) (diskScheduleMultiError util.MultiError, err error) {
	defer func() {
		err = errors.Wrapf(err, "error while CheckReplicasSizeExpansion for volume %v", v.Name)
	}()

	replicas, err := rcs.ds.ListVolumeReplicas(v.Name)
	if err != nil {
		return nil, err
	}
	diskIDToReplicaCount := map[string]int64{}
	diskIDToDiskInfo := map[string]*DiskSchedulingInfo{}
	for _, r := range replicas {
		if r.Spec.NodeID == "" {
			continue
		}
		node, err := rcs.ds.GetNode(r.Spec.NodeID)
		if err != nil {
			return nil, err
		}
		diskSpec, diskStatus, ok := findDiskSpecAndDiskStatusInNode(r.Spec.DiskID, node)
		if !ok {
			return util.NewMultiError(longhorn.ErrorReplicaScheduleDiskNotFound),
				fmt.Errorf("cannot find the disk %v in node %v", r.Spec.DiskID, node.Name)
		}
		diskInfo, err := rcs.GetDiskSchedulingInfo(diskSpec, &diskStatus)
		if err != nil {
			return util.NewMultiError(longhorn.ErrorReplicaScheduleDiskUnavailable),
				fmt.Errorf("failed to GetDiskSchedulingInfo %v", err)
		}
		diskIDToDiskInfo[r.Spec.DiskID] = diskInfo
		diskIDToReplicaCount[r.Spec.DiskID] = diskIDToReplicaCount[r.Spec.DiskID] + 1
	}

	expandingSize := newSize - oldSize
	for diskID, diskInfo := range diskIDToDiskInfo {
		requestingSizeExpansionOnDisk := expandingSize * diskIDToReplicaCount[diskID]
		if !rcs.IsSchedulableToDisk(requestingSizeExpansionOnDisk, 0, diskInfo) {
			logrus.Errorf("Cannot schedule %v more bytes to disk %v with %+v", requestingSizeExpansionOnDisk, diskID, diskInfo)
			return util.NewMultiError(longhorn.ErrorReplicaScheduleInsufficientStorage),
				fmt.Errorf("cannot schedule %v more bytes to disk %v with %+v", requestingSizeExpansionOnDisk, diskID, diskInfo)
		}
	}
	return nil, nil
}

func findDiskSpecAndDiskStatusInNode(diskUUID string, node *longhorn.Node) (longhorn.DiskSpec, longhorn.DiskStatus, bool) {
	for diskName, diskStatus := range node.Status.DiskStatus {
		if diskStatus.DiskUUID == diskUUID {
			diskSpec := node.Spec.Disks[diskName]
			return diskSpec, *diskStatus, true
		}
	}
	return longhorn.DiskSpec{}, longhorn.DiskStatus{}, false
}

// getCurrentNodesAndZones returns the nodes and zones a replica is already scheduled to.
//   - Some callers do not consider a node or zone to be used if it contains a failed replica.
//     ignoreFailedReplicas == true supports this use case.
//   - Otherwise, getCurrentNodesAndZones does not consider a node or zone to be occupied by a failed replica that can
//     no longer be used or is likely actively being replaced. This makes nodes and zones with useless replicas
//     available for scheduling.
func getCurrentNodesAndZones(replicas map[string]*longhorn.Replica, nodeInfo map[string]*longhorn.Node,
	ignoreFailedReplicas, creatingNewReplicasForReplenishment bool) (map[string]*longhorn.Node,
	map[string]bool, map[string]bool, map[string]bool) {
	usedNodes := map[string]*longhorn.Node{}
	usedZones := map[string]bool{}
	onlyEvictingNodes := map[string]bool{}
	onlyEvictingZones := map[string]bool{}

	for _, r := range replicas {
		if r.Spec.NodeID == "" {
			continue
		}
		if r.DeletionTimestamp != nil {
			continue
		}
		if r.Spec.FailedAt != "" {
			if ignoreFailedReplicas {
				continue
			}
			if !IsPotentiallyReusableReplica(r) {
				continue // This replica can never be used again, so it does not count in scheduling decisions.
			}
			if creatingNewReplicasForReplenishment {
				continue // Maybe this replica can be used again, but it is being actively replaced anyway.
			}
		}

		if node, ok := nodeInfo[r.Spec.NodeID]; ok {
			if r.Spec.EvictionRequested {
				if _, ok := usedNodes[r.Spec.NodeID]; !ok {
					// This is an evicting replica on a thus far unused node. We won't change this again unless we
					// find a non-evicting replica on this node.
					onlyEvictingNodes[node.Name] = true
				}
				if used := usedZones[node.Status.Zone]; !used {
					// This is an evicting replica in a thus far unused zone. We won't change this again unless we
					// find a non-evicting replica in this zone.
					onlyEvictingZones[node.Status.Zone] = true
				}
			} else {
				// There is now at least one replica on this node and in this zone that is not evicting.
				onlyEvictingNodes[node.Name] = false
				onlyEvictingZones[node.Status.Zone] = false
			}

			usedNodes[node.Name] = node
			// For empty zone label, we treat them as one zone.
			usedZones[node.Status.Zone] = true
		}
	}

	return usedNodes, usedZones, onlyEvictingNodes, onlyEvictingZones
}

// timeToReplacementReplica returns the amount of time until Longhorn should create a new replica for a degraded volume,
// even if there are potentially reusable failed replicas. It returns 0 if replica-replenishment-wait-interval has
// elapsed and a new replica is needed right now.
func (rcs *ReplicaScheduler) timeToReplacementReplica(volume *longhorn.Volume) (time.Duration, time.Time, error) {
	settingValue, err := rcs.ds.GetSettingAsInt(types.SettingNameReplicaReplenishmentWaitInterval)
	if err != nil {
		err = errors.Wrapf(err, "failed to get setting ReplicaReplenishmentWaitInterval")
		return 0, time.Time{}, err
	}
	waitInterval := time.Duration(settingValue) * time.Second

	lastDegradedAt, err := util.ParseTime(volume.Status.LastDegradedAt)
	if err != nil {
		err = errors.Wrapf(err, "failed to parse last degraded timestamp %v", volume.Status.LastDegradedAt)
		return 0, time.Time{}, err
	}

	now := rcs.nowHandler()
	timeOfNext := lastDegradedAt.Add(waitInterval)
	if now.After(timeOfNext) {
		// A replacement replica is needed now.
		return 0, time.Time{}, nil
	}

	return timeOfNext.Sub(now), timeOfNext, nil
}
