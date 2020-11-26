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

	// Specify to use an existing runtime
	// default is to drop it.
	envUseExistingRuntime = "USE_EXISTING_RUNTIME"

	// Specify to keep the testing runtime to review,
	// default is to drop it.
	envKeepTestingRuntime = "KEEP_TESTING_RUNTIME"

	// Specify to keep the testing vm to review,
	// default is to drop it.
	envKeepTestingVM = "KEEP_TESTING_VM"

	// Specify to don't use soft emulation
	// default is to drop it.
	envDontUseEmulation = "DONT_USE_EMULATION"

	// In summary, default is:
	// 1. Create a new local cluster
	// 2. Deploy runtime
	// 3. KubeVirt will use soft emulation mode
	// 4. Clean everything after all test finish

	// If you use an existing harvester runtime to test , you can configure as follows:
	// export USE_EXISTING_CLUSTER=true
	// export USE_EXISTING_RUNTIME=true
	// export DONT_USE_EMULATION=true
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

// IsUsingExistingRuntime validates whether use an existing runtime for testing,
// if "USE_EXISTING_RUNTIME=true", will not create or update an runtime for testing.
func IsUsingExistingRuntime() bool {
	return IsTrue(envUseExistingRuntime)
}

// IsKeepingTestingRuntime validates whether keep the testing runtime,
// if "KEEP_TESTING_RUNTIME=true", will keep the testing runtime for reviewing.
func IsKeepingTestingRuntime() bool {
	return IsTrue(envKeepTestingRuntime)
}

// IsKeepingTestingVM validates whether keep the testing vm,
// if "KEEP_TESTING_VM=true", will keep the testing vm for reviewing.
func IsKeepingTestingVM() bool {
	return IsTrue(envKeepTestingVM)
}

// IsUsingEmulation validates whether use qemu soft emulation,
// if "DONT_USE_EMULATION=true", will not use qemu soft emulation.
func IsUsingEmulation() bool {
	return !IsTrue(envDontUseEmulation)
}
