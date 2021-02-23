package runtime

import (
	"fmt"
	"os"

	"k8s.io/client-go/rest"

	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/settings"
	"github.com/rancher/harvester/tests/framework/client"
	"github.com/rancher/harvester/tests/framework/cluster"
	"github.com/rancher/harvester/tests/framework/fuzz"
)

const (
	testChartDir               = "../../../deploy/charts/harvester"
	testHarvesterNamespace     = "harvester-system"
	testLonghornNamespace      = "longhorn-system"
	testChartReleaseName       = "harvester"
	testImageStorageAccessKey  = "YOURACCESSKEY"
	testImageStorageSecretKey  = "YOURSECRETKEY"
	testImageStorageDeployment = "minio"
	testImageStorageService    = "minio"
)

var (
	testDeploymentManifest = []string{
		"cdi-operator",
		"cdi-apiserver",
		"cdi-deployment",
		"cdi-uploadproxy",
		"virt-operator",
		"virt-api",
		"virt-controller",
		"minio",
	}
	testDaemonSetManifest = []string{
		"virt-handler",
	}
	longhornDeploymentManifest = []string{
		"csi-attacher",
		"csi-snapshotter",
		"csi-provisioner",
		"csi-resizer",
		"longhorn-driver-deployer",
	}
	longhornDaemonSetManifest = []string{
		"longhorn-manager",
		"engine-image-ei-2938e020",
		"longhorn-csi-plugin",
	}
)

// SetConfig configures the public variables exported in github.com/rancher/harvester/pkg/config package.
func SetConfig(kubeConfig *rest.Config, testCluster cluster.Cluster) (config.Options, error) {
	var options config.Options

	// generate two random ports
	ports, err := fuzz.FreePorts(2)
	if err != nil {
		return options, fmt.Errorf("failed to get listening ports of harvester server, %v", err)
	}

	// config http and https
	options.HTTPListenPort = ports[0]
	options.HTTPSListenPort = ports[1]

	// config skip auth
	options.SkipAuthentication = true

	// config imageStorage
	imageStorageEndpoint, err := client.GetNodePortEndPoint(kubeConfig,
		testHarvesterNamespace, testImageStorageDeployment, testImageStorageService)
	if err != nil {
		return options, fmt.Errorf("failed to get storage endpoint of %s, %v", testImageStorageService, err)
	}

	options.ImageStorageEndpoint = fmt.Sprintf("http://%s", imageStorageEndpoint)
	options.ImageStorageAccessKey = testImageStorageAccessKey
	options.ImageStorageSecretKey = testImageStorageSecretKey

	options.Namespace = testHarvesterNamespace

	// inject the preset envs, this is used for testing setting.
	err = os.Setenv(settings.GetEnvKey(settings.APIUIVersion.Name), settings.APIUIVersion.Default)
	if err != nil {
		return options, fmt.Errorf("failed to preset ENVs of harvester server, %w", err)
	}
	return options, nil
}
