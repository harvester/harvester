package client

import (
	"fmt"

	"github.com/pkg/errors"

	rpc "github.com/longhorn/types/pkg/generated/imrpc"
)

func (c *ProxyClient) MetricsGet(dataEngine, engineName, volumeName, serviceAddress string) (metrics *Metrics, err error) {
	input := map[string]string{
		"engineName":     engineName,
		"volumeName":     volumeName,
		"serviceAddress": serviceAddress,
		"dataEngine":     dataEngine,
	}
	if err := validateProxyMethodParameters(input); err != nil {
		return nil, errors.Wrap(err, "failed to get metrics for volume")
	}

	driver, ok := rpc.DataEngine_value[getDataEngine(dataEngine)]
	if !ok {
		return nil, fmt.Errorf("failed to get metrics for volume: invalid data engine %v", dataEngine)
	}

	defer func() {
		err = errors.Wrapf(err, "%v failed to get metrics for volume", c.getProxyErrorPrefix(serviceAddress))
	}()

	req := &rpc.ProxyEngineRequest{
		Address:    serviceAddress,
		EngineName: engineName,
		VolumeName: volumeName,
		DataEngine: rpc.DataEngine(driver),
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
