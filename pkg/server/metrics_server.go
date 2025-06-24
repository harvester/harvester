package server

import (
	"context"
	"net/http"

	"github.com/sirupsen/logrus"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/harvester/harvester/pkg/config"
)

var (
	metricsServer *MetricsServer
)

const (
	metricsPath = "/metrics"
)

type MetricsServer struct {
	context context.Context
	reg     *prometheus.Registry
	handler http.Handler
}

func (ms *MetricsServer) MustRegister(cs ...prometheus.Collector) {
	for _, c := range cs {
		ms.reg.MustRegister(c)
	}
}

func NewMetricsServer(ctx context.Context) (*MetricsServer, error) {
	if metricsServer != nil {
		return metricsServer, nil
	}

	logrus.Infof("New metrics server and handle %v", metricsPath)
	server := &MetricsServer{
		context: ctx,
	}
	server.reg = config.ScaledWithContext(ctx).GetMetricsRegistry()
	server.handler = promhttp.HandlerFor(server.reg, promhttp.HandlerOpts{Registry: server.reg})
	//http.Handle("/metrics", promhttp.HandlerFor(server.reg, promhttp.HandlerOpts{Registry: server.reg}))
	http.Handle(metricsPath, server.handler)
	metricsServer = server

	return metricsServer, nil
}

func (s *MetricsServer) ListenAndServe() error {
	logrus.Infof("Starting metrics server on 9500")
	if err := http.ListenAndServe(":9500", nil); err != nil {
		return err
	}
	<-s.context.Done()
	return s.context.Err()
}
