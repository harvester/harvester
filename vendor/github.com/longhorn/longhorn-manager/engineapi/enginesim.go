package engineapi

import (
	"fmt"
	"sync"

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
		if err := s.ReplicaAdd(addr, false); err != nil {
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

func (e *EngineSimulator) ReplicaList() (map[string]*Replica, error) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	ret := map[string]*Replica{}
	for _, replica := range e.replicas {
		rep := *replica
		ret[replica.URL] = &rep
	}
	return ret, nil
}

func (e *EngineSimulator) ReplicaAdd(url string, isRestoreVolume bool) error {
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

func (e *EngineSimulator) ReplicaRemove(addr string) error {
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

func (e *EngineSimulator) SnapshotCreate(name string, labels map[string]string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (e *EngineSimulator) SnapshotList() (map[string]*longhorn.Snapshot, error) {
	return nil, fmt.Errorf("not implemented")
}

func (e *EngineSimulator) SnapshotGet(name string) (*longhorn.Snapshot, error) {
	return nil, fmt.Errorf("not implemented")
}

func (e *EngineSimulator) SnapshotDelete(name string) error {
	return fmt.Errorf("not implemented")
}

func (e *EngineSimulator) SnapshotRevert(name string) error {
	return fmt.Errorf("not implemented")
}

func (e *EngineSimulator) SnapshotPurge() error {
	return fmt.Errorf("not implemented")
}

func (e *EngineSimulator) SnapshotPurgeStatus() (map[string]*longhorn.PurgeStatus, error) {
	return nil, fmt.Errorf("not implemented")
}

func (e *EngineSimulator) SnapshotBackup(backupName, snapName, backupTarget, backingImageName, backingImageChecksum string, labels, credential map[string]string) (string, string, error) {
	return "", "", fmt.Errorf("not implemented")
}

func (e *EngineSimulator) SnapshotBackupStatus(backupName, replicaAddress string) (*longhorn.EngineBackupStatus, error) {
	return nil, fmt.Errorf("not implemented")
}

func (e *EngineSimulator) Version(clientOnly bool) (*EngineVersion, error) {
	return nil, fmt.Errorf("not implemented")
}

func (e *EngineSimulator) Info() (*Volume, error) {
	return nil, fmt.Errorf("not implemented")
}

func (e *EngineSimulator) Expand(size int64) error {
	return fmt.Errorf("not implemented")
}

func (e *EngineSimulator) BackupRestore(backupTarget, backupName, backupVolume, lastRestored string, credential map[string]string) error {
	return fmt.Errorf("not implemented")
}

func (e *EngineSimulator) SnapshotClone(snapshotName, fromControllerAddress string) error {
	return fmt.Errorf("not implemented")
}

func (e *EngineSimulator) BackupRestoreStatus() (map[string]*longhorn.RestoreStatus, error) {
	return nil, fmt.Errorf("not implemented")
}

func (e *EngineSimulator) SnapshotCloneStatus() (map[string]*longhorn.SnapshotCloneStatus, error) {
	return nil, fmt.Errorf("not implemented")
}

func (e *EngineSimulator) ReplicaRebuildStatus() (map[string]*longhorn.RebuildStatus, error) {
	return nil, fmt.Errorf("not implemented")
}

func (e *EngineSimulator) FrontendStart(volumeFrontend longhorn.VolumeFrontend) error {
	return fmt.Errorf("not implemented")
}
func (e *EngineSimulator) FrontendShutdown() error {
	return fmt.Errorf("not implemented")
}

func (e *EngineSimulator) ReplicaRebuildVerify(url string) error {
	return fmt.Errorf("not implemented")
}
