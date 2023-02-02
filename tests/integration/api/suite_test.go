package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/dynamiclistener"
	"github.com/sirupsen/logrus"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/server"
	"github.com/harvester/harvester/tests/framework/cluster"
	. "github.com/harvester/harvester/tests/framework/dsl"
	"github.com/harvester/harvester/tests/framework/helper"
	"github.com/harvester/harvester/tests/integration/runtime"
)

var (
	testSuiteStartErrChan chan error
	testCtx               context.Context
	testCtxCancel         context.CancelFunc
	harvester             *server.HarvesterServer

	kubeConfig       *restclient.Config
	KubeClientConfig clientcmd.ClientConfig
	testCluster      cluster.Cluster
	options          config.Options

	testResourceLabels = map[string]string{
		"harvester.test.io": "harvester-test",
	}
	testVMBackupLabels = map[string]string{
		"harvester.test.io/type": "vm-backup",
	}
)

const (
	harvesterStartTimeOut = 20
)

func TestAPI(t *testing.T) {
	defer GinkgoRecover()

	RegisterFailHandler(Fail)

	RunSpecs(t, "api suite")
}

var _ = BeforeSuite(func() {
	testCtx, testCtxCancel = context.WithCancel(context.Background())
	var err error

	By("starting test cluster")
	KubeClientConfig, testCluster, err = cluster.Start(GinkgoWriter)
	MustNotError(err)

	kubeConfig, err = KubeClientConfig.ClientConfig()
	MustNotError(err)

	By("construct harvester runtime")
	err = runtime.Construct(testCtx, kubeConfig)
	MustNotError(err)

	By("set harvester config")
	options, err = runtime.SetConfig(kubeConfig, testCluster)
	MustNotError(err)

	By("new harvester server")
	harvester, err = server.New(testCtx, KubeClientConfig, options)
	MustNotError(err)

	By("start harvester server")
	listenOpts := &dynamiclistener.Config{
		CloseConnOnCertChange: false,
	}
	testSuiteStartErrChan = make(chan error)
	go func() {
		testSuiteStartErrChan <- harvester.ListenAndServe(listenOpts, options)
	}()

	// NB(thxCode): since the start of all controllers is not synchronized,
	// it cannot guarantee the controllers has been start,
	// which means the cache(informer) has not ready,
	// so we give a stupid time sleep to trigger the first list-watch,
	// and please use the client interface instead of informer interface if you can.
	select {
	case <-time.After(harvesterStartTimeOut * time.Second):
		MustFinallyBeTrue(func() bool {
			return validateAPIIsReady()
		})
	case err := <-testSuiteStartErrChan:
		MustNotError(err)
	}
})

var _ = AfterSuite(func() {
	By("tearing down harvester runtime")
	err := runtime.Destruct(context.Background(), kubeConfig)
	MustNotError(err)

	By("tearing down test cluster")
	err = cluster.Stop(GinkgoWriter)
	MustNotError(err)

	By("tearing down harvester server")
	if testCtxCancel != nil {
		testCtxCancel()
	}

})

// validate the v1 api server is ready
func validateAPIIsReady() bool {
	apiURL := helper.BuildAPIURL("v1", "", options.HTTPSListenPort)
	code, _, err := helper.GetResponse(apiURL)
	if err != nil || code != http.StatusOK {
		logrus.Errorf("failed to get %s, error: %d", apiURL, err)
		return false
	}
	return true
}
