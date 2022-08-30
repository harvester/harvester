package runtime

import (
	"fmt"
	"os"

	"k8s.io/client-go/rest"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/tests/framework/cluster"
	"github.com/harvester/harvester/tests/framework/fuzz"
)

const (
	testChartDir            = "../../../deploy/charts/harvester"
	testCRDChartDir         = "../../../deploy/charts/harvester-crd"
	testHarvesterNamespace  = "harvester-system"
	testLonghornNamespace   = "longhorn-system"
	testCattleNamespace     = "cattle-system"
	testChartReleaseName    = "harvester"
	testCRDChartReleaseName = "harvester-crd"
)

var (
	testDeploymentManifest = []string{
		"virt-operator",
		"virt-api",
		"virt-controller",
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

// SetConfig configures the public variables exported in github.com/harvester/harvester/pkg/config package.
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
	options.Namespace = testHarvesterNamespace
	options.RancherEmbedded = false

	// inject the preset envs, this is used for testing setting.
	err = os.Setenv(settings.GetEnvKey(settings.APIUIVersion.Name), settings.APIUIVersion.Default)
	if err != nil {
		return options, fmt.Errorf("failed to preset ENVs of harvester server, %w", err)
	}
	return options, nil
}
