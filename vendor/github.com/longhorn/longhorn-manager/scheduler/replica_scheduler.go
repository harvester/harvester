package scheduler

import (
	"fmt"
	"sort"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
)

const (
	FailedReplicaMaxRetryCount = 5
)

type ReplicaScheduler struct {
	ds *datastore.DataStore
}

type Disk struct {
	longhorn.DiskSpec
	*longhorn.DiskStatus
	NodeID string
}

type DiskSchedulingInfo struct {
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
	}
	return rcScheduler
}

// ScheduleReplica will return (nil, nil) for unschedulable replica
func (rcs *ReplicaScheduler) ScheduleReplica(replica *longhorn.Replica, replicas map[string]*longhorn.Replica, volume *longhorn.Volume) (*longhorn.Replica, error) {
	// only called when replica is starting for the first time
	if replica.Spec.NodeID != "" {
		return nil, fmt.Errorf("BUG: Replica %v has been scheduled to node %v", replica.Name, replica.Spec.NodeID)
	}

	// get all hosts
	nodesInfo, err := rcs.getNodeInfo()
	if err != nil {
		return nil, err
	}

	nodeCandidates := rcs.getNodeCandidates(nodesInfo, replica)

	if len(nodeCandidates) == 0 {
		logrus.Errorf("There's no available node for replica %v, size %v", replica.ObjectMeta.Name, replica.Spec.VolumeSize)
		return nil, nil
	}

	nodeDisksMap := map[string]map[string]struct{}{}
	for _, node := range nodeCandidates {
		disks := map[string]struct{}{}
		for fsid, diskStatus := range node.Status.DiskStatus {
			diskSpec, exists := node.Spec.Disks[fsid]
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

	diskCandidates := rcs.getDiskCandidates(nodeCandidates, nodeDisksMap, replicas, volume, true)

	// there's no disk that fit for current replica
	if len(diskCandidates) == 0 {
		logrus.Errorf("There's no available disk for replica %v, size %v", replica.ObjectMeta.Name, replica.Spec.VolumeSize)
		return nil, nil
	}

	// schedule replica to disk
	rcs.scheduleReplicaToDisk(replica, diskCandidates)

	return replica, nil
}

func (rcs *ReplicaScheduler) getNodeCandidates(nodesInfo map[string]*longhorn.Node, schedulingReplica *longhorn.Replica) map[string]*longhorn.Node {
	if schedulingReplica.Spec.HardNodeAffinity != "" {
		node, exist := nodesInfo[schedulingReplica.Spec.HardNodeAffinity]
		if !exist {
			return nil
		}
		nodesInfo = map[string]*longhorn.Node{}
		nodesInfo[schedulingReplica.Spec.HardNodeAffinity] = node
	}

	nodeCandidates := map[string]*longhorn.Node{}

	for _, node := range nodesInfo {
		if isReady, _ := rcs.ds.CheckEngineImageReadiness(schedulingReplica.Spec.EngineImage, node.Name); isReady {
			nodeCandidates[node.Name] = node
		}
	}

	return nodeCandidates
}

// getNodesWithEvictingReplicas returns nodes that have replicas being evicted
func getNodesWithEvictingReplicas(replicas map[string]*longhorn.Replica, nodeInfo map[string]*longhorn.Node) map[string]*longhorn.Node {
	nodesWithEvictingReplicas := map[string]*longhorn.Node{}
	for _, r := range replicas {
		if r.Status.EvictionRequested {
			if node, ok := nodeInfo[r.Spec.NodeID]; ok {
				nodesWithEvictingReplicas[r.Spec.NodeID] = node
			}
		}
	}
	return nodesWithEvictingReplicas
}

func (rcs *ReplicaScheduler) getDiskCandidates(nodeInfo map[string]*longhorn.Node, nodeDisksMap map[string]map[string]struct{}, replicas map[string]*longhorn.Replica, volume *longhorn.Volume, requireSchedulingCheck bool) map[string]*Disk {
	nodeSoftAntiAffinity, err :=
		rcs.ds.GetSettingAsBool(types.SettingNameReplicaSoftAntiAffinity)
	if err != nil {
		logrus.Errorf("error getting replica soft anti-affinity setting: %v", err)
	}

	zoneSoftAntiAffinity, err :=
		rcs.ds.GetSettingAsBool(types.SettingNameReplicaZoneSoftAntiAffinity)
	if err != nil {
		logrus.Errorf("Error getting replica zone soft anti-affinity setting: %v", err)
	}

	getDiskCandidatesFromNodes := func(nodes map[string]*longhorn.Node) map[string]*Disk {
		for _, node := range nodes {
			if diskCandidates := rcs.filterNodeDisksForReplica(node, nodeDisksMap[node.Name], replicas, volume, requireSchedulingCheck); len(diskCandidates) > 0 {
				return diskCandidates
			}
		}
		return map[string]*Disk{}
	}

	usedNodes := map[string]*longhorn.Node{}
	usedZones := map[string]bool{}
	replicasCountPerNode := map[string]int{}
	// Get current nodes and zones
	for _, r := range replicas {
		if r.Spec.NodeID != "" && r.DeletionTimestamp == nil && r.Spec.FailedAt == "" {
			if node, ok := nodeInfo[r.Spec.NodeID]; ok {
				usedNodes[r.Spec.NodeID] = node
				// For empty zone label, we treat them as
				// one zone.
				usedZones[node.Status.Zone] = true
				replicasCountPerNode[r.Spec.NodeID] = replicasCountPerNode[r.Spec.NodeID] + 1
			}
		}
	}

	filterNodesWithLessThanTwoReplicas := func(nodes map[string]*longhorn.Node) map[string]*longhorn.Node {
		result := map[string]*longhorn.Node{}
		for nodeName, node := range nodes {
			if replicasCountPerNode[nodeName] < 2 {
				result[nodeName] = node
			}
		}
		return result
	}

	unusedNodes := map[string]*longhorn.Node{}
	unusedNodesInNewZones := map[string]*longhorn.Node{}
	nodesInUnusedZones := map[string]*longhorn.Node{}
	nodesWithEvictingReplicas := getNodesWithEvictingReplicas(replicas, nodeInfo)

	for nodeName, node := range nodeInfo {
		// Filter Nodes. If the Nodes don't match the tags, don't bother marking them as candidates.
		if !rcs.checkTagsAreFulfilled(node.Spec.Tags, volume.Spec.NodeSelector) {
			continue
		}
		if _, ok := usedNodes[nodeName]; !ok {
			unusedNodes[nodeName] = node
			if _, ok := usedZones[node.Status.Zone]; !ok {
				unusedNodesInNewZones[nodeName] = node
			}
		}
		if _, ok := usedZones[node.Status.Zone]; !ok {
			nodesInUnusedZones[nodeName] = node
		}
	}

	switch {
	case !zoneSoftAntiAffinity && !nodeSoftAntiAffinity:
		if diskCandidates := getDiskCandidatesFromNodes(unusedNodesInNewZones); len(diskCandidates) > 0 {
			return diskCandidates
		}
		if diskCandidates := getDiskCandidatesFromNodes(filterNodesWithLessThanTwoReplicas(nodesWithEvictingReplicas)); len(diskCandidates) > 0 {
			return diskCandidates
		}
	case zoneSoftAntiAffinity && !nodeSoftAntiAffinity:
		if diskCandidates := getDiskCandidatesFromNodes(unusedNodesInNewZones); len(diskCandidates) > 0 {
			return diskCandidates
		}
		if diskCandidates := getDiskCandidatesFromNodes(unusedNodes); len(diskCandidates) > 0 {
			return diskCandidates
		}
		if diskCandidates := getDiskCandidatesFromNodes(filterNodesWithLessThanTwoReplicas(nodesWithEvictingReplicas)); len(diskCandidates) > 0 {
			return diskCandidates
		}
	case !zoneSoftAntiAffinity && nodeSoftAntiAffinity:
		if diskCandidates := getDiskCandidatesFromNodes(unusedNodesInNewZones); len(diskCandidates) > 0 {
			return diskCandidates
		}
		if diskCandidates := getDiskCandidatesFromNodes(nodesInUnusedZones); len(diskCandidates) > 0 {
			return diskCandidates
		}
		if diskCandidates := getDiskCandidatesFromNodes(nodesWithEvictingReplicas); len(diskCandidates) > 0 {
			return diskCandidates
		}
	case zoneSoftAntiAffinity && nodeSoftAntiAffinity:
		if diskCandidates := getDiskCandidatesFromNodes(unusedNodesInNewZones); len(diskCandidates) > 0 {
			return diskCandidates
		}
		if diskCandidates := getDiskCandidatesFromNodes(unusedNodes); len(diskCandidates) > 0 {
			return diskCandidates
		}
		if diskCandidates := getDiskCandidatesFromNodes(usedNodes); len(diskCandidates) > 0 {
			return diskCandidates
		}
	}
	return map[string]*Disk{}
}

func (rcs *ReplicaScheduler) filterNodeDisksForReplica(node *longhorn.Node, disks map[string]struct{}, replicas map[string]*longhorn.Replica, volume *longhorn.Volume, requireSchedulingCheck bool) map[string]*Disk {
	preferredDisk := map[string]*Disk{}
	// find disk that fit for current replica
	for diskUUID := range disks {
		var fsid string
		var diskSpec longhorn.DiskSpec
		var diskStatus *longhorn.DiskStatus
		diskFound := false
		for fsid, diskStatus = range node.Status.DiskStatus {
			if types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeSchedulable).Status == longhorn.ConditionStatusTrue &&
				diskStatus.DiskUUID == diskUUID {
				diskFound = true
				diskSpec = node.Spec.Disks[fsid]
				break
			}
		}
		if !diskFound {
			logrus.Errorf("Cannot find the spec or the status for disk %v when scheduling replica", diskUUID)
			continue
		}
		if requireSchedulingCheck {
			info, err := rcs.GetDiskSchedulingInfo(diskSpec, diskStatus)
			if err != nil {
				logrus.Errorf("Fail to get settings when scheduling replica: %v", err)
				return preferredDisk
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
				continue
			}
		}

		// Check if the Disk's Tags are valid.
		if !rcs.checkTagsAreFulfilled(diskSpec.Tags, volume.Spec.DiskSelector) {
			continue
		}

		suggestDisk := &Disk{
			DiskSpec:   diskSpec,
			DiskStatus: diskStatus,
			NodeID:     node.Name,
		}
		preferredDisk[diskUUID] = suggestDisk
	}

	return preferredDisk
}

func (rcs *ReplicaScheduler) checkTagsAreFulfilled(itemTags, volumeTags []string) bool {
	if !sort.StringsAreSorted(itemTags) {
		logrus.Warnf("BUG: Tags are not sorted, sort now")
		sort.Strings(itemTags)
	}

	for _, tag := range volumeTags {
		if index := sort.SearchStrings(itemTags, tag); index >= len(itemTags) || itemTags[index] != tag {
			return false
		}
	}

	return true
}

func (rcs *ReplicaScheduler) getNodeInfo() (map[string]*longhorn.Node, error) {
	nodeInfo, err := rcs.ds.ListNodes()
	if err != nil {
		return nil, err
	}
	scheduledNode := map[string]*longhorn.Node{}

	for _, node := range nodeInfo {
		// First check node ready condition
		nodeReadyCondition := types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeReady)
		// Get Schedulable condition
		nodeSchedulableCondition :=
			types.GetCondition(node.Status.Conditions,
				longhorn.NodeConditionTypeSchedulable)
		if node != nil && node.DeletionTimestamp == nil &&
			nodeReadyCondition.Status == longhorn.ConditionStatusTrue &&
			nodeSchedulableCondition.Status == longhorn.ConditionStatusTrue &&
			node.Spec.AllowScheduling {
			scheduledNode[node.Name] = node
		}
	}
	return scheduledNode, nil
}

func (rcs *ReplicaScheduler) scheduleReplicaToDisk(replica *longhorn.Replica, diskCandidates map[string]*Disk) {
	// get a random disk from diskCandidates
	for _, disk := range diskCandidates {
		replica.Spec.NodeID = disk.NodeID
		replica.Spec.DiskID = disk.DiskUUID
		replica.Spec.DiskPath = disk.Path
		break
	}
	replica.Spec.DataDirectoryName = replica.Spec.VolumeName + "-" + util.RandomID()
	logrus.Debugf("Schedule replica %v to node %v, disk %v, diskPath %v, dataDirectoryName %v",
		replica.Name, replica.Spec.NodeID, replica.Spec.DiskID, replica.Spec.DiskPath, replica.Spec.DataDirectoryName)
}

func (rcs *ReplicaScheduler) CheckAndReuseFailedReplica(replicas map[string]*longhorn.Replica, volume *longhorn.Volume, hardNodeAffinity string) (*longhorn.Replica, error) {
	allNodesInfo, err := rcs.getNodeInfo()
	if err != nil {
		return nil, err
	}

	availableNodesInfo := map[string]*longhorn.Node{}
	availableNodeDisksMap := map[string]map[string]struct{}{}
	reusableNodeReplicasMap := map[string][]*longhorn.Replica{}
	for _, r := range replicas {
		if !rcs.isFailedReplicaReusable(r, volume, allNodesInfo, hardNodeAffinity) {
			continue
		}

		disks, exists := availableNodeDisksMap[r.Spec.NodeID]
		if exists {
			disks[r.Spec.DiskID] = struct{}{}
		} else {
			disks = map[string]struct{}{r.Spec.DiskID: struct{}{}}
		}
		availableNodesInfo[r.Spec.NodeID] = allNodesInfo[r.Spec.NodeID]
		availableNodeDisksMap[r.Spec.NodeID] = disks

		if _, exists := reusableNodeReplicasMap[r.Spec.NodeID]; exists {
			reusableNodeReplicasMap[r.Spec.NodeID] = append(reusableNodeReplicasMap[r.Spec.NodeID], r)
		} else {
			reusableNodeReplicasMap[r.Spec.NodeID] = []*longhorn.Replica{r}
		}
	}

	diskCandidates := rcs.getDiskCandidates(availableNodesInfo, availableNodeDisksMap, replicas, volume, false)

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
		logrus.Errorf("Cannot pick up a reusable failed replicas")
		return nil, nil
	}

	return reusedReplica, nil
}

// RequireNewReplica is used to check if creating new replica immediately is necessary **after a reusable failed replica is not found**.
// A new replica needs to be created when:
//   1. the volume is a new volume (volume.Status.Robustness is Empty)
//   2. data locality is required (hardNodeAffinity is not Empty and volume.Status.Robustness is Healthy)
//   3. replica eviction happens (volume.Status.Robustness is Healthy)
//   4. there is no potential reusable replica
//   5. there is potential reusable replica but the replica replenishment wait interval is passed.
func (rcs *ReplicaScheduler) RequireNewReplica(replicas map[string]*longhorn.Replica, volume *longhorn.Volume, hardNodeAffinity string) bool {
	if volume.Status.Robustness != longhorn.VolumeRobustnessDegraded {
		return true
	}
	if hardNodeAffinity != "" {
		return true
	}

	hasPotentiallyReusableReplica := false
	for _, r := range replicas {
		if IsPotentiallyReusableReplica(r, hardNodeAffinity) {
			hasPotentiallyReusableReplica = true
			break
		}
	}
	if !hasPotentiallyReusableReplica {
		return true
	}

	// Otherwise Longhorn will relay the new replica creation then there is a chance to reuse failed replicas later.
	settingValue, err := rcs.ds.GetSettingAsInt(types.SettingNameReplicaReplenishmentWaitInterval)
	if err != nil {
		logrus.Errorf("Failed to get Setting ReplicaReplenishmentWaitInterval, will directly replenish a new replica: %v", err)
		return true
	}
	waitInterval := time.Duration(settingValue) * time.Second
	lastDegradedAt, err := util.ParseTime(volume.Status.LastDegradedAt)
	if err != nil {
		logrus.Errorf("Failed to get parse volume last degraded timestamp %v, will directly replenish a new replica: %v", volume.Status.LastDegradedAt, err)
		return true
	}
	if time.Now().After(lastDegradedAt.Add(waitInterval)) {
		return true
	}

	logrus.Debugf("Replica replenishment is delayed until %v", lastDegradedAt.Add(waitInterval))
	return false
}

func (rcs *ReplicaScheduler) isFailedReplicaReusable(r *longhorn.Replica, v *longhorn.Volume, nodeInfo map[string]*longhorn.Node, hardNodeAffinity string) bool {
	if r.Spec.FailedAt == "" {
		return false
	}
	if r.Spec.NodeID == "" || r.Spec.DiskID == "" {
		return false
	}
	if r.Spec.RebuildRetryCount >= FailedReplicaMaxRetryCount {
		return false
	}
	if r.Status.EvictionRequested {
		return false
	}
	if hardNodeAffinity != "" && r.Spec.NodeID != hardNodeAffinity {
		return false
	}
	if isReady, _ := rcs.ds.CheckEngineImageReadiness(r.Spec.EngineImage, r.Spec.NodeID); !isReady {
		return false
	}

	node, exists := nodeInfo[r.Spec.NodeID]
	if !exists {
		return false
	}
	diskFound := false
	for diskName, diskStatus := range node.Status.DiskStatus {
		if types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeReady).Status != longhorn.ConditionStatusTrue {
			continue
		}
		if types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeSchedulable).Status != longhorn.ConditionStatusTrue {
			continue
		}
		if diskStatus.DiskUUID == r.Spec.DiskID {
			diskFound = true
			diskSpec, exists := node.Spec.Disks[diskName]
			if !exists {
				return false
			}
			if !diskSpec.AllowScheduling || diskSpec.EvictionRequested {
				return false
			}
			if !rcs.checkTagsAreFulfilled(diskSpec.Tags, v.Spec.DiskSelector) {
				return false
			}
		}
	}
	if !diskFound {
		return false
	}

	im, err := rcs.ds.GetInstanceManagerByInstance(r)
	if err != nil {
		logrus.Errorf("failed to get instance manager when checking replica %v is reusable: %v", r.Name, err)
		return false
	}
	if im.DeletionTimestamp != nil || im.Status.CurrentState != longhorn.InstanceManagerStateRunning {
		return false
	}

	return true
}

// IsPotentiallyReusableReplica is used to check if a failed replica is potentially reusable.
// A potentially reusable replica means this failed replica may be able to reuse it later but it’s not valid now due to node/disk down issue.
func IsPotentiallyReusableReplica(r *longhorn.Replica, hardNodeAffinity string) bool {
	if r.Spec.FailedAt == "" {
		return false
	}
	if r.Spec.NodeID == "" || r.Spec.DiskID == "" {
		return false
	}
	if r.Spec.RebuildRetryCount >= FailedReplicaMaxRetryCount {
		return false
	}
	if r.Status.EvictionRequested {
		return false
	}
	if hardNodeAffinity != "" && r.Spec.NodeID != hardNodeAffinity {
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
		StorageAvailable:           diskStatus.StorageAvailable,
		StorageScheduled:           diskStatus.StorageScheduled,
		StorageReserved:            disk.StorageReserved,
		StorageMaximum:             diskStatus.StorageMaximum,
		OverProvisioningPercentage: overProvisioningPercentage,
		MinimalAvailablePercentage: minimalAvailablePercentage,
	}
	return info, nil
}

func (rcs *ReplicaScheduler) CheckReplicasSizeExpansion(v *longhorn.Volume, oldSize, newSize int64) (err error) {
	defer func() {
		err = errors.Wrapf(err, "error while CheckReplicasSizeExpansion for volume %v", v.Name)
	}()

	replicas, err := rcs.ds.ListVolumeReplicas(v.Name)
	if err != nil {
		return err
	}
	diskIDToReplicaCount := map[string]int64{}
	diskIDToDiskInfo := map[string]*DiskSchedulingInfo{}
	for _, r := range replicas {
		if r.Spec.NodeID == "" {
			continue
		}
		node, err := rcs.ds.GetNode(r.Spec.NodeID)
		if err != nil {
			return err
		}
		diskSpec, diskStatus, ok := findDiskSpecAndDiskStatusInNode(r.Spec.DiskID, node)
		if !ok {
			return fmt.Errorf("cannot find the disk %v in node %v", r.Spec.DiskID, node.Name)
		}
		diskInfo, err := rcs.GetDiskSchedulingInfo(diskSpec, &diskStatus)
		if err != nil {
			return fmt.Errorf("failed to GetDiskSchedulingInfo %v", err)
		}
		diskIDToDiskInfo[r.Spec.DiskID] = diskInfo
		diskIDToReplicaCount[r.Spec.DiskID] = diskIDToReplicaCount[r.Spec.DiskID] + 1
	}

	expandingSize := newSize - oldSize
	for diskID, diskInfo := range diskIDToDiskInfo {
		requestingSizeExpansionOnDisk := expandingSize * diskIDToReplicaCount[diskID]
		if !rcs.IsSchedulableToDisk(requestingSizeExpansionOnDisk, 0, diskInfo) {
			logrus.Errorf("Cannot schedule %v more bytes to disk %v with %+v", requestingSizeExpansionOnDisk, diskID, diskInfo)
			return fmt.Errorf("cannot schedule %v more bytes to disk %v with %+v", requestingSizeExpansionOnDisk, diskID, diskInfo)
		}
	}
	return nil
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
