package engineapi

import (
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func (p *Proxy) MetricsGet(e *longhorn.Engine) (*Metrics, error) {
	metrics, err := p.grpcClient.MetricsGet(string(e.Spec.DataEngine), e.Name, e.Spec.VolumeName, p.DirectToURL(e))
	if err != nil {
		return nil, err
	}
	return (*Metrics)(metrics), err
}
