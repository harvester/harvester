package controllers

import (
	"context"
	"io/ioutil"
	"testing"
	"time"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/wrangler/pkg/generic"
	batchv1 "k8s.io/api/batch/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/controller/master/addon"
	"github.com/harvester/harvester/pkg/server"
	"github.com/harvester/harvester/tests/framework/cluster"
	. "github.com/harvester/harvester/tests/framework/dsl"
	"github.com/harvester/harvester/tests/integration/controllers/fake"
)

var (
	testSuiteStartErrChan chan error
	testCtx               context.Context
	testCtxCancel         context.CancelFunc
	harvester             *server.HarvesterServer

	KubeClientConfig clientcmd.ClientConfig
	testCluster      cluster.Cluster
	options          config.Options
	cfg              *rest.Config
	scaled           *config.Scaled
	testEnv          *envtest.Environment
	scheme           = runtime.NewScheme()
	crdList          = []string{"./manifest/helm-crd.yaml", "../../../deploy/charts/harvester-crd/templates/harvesterhci.io_addons.yaml"}
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

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{"../../../deploy/charts/harvester-crd/templates", "./manifest"},
	}

	By("starting test cluster")
	KubeClientConfig, testCluster, err = cluster.Start(GinkgoWriter)
	MustNotError(err)

	cfg, err = KubeClientConfig.ClientConfig()
	MustNotError(err)

	err = helmv1.AddToScheme(scheme)
	MustNotError(err)

	err = harvesterv1.AddToScheme(scheme)
	MustNotError(err)

	err = batchv1.AddToScheme(scheme)
	MustNotError(err)

	factory, err := controller.NewSharedControllerFactoryFromConfig(cfg, scheme)
	MustNotError(err)

	factoryOpts := &generic.FactoryOptions{
		SharedControllerFactory: factory,
	}

	testCtx, scaled, err = config.SetupScaled(testCtx, cfg, factoryOpts, options.Namespace)
	MustNotError(err)

	var crds []apiextensionsv1.CustomResourceDefinition

	for _, v := range crdList {
		objs, err := generateObjects(v)
		MustNotError(err)
		crds = append(crds, objs)
	}

	err = applyObj(crds)
	MustNotError(err)

	err = addon.Register(testCtx, scaled.Management, config.Options{})
	MustNotError(err)

	// fake job controller to patch statuses
	err = fake.RegisterFakeControllers(testCtx, scaled.Management, config.Options{})
	MustNotError(err)

	go func() {
		err = scaled.Management.Start(5)
		MustNotError(err)
	}()

	time.Sleep(30 * time.Second)
})

var _ = AfterSuite(func() {

	By("tearing down test cluster")
	testEnv.Stop()

	By("tearing down harvester server")
	if testCtxCancel != nil {
		testCtxCancel()
	}

})

func generateObjects(fileName string) (apiextensionsv1.CustomResourceDefinition, error) {
	var result apiextensionsv1.CustomResourceDefinition
	contentBytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return result, err
	}

	err = yaml.Unmarshal(contentBytes, &result)
	if err != nil {
		return result, err
	}
	return result, nil
}

func applyObj(obj []apiextensionsv1.CustomResourceDefinition) error {
	apiClient, err := apiextensionsclient.NewForConfig(cfg)
	if err != nil {
		return err
	}

	for _, v := range obj {
		if _, err := apiClient.ApiextensionsV1().CustomResourceDefinitions().Create(testCtx, &v, metav1.CreateOptions{}); err != nil {
			return err
		}
	}
	return nil
}
