package controllers

import (
	"context"
	"os"
	"testing"
	"time"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	ctlhelmv1 "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/rancher/lasso/pkg/cache"
	"github.com/rancher/lasso/pkg/client"
	"github.com/rancher/lasso/pkg/controller"
	catalogv1 "github.com/rancher/rancher/pkg/apis/catalog.cattle.io/v1"
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	ctlcatalogv1 "github.com/rancher/rancher/pkg/generated/controllers/catalog.cattle.io"
	ctlrancherv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io"
	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core"
	"github.com/rancher/wrangler/v3/pkg/generic"
	"github.com/rancher/wrangler/v3/pkg/start"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/controller/master/addon"
	"github.com/harvester/harvester/pkg/controller/master/mcmsettings"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io"
	"github.com/harvester/harvester/tests/framework/cluster"
	"github.com/harvester/harvester/tests/framework/dsl"
	"github.com/harvester/harvester/tests/integration/controllers/fake"
)

var (
	testCtx       context.Context
	testCtxCancel context.CancelFunc

	KubeClientConfig clientcmd.ClientConfig
	testCluster      cluster.Cluster
	cfg              *rest.Config
	scaled           *config.Scaled
	scheme           = runtime.NewScheme()
	crdList          = []string{"./manifest/helm-crd.yaml", "./manifest/app-crd.yaml", "./manifest/ranchersettings-crd.yaml", "./manifest/clusterrepos-crd.yaml", "../../../deploy/charts/harvester-crd/templates/harvesterhci.io_addons.yaml"}
)

func TestAPI(t *testing.T) {
	defer ginkgo.GinkgoRecover()

	gomega.RegisterFailHandler(ginkgo.Fail)

	ginkgo.RunSpecs(t, "api suite")
}

var _ = ginkgo.BeforeSuite(func() {
	testCtx, testCtxCancel = context.WithCancel(context.Background())
	var err error

	ginkgo.By("starting test cluster")
	KubeClientConfig, testCluster, err = cluster.Start(ginkgo.GinkgoWriter)
	dsl.MustNotError(err)

	cfg, err = KubeClientConfig.ClientConfig()
	dsl.MustNotError(err)

	kubeConfig, err := KubeClientConfig.ClientConfig()
	dsl.MustNotError(err)

	ginkgo.By("install crds")
	var crds []apiextensionsv1.CustomResourceDefinition

	for _, v := range crdList {
		objs, err := generateObjects(v)
		dsl.MustNotError(err)
		crds = append(crds, objs)
	}
	err = applyObj(crds)
	dsl.MustNotError(err)

	err = helmv1.AddToScheme(scheme)
	dsl.MustNotError(err)

	err = harvesterv1.AddToScheme(scheme)
	dsl.MustNotError(err)

	err = batchv1.AddToScheme(scheme)
	dsl.MustNotError(err)

	err = catalogv1.AddToScheme(scheme)
	dsl.MustNotError(err)

	err = managementv3.AddToScheme(scheme)
	dsl.MustNotError(err)

	err = corev1.AddToScheme(scheme)
	dsl.MustNotError(err)

	err = catalogv1.AddToScheme(scheme)
	dsl.MustNotError(err)

	clientFactory, err := client.NewSharedClientFactory(kubeConfig, nil)
	dsl.MustNotError(err)

	cacheFactory := cache.NewSharedCachedFactory(clientFactory, nil)
	scf := controller.NewSharedControllerFactory(cacheFactory, &controller.SharedControllerFactoryOptions{})

	factoryOpts := &generic.FactoryOptions{
		SharedControllerFactory: scf,
	}

	testCtx, scaled, err = config.SetupScaled(testCtx, cfg, factoryOpts)
	dsl.MustNotError(err)

	err = startControllers(testCtx, kubeConfig, factoryOpts)
	dsl.MustNotError(err)

})

var _ = ginkgo.AfterSuite(func() {

	testCtx.Done()
	ginkgo.By("tearing down test cluster")
	err := cluster.Stop(ginkgo.GinkgoWriter)
	dsl.MustNotError(err)

	ginkgo.By("tearing down harvester server")
	if testCtxCancel != nil {
		testCtxCancel()
	}

})

func generateObjects(fileName string) (apiextensionsv1.CustomResourceDefinition, error) {
	var result apiextensionsv1.CustomResourceDefinition
	contentBytes, err := os.ReadFile(fileName)
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

	for i := range obj {
		if _, err := apiClient.ApiextensionsV1().CustomResourceDefinitions().Create(testCtx, &obj[i], metav1.CreateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func startControllers(ctx context.Context, restConfig *rest.Config, opts *ctlharvesterv1.FactoryOptions) error {

	// to speed up testing, override default backofflimit for jobs
	harvesterv1.DefaultJobBackOffLimit = 1

	harvesterFactory, err := ctlharvesterv1.NewFactoryFromConfigWithOptions(restConfig, opts)

	if err != nil {
		return err
	}

	core, err := ctlcorev1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return err
	}

	batch, err := ctlbatchv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return err
	}

	helm, err := ctlhelmv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return err
	}

	catalog, err := ctlcatalogv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return err
	}

	rancher, err := ctlrancherv3.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return err
	}

	m := &config.Management{
		HarvesterFactory:         harvesterFactory,
		CoreFactory:              core,
		BatchFactory:             batch,
		HelmFactory:              helm,
		CatalogFactory:           catalog,
		RancherManagementFactory: rancher,
	}

	_ = batch.ControllerFactory().SharedCacheFactory().WaitForCacheSync(ctx)

	if err = addon.Register(testCtx, m, config.Options{}); err != nil {
		return err
	}

	if err = mcmsettings.Register(testCtx, m, config.Options{}); err != nil {
		return err
	}

	if err = fake.RegisterFakeControllers(testCtx, m, config.Options{}); err != nil {
		return err
	}

	logrus.Infof("sync status of batch informer: %v", batch.Batch().V1().Job().Informer().HasSynced())
	if err = start.All(ctx, 10, harvesterFactory, core, batch, helm, catalog, rancher); err != nil {
		return err
	}

	for !batch.Batch().V1().Job().Informer().HasSynced() {
		time.Sleep(5 * time.Second)
	}

	return nil
}
