package monitor

import (
	"context"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/longhorn/longhorn-manager/datastore"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/util"
)

const (
	TestDiskID1 = "fsid"

	TestOrphanedReplicaDirectoryName = "test-volume-r-000000000"
)

func NewFakeNodeMonitor(logger logrus.FieldLogger, ds *datastore.DataStore, nodeName string, syncCallback func(key string)) (*NodeMonitor, error) {
	ctx, quit := context.WithCancel(context.Background())

	m := &NodeMonitor{
		baseMonitor: newBaseMonitor(ctx, quit, logger, ds, NodeMonitorSyncPeriod),

		nodeName:        nodeName,
		checkVolumeMeta: false,

		collectedDataLock: sync.RWMutex{},
		collectedData:     make(map[string]*CollectedDiskInfo, 0),

		syncCallback: syncCallback,

		getDiskStatHandler:               fakeGetDiskStat,
		getDiskConfig:                    fakeGetDiskConfig,
		generateDiskConfig:               fakeGenerateDiskConfig,
		getPossibleReplicaDirectoryNames: fakeGetPossibleReplicaDirectoryNames,
	}

	return m, nil
}

func fakeGetPossibleReplicaDirectoryNames(node *longhorn.Node, diskName, diskUUID, diskPath string) map[string]string {
	return map[string]string{
		TestOrphanedReplicaDirectoryName: "",
	}
}

func fakeGetDiskStat(directory string) (*util.DiskStat, error) {
	return &util.DiskStat{
		Fsid:       "fsid",
		Path:       directory,
		Type:       "ext4",
		FreeBlock:  0,
		TotalBlock: 0,
		BlockSize:  0,

		StorageMaximum:   0,
		StorageAvailable: 0,
	}, nil
}

func fakeGetDiskConfig(path string) (*util.DiskConfig, error) {
	return &util.DiskConfig{
		DiskUUID: TestDiskID1,
	}, nil
}

func fakeGenerateDiskConfig(path string) (*util.DiskConfig, error) {
	return &util.DiskConfig{
		DiskUUID: TestDiskID1,
	}, nil
}
