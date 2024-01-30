package engineapi

import (
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func (p *Proxy) SnapshotCreate(e *longhorn.Engine, name string, labels map[string]string) (string, error) {
	return p.grpcClient.VolumeSnapshot(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName, p.DirectToURL(e),
		name, labels)
}

func (p *Proxy) SnapshotList(e *longhorn.Engine) (snapshots map[string]*longhorn.SnapshotInfo, err error) {
	recv, err := p.grpcClient.SnapshotList(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName,
		p.DirectToURL(e))
	if err != nil {
		return nil, err
	}

	snapshots = map[string]*longhorn.SnapshotInfo{}
	for k, v := range recv {
		snapshots[k] = (*longhorn.SnapshotInfo)(v)
	}
	return snapshots, nil
}

func (p *Proxy) SnapshotGet(e *longhorn.Engine, name string) (snapshot *longhorn.SnapshotInfo, err error) {
	recv, err := p.SnapshotList(e)
	if err != nil {
		return nil, err
	}

	return recv[name], nil
}

func (p *Proxy) SnapshotClone(e *longhorn.Engine, snapshotName, fromEngineAddress, fromVolumeName, fromEngineName string,
	fileSyncHTTPClientTimeout int64) (err error) {
	return p.grpcClient.SnapshotClone(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName, p.DirectToURL(e),
		snapshotName, fromEngineAddress, fromVolumeName, fromEngineName, int(fileSyncHTTPClientTimeout))
}

func (p *Proxy) SnapshotCloneStatus(e *longhorn.Engine) (status map[string]*longhorn.SnapshotCloneStatus, err error) {
	recv, err := p.grpcClient.SnapshotCloneStatus(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName,
		p.DirectToURL(e))
	if err != nil {
		return nil, err
	}

	status = map[string]*longhorn.SnapshotCloneStatus{}
	for k, v := range recv {
		status[k] = (*longhorn.SnapshotCloneStatus)(v)
	}
	return status, nil
}

func (p *Proxy) SnapshotRevert(e *longhorn.Engine, snapshotName string) (err error) {
	return p.grpcClient.SnapshotRevert(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName, p.DirectToURL(e),
		snapshotName)
}

func (p *Proxy) SnapshotPurge(e *longhorn.Engine) (err error) {
	return p.grpcClient.SnapshotPurge(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName, p.DirectToURL(e),
		true)
}

func (p *Proxy) SnapshotPurgeStatus(e *longhorn.Engine) (status map[string]*longhorn.PurgeStatus, err error) {
	recv, err := p.grpcClient.SnapshotPurgeStatus(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName,
		p.DirectToURL(e))
	if err != nil {
		return nil, err
	}

	status = map[string]*longhorn.PurgeStatus{}
	for k, v := range recv {
		status[k] = (*longhorn.PurgeStatus)(v)
	}
	return status, nil
}

func (p *Proxy) SnapshotDelete(e *longhorn.Engine, name string) (err error) {
	return p.grpcClient.SnapshotRemove(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName, p.DirectToURL(e),
		[]string{name})
}

func (p *Proxy) SnapshotHash(e *longhorn.Engine, snapshotName string, rehash bool) error {
	return p.grpcClient.SnapshotHash(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName, p.DirectToURL(e),
		snapshotName, rehash)
}

func (p *Proxy) SnapshotHashStatus(e *longhorn.Engine, snapshotName string) (status map[string]*longhorn.HashStatus, err error) {
	recv, err := p.grpcClient.SnapshotHashStatus(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName,
		p.DirectToURL(e), snapshotName)
	if err != nil {
		return nil, err
	}

	status = map[string]*longhorn.HashStatus{}
	for k, v := range recv {
		status[k] = (*longhorn.HashStatus)(v)
	}
	return status, nil
}
