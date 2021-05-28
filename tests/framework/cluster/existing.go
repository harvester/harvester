package cluster

import (
	"io"

	"github.com/harvester/harvester/tests/framework/logs"
)

var _ Cluster = &ExistingCluster{}

// ExistingCluster is a cluster which provided by kubeConfig, so It's kind is unknown.
type ExistingCluster struct {
}

func (e *ExistingCluster) String() string {
	return "this is an existing cluster"
}

func (e *ExistingCluster) GetKind() string {
	return ExistingClusterKind
}

func (e *ExistingCluster) Startup(output io.Writer) error {
	var logger = logs.NewLogger(output, 0)
	logger.V(0).Info("skip exist cluster")
	return nil
}

func (e *ExistingCluster) LoadImages(output io.Writer) error {
	var logger = logs.NewLogger(output, 0)
	logger.V(0).Info("skip loading images")
	return nil
}

func (e *ExistingCluster) Cleanup(output io.Writer) error {
	var logger = logs.NewLogger(output, 0)
	logger.V(0).Info("skip exist cluster")
	return nil
}
