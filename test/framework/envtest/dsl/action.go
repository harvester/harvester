// +build test

package dsl

import (
	"github.com/onsi/ginkgo"

	"github.com/rancher/harvester/test/framework/envtest"
)

// Cleanup executes the target cleanup execution if "KEEP_TESTING_RESOURCE" isn't "true".
func Cleanup(body interface{}, timeout ...float64) bool {
	if envtest.IsKeepingTestingResource() {
		return true
	}

	return ginkgo.AfterEach(body, timeout...)
}
