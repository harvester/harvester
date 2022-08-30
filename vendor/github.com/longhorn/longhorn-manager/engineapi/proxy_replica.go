package engineapi

import (
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func (p *Proxy) ReplicaAdd(e *longhorn.Engine, address string, restore bool) (err error) {
	return p.grpcClient.ReplicaAdd(p.DirectToURL(e), address, restore)
}

func (p *Proxy) ReplicaRemove(e *longhorn.Engine, address string) (err error) {
	return p.grpcClient.ReplicaRemove(p.DirectToURL(e), address)
}

func (p *Proxy) ReplicaList(e *longhorn.Engine) (replicas map[string]*Replica, err error) {
	resp, err := p.grpcClient.ReplicaList(p.DirectToURL(e))
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
	recv, err := p.grpcClient.ReplicaRebuildingStatus(p.DirectToURL(e))
	if err != nil {
		return nil, err
	}

	status = make(map[string]*longhorn.RebuildStatus)
	for k, v := range recv {
		status[k] = (*longhorn.RebuildStatus)(v)
	}
	return status, nil
}

func (p *Proxy) ReplicaRebuildVerify(e *longhorn.Engine, url string) (err error) {
	if err := ValidateReplicaURL(url); err != nil {
		return err
	}

	return p.grpcClient.ReplicaVerifyRebuild(p.DirectToURL(e), url)
}
