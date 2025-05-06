package env

import (
	"os"
	"strings"
)

const (
	// Specify to test the cases on an existing cluster,
	// or create a local cluster for testing if blank.
	envUseExistingCluster = "USE_EXISTING_CLUSTER"

	// Specify to keep the testing cluster to review,
	// default is to drop it.
	envKeepTestingCluster = "KEEP_TESTING_CLUSTER"

	// Specify whether to skip Harvester installation
	// default is to drop it.
	envSkipHarvesterInstallation = "SKIP_HARVESTER_INSTALLATION"

	// Specify to keep the testing runtime to review,
	// default is to drop it.
	envKeepHarvesterInstallation = "KEEP_HARVESTER_INSTALLATION"

	// Specify to keep the testing resource to review,
	// default is to drop it.
	envKeepTestingResource = "KEEP_TESTING_RESOURCE"

	// Specify to don't use soft emulation
	// default is to drop it.
	envDontUseEmulation = "DONT_USE_EMULATION"

	// Specify whether to run e2e tests, an existing cluster is required
	// default to false.
	envEnableE2ETests = "ENABLE_E2E_TESTS"

	// Specify images to be pre-loaded into nodes. Use comma "," to specify multiple
	// images. e.g., "repo1/image1:tag1,repo2/image2:tag2"
	envPreloadingImages = "PRELOADING_IMAGES"

	// Specify Webhook image name. Use the value in the helm chart if not specified.
	envWebhookImage = "WEBHOOK_IMAGE_NAME"

	// In summary, default is:
	// 1. Create a new local cluster
	// 2. Deploy runtime
	// 3. KubeVirt will use soft emulation mode
	// 4. Clean everything after all test finish

	// If you use an existing harvester runtime to test , you can configure as follows:
	// export USE_EXISTING_CLUSTER=true
	// export SKIP_HARVESTER_INSTALLATION=true
	// export DONT_USE_EMULATION=true

	// DefaultStorageClass in integration test is "standard", comes from `rancher.io/local-path`
	DefaultStorageClassName = "standard"

	AnnoVMImageStorageClass = "harvesterhci.io/storageClassName"
)

// IsTrue validates that the specified environment variable is true.
func IsTrue(key string) bool {
	return strings.EqualFold(os.Getenv(key), "true")
}

// IsUsingExistingCluster validates whether use an existing cluster for testing,
// if "USE_EXISTING_CLUSTER=true", will not create a local cluster for testing.
func IsUsingExistingCluster() bool {
	return IsTrue(envUseExistingCluster)
}

// IsKeepingTestingCluster validates whether keep the testing cluster,
// if "KEEP_TESTING_CLUSTER=true", will keep the testing cluster for reviewing.
func IsKeepingTestingCluster() bool {
	return IsTrue(envKeepTestingCluster)
}

// IsSkipHarvesterInstallation validates whether to skip harvester installation for testing,
// if "SKIP_HARVESTER_INSTALLATION=true", will not create or update the harvester chart for testing.
func IsSkipHarvesterInstallation() bool {
	return IsTrue(envSkipHarvesterInstallation)
}

// IsKeepingHarvesterInstallation validates whether keep the harvester installation,
// if "KEEP_HARVESTER_INSTALLATION=true", will keep the harvester installation for reviewing.
func IsKeepingHarvesterInstallation() bool {
	return IsTrue(envKeepHarvesterInstallation)
}

// IsKeepingTestingResource validates whether keep the testing resource,
// if "KEEP_TESTING_RESOURCE=true", will keep the testing resource for reviewing.
func IsKeepingTestingResource() bool {
	return IsTrue(envKeepTestingResource)
}

// IsUsingEmulation validates whether use qemu soft emulation,
// if "DONT_USE_EMULATION=true", will not use qemu soft emulation.
func IsUsingEmulation() bool {
	return !IsTrue(envDontUseEmulation)
}

// IsE2ETestsEnabled validates whether to enable the e2e tests
func IsE2ETestsEnabled() bool {
	return IsTrue(envEnableE2ETests)
}

// GetPreloadingImages returns preloading image names
func GetPreloadingImages() []string {
	images := []string{}
	for _, image := range strings.Split(os.Getenv(envPreloadingImages), ",") {
		images = append(images, strings.TrimSpace(image))
	}
	return images
}

// GetWebhookImage returns webhook image name and tag
func GetWebhookImage() (string, string) {
	image := os.Getenv(envWebhookImage)
	if image == "" {
		return "", ""
	}

	tokens := strings.Split(image, ":")
	if len(tokens) > 1 {
		return tokens[0], tokens[1]
	}
	return tokens[0], ""
}
