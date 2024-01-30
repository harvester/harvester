package engineapi

import (
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/longhorn/longhorn-manager/datastore"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

type EngineSimulatorRequest struct {
	VolumeName     string
	VolumeSize     int64
	ControllerAddr string
	ReplicaAddrs   []string
}

type EngineSimulatorCollection struct {
	simulators map[string]*EngineSimulator
	mutex      *sync.Mutex
}

func NewEngineSimulatorCollection() *EngineSimulatorCollection {
	return &EngineSimulatorCollection{
		simulators: map[string]*EngineSimulator{},
		mutex:      &sync.Mutex{},
	}
}

func (c *EngineSimulatorCollection) CreateEngineSimulator(request *EngineSimulatorRequest) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.simulators[request.VolumeName] != nil {
		return fmt.Errorf("duplicate simulator with volume name %v already exists", request.VolumeName)
	}
	s := &EngineSimulator{
		volumeName:     request.VolumeName,
		volumeSize:     request.VolumeSize,
		controllerAddr: request.ControllerAddr,
		running:        true,
		replicas:       map[string]*Replica{},
		mutex:          &sync.RWMutex{},
	}
	for _, addr := range request.ReplicaAddrs {
		if err := s.ReplicaAdd(&longhorn.Engine{}, "", addr, false, false, 30); err != nil {
			return err
		}
	}
	c.simulators[s.volumeName] = s
	return nil
}

func (c *EngineSimulatorCollection) GetEngineSimulator(volumeName string) (*EngineSimulator, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.simulators[volumeName] == nil {
		return nil, fmt.Errorf("unable to find simulator with volume name %v", volumeName)
	}
	return c.simulators[volumeName], nil
}

func (c *EngineSimulatorCollection) DeleteEngineSimulator(volumeName string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.simulators[volumeName] == nil {
		return fmt.Errorf("unable to find simulator with volume name %v", volumeName)
	}
	// stop the references
	c.simulators[volumeName].running = false
	delete(c.simulators, volumeName)
	return nil
}

func (c *EngineSimulatorCollection) NewEngineClient(request *EngineClientRequest) (EngineClient, error) {
	engine, err := c.GetEngineSimulator(request.VolumeName)
	if err != nil {
		return nil, fmt.Errorf("cannot find existing engine simulator for client")
	}
	return engine, nil
}

type EngineSimulator struct {
	volumeName     string
	volumeSize     int64
	controllerAddr string
	running        bool
	replicas       map[string]*Replica
	mutex          *sync.RWMutex
}

func (e *EngineSimulator) Name() string {
	return e.volumeName
}

func (e *EngineSimulator) IsGRPC() bool {
	return false
}

func (e *EngineSimulator) Start(*longhorn.InstanceManager, logrus.FieldLogger, *datastore.DataStore) error {
	return fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) Stop(*longhorn.InstanceManager) error {
	return fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) ReplicaList(*longhorn.Engine) (map[string]*Replica, error) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	ret := map[string]*Replica{}
	for _, replica := range e.replicas {
		rep := *replica
		ret[replica.URL] = &rep
	}
	return ret, nil
}

func (e *EngineSimulator) ReplicaAdd(engine *longhorn.Engine, replicaName, url string, isRestoreVolume, fastSync bool, replicaFileSyncHTTPClientTimeout int64) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	for name, replica := range e.replicas {
		if replica.Mode == longhorn.ReplicaModeERR {
			return fmt.Errorf("replica %v is in ERR mode, cannot add new replica", name)
		}
	}
	if e.replicas[url] != nil {
		return fmt.Errorf("duplicate replica %v already exists", url)
	}
	e.replicas[url] = &Replica{
		URL:  url,
		Mode: longhorn.ReplicaModeRW,
	}
	return nil
}

func (e *EngineSimulator) ReplicaRemove(engine *longhorn.Engine, addr string) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if e.replicas[addr] == nil {
		return fmt.Errorf("unable to find replica %v", addr)
	}
	delete(e.replicas, addr)
	return nil
}

func (e *EngineSimulator) SimulateStopReplica(addr string) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if e.replicas[addr] == nil {
		return fmt.Errorf("unable to find replica %v", addr)
	}
	e.replicas[addr].Mode = longhorn.ReplicaModeERR
	return nil
}

func (e *EngineSimulator) SnapshotCreate(engine *longhorn.Engine, name string, labels map[string]string) (string, error) {
	return "", fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) SnapshotList(engine *longhorn.Engine) (map[string]*longhorn.SnapshotInfo, error) {
	return nil, fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) SnapshotGet(engine *longhorn.Engine, name string) (*longhorn.SnapshotInfo, error) {
	return nil, fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) SnapshotDelete(engine *longhorn.Engine, name string) error {
	return fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) SnapshotRevert(engine *longhorn.Engine, name string) error {
	return fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) SnapshotPurge(*longhorn.Engine) error {
	return fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) SnapshotPurgeStatus(*longhorn.Engine) (map[string]*longhorn.PurgeStatus, error) {
	return nil, fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) SnapshotBackup(engine *longhorn.Engine, backupName, snapName, backupTarget,
	backingImageName, backingImageChecksum, compressionMethod string, concurrentLimit int, storageClassName string,
	labels, credential map[string]string) (string, string, error) {
	return "", "", fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) SnapshotBackupStatus(engine *longhorn.Engine, backupName, replicaAddress,
	replicaName string) (*longhorn.EngineBackupStatus, error) {
	return nil, fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) VersionGet(engine *longhorn.Engine, clientOnly bool) (*EngineVersion, error) {
	return nil, fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) VolumeGet(*longhorn.Engine) (*Volume, error) {
	return nil, fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) VolumeExpand(*longhorn.Engine) error {
	return fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) BackupRestore(engine *longhorn.Engine, backupTarget, backupName, backupVolume, lastRestored string, credential map[string]string, concurrentLimit int) error {
	return fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) SnapshotClone(engine *longhorn.Engine, snapshotName, fromEngineAddress, fromVolumeName,
	fromEngineName string, fileSyncHTTPClientTimeout int64) error {
	return fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) BackupRestoreStatus(*longhorn.Engine) (map[string]*longhorn.RestoreStatus, error) {
	return nil, fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) SnapshotCloneStatus(*longhorn.Engine) (map[string]*longhorn.SnapshotCloneStatus, error) {
	return nil, fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) ReplicaRebuildStatus(*longhorn.Engine) (map[string]*longhorn.RebuildStatus, error) {
	return nil, fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) VolumeFrontendStart(*longhorn.Engine) error {
	return fmt.Errorf(ErrNotImplement)
}
func (e *EngineSimulator) VolumeFrontendShutdown(*longhorn.Engine) error {
	return fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) VolumeUnmapMarkSnapChainRemovedSet(*longhorn.Engine) error {
	return fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) ReplicaRebuildVerify(engine *longhorn.Engine, replicaName, url string) error {
	return fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) SnapshotHash(engine *longhorn.Engine, snapshotName string, rehash bool) error {
	return fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) SnapshotHashStatus(engine *longhorn.Engine, snapshotName string) (map[string]*longhorn.HashStatus, error) {
	return nil, fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) ReplicaModeUpdate(engine *longhorn.Engine, url, mode string) error {
	return fmt.Errorf(ErrNotImplement)
}

func (e *EngineSimulator) MetricsGet(*longhorn.Engine) (*Metrics, error) {
	return nil, fmt.Errorf(ErrNotImplement)
}
