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
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"
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

	getDiskStatHandler               GetDiskStatHandler
	getDiskConfig                    GetDiskConfig
	generateDiskConfig               GenerateDiskConfig
	getPossibleReplicaDirectoryNames GetPossibleReplicaDirectoryNames
}

type CollectedDiskInfo struct {
	Path                          string
	NodeOrDiskEvicted             bool
	DiskStat                      *util.DiskStat
	DiskUUID                      string
	Condition                     *longhorn.Condition
	OrphanedReplicaDirectoryNames map[string]string
}

type GetDiskStatHandler func(string) (*util.DiskStat, error)
type GetDiskConfig func(string) (*util.DiskConfig, error)
type GenerateDiskConfig func(string) (*util.DiskConfig, error)
type GetPossibleReplicaDirectoryNames func(*longhorn.Node, string, string, string) map[string]string

func NewNodeMonitor(logger logrus.FieldLogger, ds *datastore.DataStore, nodeName string, syncCallback func(key string)) (*NodeMonitor, error) {
	ctx, quit := context.WithCancel(context.Background())

	m := &NodeMonitor{
		baseMonitor: newBaseMonitor(ctx, quit, logger, ds, NodeMonitorSyncPeriod),

		nodeName:        nodeName,
		checkVolumeMeta: true,

		collectedDataLock: sync.RWMutex{},
		collectedData:     make(map[string]*CollectedDiskInfo, 0),

		syncCallback: syncCallback,

		getDiskStatHandler:               util.GetDiskStat,
		getDiskConfig:                    util.GetDiskConfig,
		generateDiskConfig:               util.GenerateDiskConfig,
		getPossibleReplicaDirectoryNames: getPossibleReplicaDirectoryNames,
	}

	go m.Start()

	return m, nil
}

func (m *NodeMonitor) Start() {
	wait.PollImmediateUntil(m.syncPeriod, func() (done bool, err error) {
		if err := m.SyncCollectedData(); err != nil {
			m.logger.Errorf("Stop monitoring because of %v", err)
		}
		return false, nil
	}, m.ctx.Done())
}

func (m *NodeMonitor) Close() {
	m.quit()
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

func (m *NodeMonitor) SyncCollectedData() error {
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

// Collect disk data and generate disk UUID blindly.
func (m *NodeMonitor) collectDiskData(node *longhorn.Node) map[string]*CollectedDiskInfo {
	diskInfoMap := make(map[string]*CollectedDiskInfo, 0)
	orphanedReplicaDirectoryNames := map[string]string{}

	for diskName, disk := range node.Spec.Disks {
		nodeOrDiskEvicted := isNodeOrDiskEvicted(node, disk)

		stat, err := m.getDiskStatHandler(disk.Path)
		if err != nil {
			diskInfoMap[diskName] = NewDiskInfo(disk.Path, "", nodeOrDiskEvicted, nil,
				orphanedReplicaDirectoryNames, string(longhorn.DiskConditionReasonNoDiskInfo),
				fmt.Sprintf("Disk %v(%v) on node %v is not ready: Get disk information error: %v",
					diskName, node.Spec.Disks[diskName].Path, node.Name, err))
			continue
		}

		diskConfig, err := m.getDiskConfig(disk.Path)
		if err != nil {
			if !types.ErrorIsNotFound(err) {
				diskInfoMap[diskName] = NewDiskInfo(disk.Path, "", nodeOrDiskEvicted, nil,
					orphanedReplicaDirectoryNames, string(longhorn.DiskConditionReasonNoDiskInfo),
					fmt.Sprintf("Disk %v(%v) on node %v is not ready: failed to get disk config: error: %v",
						diskName, disk.Path, node.Name, err))
				continue
			}
			// Blindly check or generate disk config.
			// The handling of all disks containing the same fsid will be done in NodeController.
			if diskConfig, err = m.generateDiskConfig(node.Spec.Disks[diskName].Path); err != nil {
				diskInfoMap[diskName] = NewDiskInfo(disk.Path, "", nodeOrDiskEvicted, nil,
					orphanedReplicaDirectoryNames, string(longhorn.DiskConditionReasonNoDiskInfo),
					fmt.Sprintf("Disk %v(%v) on node %v is not ready: failed to generate disk config: error: %v",
						diskName, disk.Path, node.Name, err))
				continue
			}
		}

		replicaDirectoryNames := m.getPossibleReplicaDirectoryNames(node, diskName, diskConfig.DiskUUID, disk.Path)
		orphanedReplicaDirectoryNames := m.getOrphanedReplicaDirectoryNames(node, diskName, diskConfig.DiskUUID, disk.Path, replicaDirectoryNames)

		diskInfoMap[diskName] = NewDiskInfo(disk.Path, diskConfig.DiskUUID, nodeOrDiskEvicted, stat,
			orphanedReplicaDirectoryNames, string(longhorn.DiskConditionReasonNoDiskInfo), "")
	}

	return diskInfoMap
}

func isNodeOrDiskEvicted(node *longhorn.Node, disk longhorn.DiskSpec) bool {
	return node.Spec.EvictionRequested || disk.EvictionRequested
}

func getPossibleReplicaDirectoryNames(node *longhorn.Node, diskName, diskUUID, diskPath string) map[string]string {
	if !canCollectDiskData(node, diskName, diskUUID, diskPath) {
		return map[string]string{}
	}

	possibleReplicaDirectoryNames, err := util.GetPossibleReplicaDirectoryNames(diskPath)
	if err != nil {
		logrus.Errorf("unable to get possible replica directories in disk %v on node %v since %v", diskPath, node.Name, err.Error())
		return map[string]string{}
	}

	return possibleReplicaDirectoryNames
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

func (m *NodeMonitor) getOrphanedReplicaDirectoryNames(node *longhorn.Node, diskName, diskUUID, diskPath string, replicaDirectoryNames map[string]string) map[string]string {
	if len(replicaDirectoryNames) == 0 {
		return map[string]string{}
	}

	// Find out the orphaned directories by checking with replica CRs
	replicas, err := m.ds.ListReplicasByDiskUUID(diskUUID)
	if err != nil {
		logrus.Errorf("unable to list replicas for disk UUID %v since %v", diskUUID, err.Error())
		return map[string]string{}
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

	return replicaDirectoryNames
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
