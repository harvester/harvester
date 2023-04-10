package client

import (
	"github.com/pkg/errors"

	rpc "github.com/longhorn/longhorn-instance-manager/pkg/imrpc"
)

func (c *ProxyClient) MetricsGet(serviceAddress string) (metrics *Metrics, err error) {
	input := map[string]string{
		"serviceAddress": serviceAddress,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get metrics for volume")
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to get metrics for volume", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.ProxyEngineRequest{
		Address: serviceAddress,
	}
	resp, err := c.service.MetricsGet(getContextWithGRPCTimeout(c.ctx), req)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		ReadThroughput:  resp.Metrics.ReadThroughput,
		WriteThroughput: resp.Metrics.WriteThroughput,
		ReadIOPS:        resp.Metrics.ReadIOPS,
		WriteIOPS:       resp.Metrics.WriteIOPS,
		ReadLatency:     resp.Metrics.ReadLatency,
		WriteLatency:    resp.Metrics.WriteLatency,
	}, nil
}
