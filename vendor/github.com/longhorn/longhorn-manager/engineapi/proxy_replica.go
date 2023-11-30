package engineapi

import (
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func (p *Proxy) ReplicaAdd(e *longhorn.Engine, replicaName, replicaAddress string, restore, fastSync bool, replicaFileSyncHTTPClientTimeout int64) (err error) {
	return p.grpcClient.ReplicaAdd(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName, p.DirectToURL(e),
		replicaName, replicaAddress, restore, e.Spec.VolumeSize, e.Status.CurrentSize,
		int(replicaFileSyncHTTPClientTimeout), fastSync)
}

func (p *Proxy) ReplicaRemove(e *longhorn.Engine, address string) (err error) {
	return p.grpcClient.ReplicaRemove(string(e.Spec.BackendStoreDriver), p.DirectToURL(e), e.Name, address, "")
}

func (p *Proxy) ReplicaList(e *longhorn.Engine) (replicas map[string]*Replica, err error) {
	resp, err := p.grpcClient.ReplicaList(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName,
		p.DirectToURL(e))
	if err != nil {
		return nil, err
	}

	replicas = make(map[string]*Replica)
	for _, r := range resp {
		mode := longhorn.ReplicaMode(r.Mode)
		if mode != longhorn.ReplicaModeRW && mode != longhorn.ReplicaModeWO {
			mode = longhorn.ReplicaModeERR
		}
		replicas[r.Address] = &Replica{
			URL:  r.Address,
			Mode: mode,
		}
	}
	return replicas, nil
}

func (p *Proxy) ReplicaRebuildStatus(e *longhorn.Engine) (status map[string]*longhorn.RebuildStatus, err error) {
	recv, err := p.grpcClient.ReplicaRebuildingStatus(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName,
		p.DirectToURL(e))
	if err != nil {
		return nil, err
	}

	status = make(map[string]*longhorn.RebuildStatus)
	for k, v := range recv {
		status[k] = (*longhorn.RebuildStatus)(v)
	}
	return status, nil
}

func (p *Proxy) ReplicaRebuildVerify(e *longhorn.Engine, replicaName, url string) (err error) {
	if err := ValidateReplicaURL(url); err != nil {
		return err
	}
	return p.grpcClient.ReplicaVerifyRebuild(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName,
		p.DirectToURL(e), url, replicaName)
}

func (p *Proxy) ReplicaModeUpdate(e *longhorn.Engine, url, mode string) (err error) {
	if err := ValidateReplicaURL(url); err != nil {
		return err
	}

	return p.grpcClient.ReplicaModeUpdate(string(e.Spec.BackendStoreDriver), p.DirectToURL(e), url, mode)
}
