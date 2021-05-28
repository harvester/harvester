package cluster

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"path"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/harvester/harvester/tests/framework/env"
)

const (
	KindClusterKind     = "kind"
	ExistingClusterKind = "unknown"
)

// Cluster defines the basic actions of a cluster handler.
type Cluster interface {
	fmt.Stringer

	// GetKind returns the kind of cluster.
	GetKind() string

	// Startup runs a cluster.
	Startup(output io.Writer) error

	// Load images into a cluster.
	LoadImages(output io.Writer) error

	// Cleanup closes the cluster.
	Cleanup(output io.Writer) error
}

// NewLocalCluster returns the local cluster from environment,
// the default runtime is based on kubernetes-sigs/kind.
func NewLocalCluster() Cluster {
	return NewLocalKindCluster()
}

// GetExistCluster returns the exist cluster from environment,
func GetExistCluster() Cluster {
	return &ExistingCluster{}
}

// Start starts a test environment and redirects Stdout/Stderr to output if "USE_EXISTING_CLUSTER" is not "true".
func Start(output io.Writer) (clientcmd.ClientConfig, Cluster, error) {
	var cluster Cluster
	if env.IsUsingExistingCluster() {
		cluster = GetExistCluster()
	} else {
		cluster = NewLocalCluster()
	}
	err := cluster.Startup(output)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to startup the local cluster %s, %v", cluster, err)
	}

	err = cluster.LoadImages(output)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load images into cluster %s, %v", cluster, err)
	}

	KubeClientConfig, err := GetConfig()
	return KubeClientConfig, cluster, err
}

// Stop stops a test environment and redirects Stdout/Stderr to output
// if "KEEP_TESTING_CLUSTER" is not "true" and "USE_EXISTING_CLUSTER" is not "true".
func Stop(output io.Writer) error {
	if env.IsUsingExistingCluster() || env.IsKeepingTestingCluster() {
		return nil
	}
	cluster := NewLocalCluster()
	err := cluster.Cleanup(output)
	if err != nil {
		return fmt.Errorf("failed to cleanup the local '%s' cluster, %v", cluster.GetKind(), err)
	}
	return nil
}

// GetConfig creates a clientcmd.ClientConfig for talking to a Kubernetes API server, the config precedence is as below:
// 1. KUBECONFIG environment variable pointing at files
// 2. $HOME/.kube/config if exists
// 3. In-cluster config if running in cluster
func GetConfig() (clientcmd.ClientConfig, error) {
	var loadingRules = clientcmd.NewDefaultClientConfigLoadingRules()
	if _, ok := os.LookupEnv("HOME"); !ok {
		var u, err = user.Current()
		if err != nil {
			return nil, fmt.Errorf("could not load config from current user as the current user is not found, %v", err)
		}
		loadingRules.Precedence = append(loadingRules.Precedence,
			path.Join(u.HomeDir, clientcmd.RecommendedHomeDir, clientcmd.RecommendedFileName))
	}

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{}), nil
}
