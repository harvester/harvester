package api_test

import (
	"context"
	"testing"
	"time"

	"github.com/rancher/dynamiclistener"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/server"
	"github.com/rancher/harvester/tests/framework/cluster"
	. "github.com/rancher/harvester/tests/framework/dsl"
	"github.com/rancher/harvester/tests/framework/printer"
	"github.com/rancher/harvester/tests/integration/runtime"
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
	beforeSuiteTimeOut    = 1200
	afterSuiteTimeOut     = 600
	harvesterStartTimeOut = 15
)

func TestAPI(t *testing.T) {
	defer GinkgoRecover()

	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t, "api suite", []Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	defer close(done)

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
	case err := <-testSuiteStartErrChan:
		MustNotError(err)
	}
}, beforeSuiteTimeOut)

var _ = AfterSuite(func(done Done) {
	defer close(done)

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

}, afterSuiteTimeOut)
