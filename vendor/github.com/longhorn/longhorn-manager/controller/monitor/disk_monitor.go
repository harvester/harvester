package monitor

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/jinzhu/copier"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	NodeMonitorSyncPeriod = 30 * time.Second

	volumeMetaData = "volume.meta"
)

type NodeMonitor struct {
	*baseMonitor

	nodeName        string
	checkVolumeMeta bool

	collectedDataLock sync.RWMutex
	collectedData     map[string]*CollectedDiskInfo

	syncCallback func(key string)

	getDiskStatHandler             GetDiskStatHandler
	getDiskConfigHandler           GetDiskConfigHandler
	generateDiskConfigHandler      GenerateDiskConfigHandler
	getReplicaInstanceNamesHandler GetReplicaInstanceNamesHandler
}

type CollectedDiskInfo struct {
	Path                          string
	NodeOrDiskEvicted             bool
	DiskStat                      *util.DiskStat
	DiskUUID                      string
	Condition                     *longhorn.Condition
	OrphanedReplicaDirectoryNames map[string]string
}

type GetDiskStatHandler func(longhorn.DiskType, string, string, *engineapi.DiskService) (*util.DiskStat, error)
type GetDiskConfigHandler func(longhorn.DiskType, string, string, *engineapi.DiskService) (*util.DiskConfig, error)
type GenerateDiskConfigHandler func(longhorn.DiskType, string, string, string, *engineapi.DiskService) (*util.DiskConfig, error)
type GetReplicaInstanceNamesHandler func(longhorn.DiskType, *longhorn.Node, string, string, string, *engineapi.DiskService) (map[string]string, error)

func NewDiskMonitor(logger logrus.FieldLogger, ds *datastore.DataStore, nodeName string, syncCallback func(key string)) (*NodeMonitor, error) {
	ctx, quit := context.WithCancel(context.Background())

	m := &NodeMonitor{
		baseMonitor: newBaseMonitor(ctx, quit, logger, ds, NodeMonitorSyncPeriod),

		nodeName:        nodeName,
		checkVolumeMeta: true,

		collectedDataLock: sync.RWMutex{},
		collectedData:     make(map[string]*CollectedDiskInfo, 0),

		syncCallback: syncCallback,

		getDiskStatHandler:             getDiskStat,
		getDiskConfigHandler:           getDiskConfig,
		generateDiskConfigHandler:      generateDiskConfig,
		getReplicaInstanceNamesHandler: getReplicaInstanceNames,
	}

	go m.Start()

	return m, nil
}

func (m *NodeMonitor) Start() {
	wait.PollImmediateUntil(m.syncPeriod, func() (done bool, err error) {
		if err := m.run(struct{}{}); err != nil {
			m.logger.Errorf("Stop monitoring because of %v", err)
		}
		return false, nil
	}, m.ctx.Done())
}

func (m *NodeMonitor) Close() {
	m.quit()
}

func (m *NodeMonitor) RunOnce() error {
	return m.run(struct{}{})
}

func (m *NodeMonitor) UpdateConfiguration(map[string]interface{}) error {
	return nil
}

func (m *NodeMonitor) GetCollectedData() (interface{}, error) {
	m.collectedDataLock.RLock()
	defer m.collectedDataLock.RUnlock()

	data := make(map[string]*CollectedDiskInfo, 0)
	if err := copier.CopyWithOption(&data, &m.collectedData, copier.Option{IgnoreEmpty: true, DeepCopy: true}); err != nil {
		return data, errors.Wrap(err, "failed to copy node monitor collected data")
	}

	return data, nil
}

func (m *NodeMonitor) run(value interface{}) error {
	node, err := m.ds.GetNode(m.nodeName)
	if err != nil {
		return errors.Wrapf(err, "failed to get longhorn node %v", m.nodeName)
	}

	collectedData := m.collectDiskData(node)
	if !reflect.DeepEqual(m.collectedData, collectedData) {
		func() {
			m.collectedDataLock.Lock()
			defer m.collectedDataLock.Unlock()
			m.collectedData = collectedData
		}()

		key := node.Namespace + "/" + m.nodeName
		m.syncCallback(key)
	}

	return nil
}

func (m *NodeMonitor) newDiskServiceClient(node *longhorn.Node) (*engineapi.DiskService, error) {
	im, err := m.ds.GetDefaultInstanceManagerByNode(m.nodeName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get default instance manager for node %v", m.nodeName)
	}

	return engineapi.NewDiskServiceClient(im, m.logger)
}

// Collect disk data and generate disk UUID blindly.
func (m *NodeMonitor) collectDiskData(node *longhorn.Node) map[string]*CollectedDiskInfo {
	diskInfoMap := make(map[string]*CollectedDiskInfo, 0)

	v2DataEngineEnabled, err := m.ds.GetSettingAsBool(types.SettingNameV2DataEngine)
	if err != nil {
		m.logger.Errorf("Failed to get setting %v: %v", types.SettingNameV2DataEngine, err)
		return diskInfoMap
	}

	diskServiceClient, err := m.newDiskServiceClient(node)
	if err != nil {
		// If failed to create disk service client, just log a warning and continue.
		// The error out will hinder the following logics in syncNode.
		logrus.WithError(err).Warnf("Failed to create disk service client")
	}
	defer func() {
		if diskServiceClient != nil {
			diskServiceClient.Close()
		}
	}()

	for diskName, disk := range node.Spec.Disks {
		if !v2DataEngineEnabled && disk.Type == longhorn.DiskTypeBlock {
			continue
		}

		orphanedReplicaDirectoryNames := map[string]string{}

		nodeOrDiskEvicted := isNodeOrDiskEvicted(node, disk)

		diskConfig, err := m.getDiskConfigHandler(disk.Type, diskName, disk.Path, diskServiceClient)
		if err != nil {
			if !types.ErrorIsNotFound(err) {
				diskInfoMap[diskName] = NewDiskInfo(disk.Path, "", nodeOrDiskEvicted, nil,
					orphanedReplicaDirectoryNames, string(longhorn.DiskConditionReasonNoDiskInfo),
					fmt.Sprintf("Disk %v(%v) on node %v is not ready: failed to get disk config: error: %v",
						diskName, disk.Path, node.Name, err))
				continue
			}

			diskUUID := ""
			if node.Status.DiskStatus != nil {
				if diskStatus, ok := node.Status.DiskStatus[diskName]; ok {
					diskUUID = diskStatus.DiskUUID
				}
			}

			// Filesystem-type disk
			//   Blindly check or generate disk config.
			//   The handling of all disks containing the same fsid will be done in NodeController.
			// Block-type disk
			//   Create a bdev lvstore
			if diskConfig, err = m.generateDiskConfigHandler(disk.Type, diskName, diskUUID, disk.Path, diskServiceClient); err != nil {
				diskInfoMap[diskName] = NewDiskInfo(disk.Path, "", nodeOrDiskEvicted, nil,
					orphanedReplicaDirectoryNames, string(longhorn.DiskConditionReasonNoDiskInfo),
					fmt.Sprintf("Disk %v(%v) on node %v is not ready: failed to generate disk config: error: %v",
						diskName, disk.Path, node.Name, err))
				continue
			}
		}

		stat, err := m.getDiskStatHandler(disk.Type, diskName, disk.Path, diskServiceClient)
		if err != nil {
			diskInfoMap[diskName] = NewDiskInfo(disk.Path, "", nodeOrDiskEvicted, nil,
				orphanedReplicaDirectoryNames, string(longhorn.DiskConditionReasonNoDiskInfo),
				fmt.Sprintf("Disk %v(%v) on node %v is not ready: Get disk information error: %v",
					diskName, node.Spec.Disks[diskName].Path, node.Name, err))
			continue
		}

		replicaInstanceNames, err := m.getReplicaInstanceNamesHandler(disk.Type, node, diskName, diskConfig.DiskUUID, disk.Path, diskServiceClient)
		if err != nil {
			logrus.WithError(err).Warnf("Failed to get replica instance names for disk %v(%v) on node %v", diskName, disk.Path, node.Name)
			continue
		}

		orphanedReplicaDirectoryNames, err = m.getOrphanedReplicaInstanceNames(disk.Type, node, diskName, diskConfig.DiskUUID, disk.Path, replicaInstanceNames)
		if err != nil {
			logrus.WithError(err).Warnf("Failed to get orphaned replica instance names for disk %v(%v) on node %v", diskName, disk.Path, node.Name)
			continue
		}

		diskInfoMap[diskName] = NewDiskInfo(disk.Path, diskConfig.DiskUUID, nodeOrDiskEvicted, stat,
			orphanedReplicaDirectoryNames, string(longhorn.DiskConditionReasonNoDiskInfo), "")
	}

	return diskInfoMap
}

func isNodeOrDiskEvicted(node *longhorn.Node, disk longhorn.DiskSpec) bool {
	return node.Spec.EvictionRequested || disk.EvictionRequested
}

func getReplicaInstanceNames(diskType longhorn.DiskType, node *longhorn.Node, diskName, diskUUID, diskPath string, client *engineapi.DiskService) (map[string]string, error) {
	switch diskType {
	case longhorn.DiskTypeFilesystem:
		return getReplicaDirectoryNames(node, diskName, diskUUID, diskPath)
	case longhorn.DiskTypeBlock:
		return getSpdkReplicaInstanceNames(client, string(diskType), diskName)
	default:
		return nil, fmt.Errorf("unknown disk type %v", diskType)
	}
}

func getReplicaDirectoryNames(node *longhorn.Node, diskName, diskUUID, diskPath string) (map[string]string, error) {
	if !canCollectDiskData(node, diskName, diskUUID, diskPath) {
		return map[string]string{}, nil
	}

	possibleReplicaDirectoryNames, err := util.GetPossibleReplicaDirectoryNames(diskPath)
	if err != nil {
		logrus.Errorf("unable to get possible replica directories in disk %v on node %v since %v", diskPath, node.Name, err.Error())
		return map[string]string{}, nil
	}

	return possibleReplicaDirectoryNames, nil
}

func canCollectDiskData(node *longhorn.Node, diskName, diskUUID, diskPath string) bool {
	return !node.Spec.EvictionRequested &&
		!node.Spec.Disks[diskName].EvictionRequested &&
		node.Spec.Disks[diskName].Path == diskPath &&
		node.Status.DiskStatus != nil &&
		node.Status.DiskStatus[diskName] != nil &&
		node.Status.DiskStatus[diskName].DiskUUID == diskUUID &&
		types.GetCondition(node.Status.DiskStatus[diskName].Conditions, longhorn.DiskConditionTypeReady).Status == longhorn.ConditionStatusTrue
}

func NewDiskInfo(path, diskUUID string, nodeOrDiskEvicted bool, stat *util.DiskStat, orphanedReplicaDirectoryNames map[string]string, errorReason, errorMessage string) *CollectedDiskInfo {
	diskInfo := &CollectedDiskInfo{
		Path:                          path,
		NodeOrDiskEvicted:             nodeOrDiskEvicted,
		DiskUUID:                      diskUUID,
		DiskStat:                      stat,
		OrphanedReplicaDirectoryNames: orphanedReplicaDirectoryNames,
	}

	if errorMessage != "" {
		diskInfo.Condition = &longhorn.Condition{
			Type:    longhorn.DiskConditionTypeError,
			Status:  longhorn.ConditionStatusFalse,
			Reason:  errorReason,
			Message: errorMessage,
		}
	}

	return diskInfo
}

func (m *NodeMonitor) getOrphanedReplicaInstanceNames(diskType longhorn.DiskType, node *longhorn.Node, diskName, diskUUID, diskPath string, replicaDirectoryNames map[string]string) (map[string]string, error) {
	switch diskType {
	case longhorn.DiskTypeFilesystem:
		return m.getOrphanedReplicaDirectoryNames(node, diskName, diskUUID, diskPath, replicaDirectoryNames)
	case longhorn.DiskTypeBlock:
		return m.getOrphanedReplicaLvolNames(diskName, replicaDirectoryNames)
	default:
		return nil, fmt.Errorf("unknown disk type %v", diskType)
	}
}

func (m *NodeMonitor) getOrphanedReplicaLvolNames(diskName string, replicaDirectoryNames map[string]string) (map[string]string, error) {
	if len(replicaDirectoryNames) == 0 {
		return map[string]string{}, nil
	}

	for name := range replicaDirectoryNames {
		_, err := m.ds.GetReplica(name)
		if err == nil || (err != nil && !datastore.ErrorIsNotFound(err)) {
			delete(replicaDirectoryNames, name)
		}
	}

	return replicaDirectoryNames, nil
}

func (m *NodeMonitor) getOrphanedReplicaDirectoryNames(node *longhorn.Node, diskName, diskUUID, diskPath string, replicaDirectoryNames map[string]string) (map[string]string, error) {
	if len(replicaDirectoryNames) == 0 {
		return map[string]string{}, nil
	}

	// Find out the orphaned directories by checking with replica CRs
	replicas, err := m.ds.ListReplicasByDiskUUID(diskUUID)
	if err != nil {
		logrus.Errorf("unable to list replicas for disk UUID %v since %v", diskUUID, err.Error())
		return map[string]string{}, nil
	}

	for _, replica := range replicas {
		if replica.Spec.DiskPath == diskPath {
			delete(replicaDirectoryNames, replica.Spec.DataDirectoryName)
		}
	}

	if m.checkVolumeMeta {
		for name := range replicaDirectoryNames {
			if err := isVolumeMetaFileExist(diskPath, name); err != nil {
				delete(replicaDirectoryNames, name)
			}
		}
	}

	return replicaDirectoryNames, nil
}

func isVolumeMetaFileExist(diskPath, replicaDirectoryName string) error {
	path := filepath.Join(diskPath, "replicas", replicaDirectoryName, volumeMetaData)
	_, err := util.GetVolumeMeta(path)
	return err
}

func GetDiskNamesFromDiskMap(diskInfoMap map[string]*CollectedDiskInfo) []string {
	disks := []string{}
	for diskName := range diskInfoMap {
		disks = append(disks, diskName)
	}
	return disks
}
