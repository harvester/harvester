package dsl

import (
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/harvester/harvester/tests/framework/env"
)

const (
	commonTimeoutInterval = 10
	commonPollingInterval = 1

	vmTimeoutInterval  = 300
	vmPollingInterval  = 2
	pvcTimeoutInterval = 300
	pvcPollingInterval = 2
)

// Cleanup executes the target cleanup execution if "KEEP_TESTING_RESOURCE" isn't "true".
func Cleanup(body interface{}) bool {
	if env.IsKeepingTestingResource() {
		return true
	}

	return ginkgo.AfterEach(body)
}

func MustFinallyBeTrue(actual func() bool, intervals ...interface{}) bool {
	var timeoutInterval interface{} = commonTimeoutInterval
	var pollingInterval interface{} = commonPollingInterval
	if len(intervals) > 0 {
		timeoutInterval = intervals[0]
	}
	if len(intervals) > 1 {
		pollingInterval = intervals[1]
	}
	return gomega.Eventually(actual, timeoutInterval, pollingInterval).Should(gomega.BeTrue())
}

func MustNotError(err error) bool {
	return gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
}

func MustEqual(actual, expected interface{}) {
	gomega.Expect(actual).Should(gomega.Equal(expected))
}

func MustNotEqual(actual, unExpected interface{}) {
	gomega.Expect(actual).ShouldNot(gomega.Equal(unExpected))
}

func MustChanged(currentValue, newValue, oldValue interface{}) {
	MustEqual(currentValue, newValue)
	MustNotEqual(currentValue, oldValue)
}

func MustRespCodeIs(expectedRespCode int, action string, err error, respCode int, respBody []byte) {
	MustNotError(err)
	if respCode != expectedRespCode {
		ginkgo.GinkgoT().Errorf("failed to %s, response with %d, %v", action, respCode, string(respBody))
	}
}

func MustRespCodeIn(action string, err error, respCode int, respBody []byte, expectedRespCodes ...int) {
	MustNotError(err)
	gomega.Expect(respCode).To(gomega.BeElementOf(expectedRespCodes), fmt.Sprintf("failed to %s, response with %d, %v", action, respCode, string(respBody)))
}

func CheckRespCodeIs(expectedRespCode int, action string, err error, respCode int, respBody []byte) bool {
	if err != nil {
		ginkgo.GinkgoT().Logf("failed to %s, %v", action, err)
		return false
	}

	if respCode != expectedRespCode {
		ginkgo.GinkgoT().Logf("failed to %s, response with %d, %v", action, respCode, string(respBody))
		return false
	}
	return true
}
