// +build test

package envtest

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"path"
	"strings"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/rancher/harvester/test/framework/envtest/cluster"
)

const (
	// Specify to test the cases on an existing cluster,
	// or create a local cluster for testing if blank.
	envUseExistingCluster = "USE_EXISTING_CLUSTER"

	// Specify to keep the testing resources to review,
	// default is to drop it.
	envKeepTestingResource = "KEEP_TESTING_RESOURCE"
)

// Start starts a test environment and redirects Stdout/Stderr to output if "USE_EXISTING_CLUSTER" is not "true".
func Start(output io.Writer) (clientcmd.ClientConfig, error) {
	if !IsUsingExistingCluster() {
		var localCluster = cluster.GetLocalCluster()
		var err = localCluster.Startup(output)
		if err != nil {
			return nil, fmt.Errorf("failed to startup the local cluster %s, %v", localCluster, err)
		}
	}
	return GetConfig()
}

// Start stops a test environment and redirects Stdout/Stderr to output if "USE_EXISTING_CLUSTER" is not "true".
func Stop(output io.Writer) error {
	if IsUsingExistingCluster() || IsKeepingTestingResource() {
		return nil
	}
	var localCluster = cluster.GetLocalCluster()
	var err = localCluster.Cleanup(output)
	if err != nil {
		return fmt.Errorf("failed to cleanup the local '%s' cluster, %v", localCluster.GetKind(), err)
	}
	return nil
}

// GetConfig creates a *rest.Config for talking to a Kubernetes API server, the config precedence is as below:
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
		loadingRules.Precedence = append(loadingRules.Precedence, path.Join(u.HomeDir, clientcmd.RecommendedHomeDir, clientcmd.RecommendedFileName))
	}

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{}), nil
}

// IsUsingExistingCluster validates whether use an existing cluster for testing,
// if "USE_EXISTING_CLUSTER=true", the envtest will not create a local cluster for testing.
func IsUsingExistingCluster() bool {
	return strings.EqualFold(os.Getenv(envUseExistingCluster), "true")
}

// IsKeepingTestingResource validates whether keep the testing resource,
// if "KEEP_TESTING_RESOURCE=true", the envtest will keep the testing resource for reviewing.
func IsKeepingTestingResource() bool {
	return strings.EqualFold(os.Getenv(envKeepTestingResource), "true")
}
