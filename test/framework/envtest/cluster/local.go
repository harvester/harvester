// +build test

package cluster

import (
	"fmt"
	"io"
)

// LocalCluster defines the basic actions of a local cluster handler.
type LocalCluster interface {
	fmt.Stringer

	// GetKind returns the kind of local cluster.
	GetKind() string

	// Startup runs a local cluster.
	Startup(output io.Writer) error

	// Cleanup closes the local cluster.
	Cleanup(output io.Writer) error
}

// GetLocalCluster returns the local cluster from environment,
// the default runtime is based on kubernetes-sigs/kind.
func GetLocalCluster() LocalCluster {
	return DefaultLocalKindCluster()
}
