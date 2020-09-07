package api

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rancher/wrangler-api/pkg/generated/controllers/apps"
	"github.com/rancher/wrangler-api/pkg/generated/controllers/core"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/server"
	"github.com/rancher/harvester/pkg/settings"
	"github.com/rancher/harvester/test/framework/envtest"
	"github.com/rancher/harvester/test/framework/envtest/printer"
	"github.com/rancher/harvester/test/framework/fuzz"

	dockerapi "github.com/docker/docker/api/types"
	dockercontainerapi "github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/rancher/wrangler/pkg/merr"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/storage/driver"
	"helm.sh/helm/v3/pkg/strvals"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	testSuiteStartErrChan chan error
	testCtx               context.Context
	testCtxCancel         context.CancelFunc

	k8sClientCfg clientcmd.ClientConfig
	harvester    *server.HarvesterServer

	testMinioContainerID   string
	testHarvesterNamespace string
)

func TestAPI(t *testing.T) {
	defer GinkgoRecover()

	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"api suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	defer close(done)

	testCtx, testCtxCancel = context.WithCancel(context.Background())
	var err error

	By("starting test environment")
	k8sClientCfg, err = envtest.Start(GinkgoWriter)
	if err != nil {
		GinkgoT().Errorf("failed to start test env, %v", err)
	}

	By("starting harvester runtime")
	err = constructHarvesterRuntime()
	if err != nil {
		GinkgoT().Errorf("failed to construct harvester runtime, %v", err)
	}

	By("starting harvester")
	harvester, err = server.New(
		testCtx,
		k8sClientCfg,
	)
	if err != nil {
		GinkgoT().Errorf("failed to create harvester server, %v", err)
	}
	testSuiteStartErrChan = make(chan error)
	go func() {
		testSuiteStartErrChan <- harvester.Start()
	}()

	// NB(thxCode): since the start of all controllers is not synchronized,
	// it cannot guarantee the controllers has been start,
	// which means the cache(informer) has not ready,
	// so we give a stupid time sleep to trigger the first list-watch,
	// and please use the client interface instead of informer interface if you can.
	select {
	case <-time.After(15 * time.Second):
	case err := <-testSuiteStartErrChan:
		// release the chan
		close(testSuiteStartErrChan)
		testSuiteStartErrChan = nil
		GinkgoT().Errorf("failed to start harvester server, %v", err)
	}
}, 900)

var _ = AfterSuite(func(done Done) {
	defer close(done)

	By("tearing down harvester runtime")
	destructHarvesterRuntime()

	By("tearing down test environment")
	var err = envtest.Stop(GinkgoWriter)
	if err != nil {
		GinkgoT().Logf("failed to stop test env, %v", err)
	}

	By("tearing down harvester")
	if testCtxCancel != nil {
		testCtxCancel()
		testCtxCancel = nil
	}
	if testSuiteStartErrChan != nil {
		// doesn't care about the content from errChan
		<-testSuiteStartErrChan
		close(testSuiteStartErrChan)
		testCtxCancel = nil
	}
}, 600)

// constructHarvesterRuntime prepares the envs, runtime and configures the public variables exported in github.com/rancher/harvester/pkg/config package.
func constructHarvesterRuntime() error {
	// generate two random ports
	ports, err := fuzz.FreePorts(2)
	if err != nil {
		return fmt.Errorf("failed to get listening ports of harvester server, %v", err)
	}
	config.HTTPListenPort = ports[0]
	config.HTTPSListenPort = ports[1]

	// create test namespace
	testHarvesterNamespace = "harvester-system"
	k8sCfg, err := k8sClientCfg.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to gain kubernetes config, %v", err)
	}
	coreFactory, err := core.NewFactoryFromConfig(k8sCfg)
	if err != nil {
		return fmt.Errorf("faield to create core dockerclient factory from kubernetes config, %v", err)
	}
	var namespaceCli = coreFactory.Core().V1().Namespace()
	_, err = namespaceCli.Create(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testHarvesterNamespace}})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create target namespace, %v", err)
	}
	config.Namespace = testHarvesterNamespace

	// inject the preset envs, this is used for testing setting.
	err = os.Setenv(settings.GetEnvKey(settings.APIUIVersion.Name), settings.APIUIVersion.Default)
	if err != nil {
		return fmt.Errorf("failed to preset ENVs of harvester server, %w", err)
	}

	// start image storage service
	imageStorageEndpoint, imageStorageAccessKey, imageStorageSecretKey, err := startMinioContainer()
	if err != nil {
		return fmt.Errorf("failed to start image storage service, %v", err)
	}
	config.ImageStorageEndpoint = imageStorageEndpoint
	config.ImageStorageAccessKey = imageStorageAccessKey
	config.ImageStorageSecretKey = imageStorageSecretKey

	// install harvester chart
	err = installHarvesterBasicComponents()
	if err != nil {
		return fmt.Errorf("failed to install harvester chart, %w", err)
	}

	return nil
}

// destructHarvesterRuntime releases the runtime.
func destructHarvesterRuntime() {
	// uninstall harvester chart
	var err = uninstallHarvesterBasicComponents()
	if err != nil {
		GinkgoT().Logf("failed to uninstall Harvester chart: %v", err)
	}

	// start image storage service
	err = stopMinioContainer()
	if err != nil {
		GinkgoT().Logf("failed to stop Minio container: %v", err)
	}
}

// installHarvesterBasicComponents installs the basic components of harvester.
func installHarvesterBasicComponents() error {
	// verifies the installed components
	k8sCfg, err := k8sClientCfg.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to gain kubernetes config, %v", err)
	}
	appsFactory, err := apps.NewFactoryFromConfig(k8sCfg)
	if err != nil {
		return fmt.Errorf("faield to create core dockerclient factory from kubernetes config: %w", err)
	}

	// installs cdi-operator
	var cdiOperatorValues = map[string]interface{}{}
	err = merr.NewErrors(
		// --set containers.operator.image.imagePullPolicy=IfNotPresent
		strvals.ParseInto("containers.operator.image.imagePullPolicy=IfNotPresent", cdiOperatorValues),
	)
	if err != nil {
		return fmt.Errorf("failed to configure cdi-operator chart: %w", err)
	}
	_, err = installChart("cdi-operator", "testdata/cdi-operator", cdiOperatorValues)
	if err != nil {
		return fmt.Errorf("failed to load cdi-operator chart: %w", err)
	}
	// verifies cdi-operator installation
	err = wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		var deploymentClient = appsFactory.Apps().V1().Deployment()

		var cdiOperatorDeployment, err = deploymentClient.Get(testHarvesterNamespace, "cdi-operator", metav1.GetOptions{})
		if err != nil {
			GinkgoT().Logf("failed to query cdi-operator deployment, %v", err)
			return false, nil
		}
		if !isDeploymentReady(&cdiOperatorDeployment.Status) {
			return false, nil
		}

		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to confirm cdi-operator installed components: %w", err)
	}

	// installs cdi
	_, err = installChart("cdi", "testdata/cdi", map[string]interface{}{})
	if err != nil {
		return fmt.Errorf("failed to load cdi chart: %w", err)
	}
	// verifies cdi installation
	err = wait.PollImmediate(3*time.Second, 5*time.Minute, func() (bool, error) {
		var deploymentClient = appsFactory.Apps().V1().Deployment()

		var cdiAPIServerDeployment, err = deploymentClient.Get(testHarvesterNamespace, "cdi-apiserver", metav1.GetOptions{})
		if err != nil {
			GinkgoT().Logf("failed to query cdi-apiserver deployment, %v", err)
			return false, nil
		}
		if !isDeploymentReady(&cdiAPIServerDeployment.Status) {
			return false, nil
		}

		cdiDeploymentDeployment, err := deploymentClient.Get(testHarvesterNamespace, "cdi-deployment", metav1.GetOptions{})
		if err != nil {
			GinkgoT().Logf("failed to query cdi-deployment deployment, %v", err)
			return false, nil
		}
		if !isDeploymentReady(&cdiDeploymentDeployment.Status) {
			return false, nil
		}

		cdiUploadProxyDeployment, err := deploymentClient.Get(testHarvesterNamespace, "cdi-uploadproxy", metav1.GetOptions{})
		if err != nil {
			GinkgoT().Logf("failed to query cdi-uploadproxy deployment, %v", err)
			return false, nil
		}
		if !isDeploymentReady(&cdiUploadProxyDeployment.Status) {
			return false, nil
		}

		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to confirm cdi installed components: %w", err)
	}

	// installs kubevirt-operator
	var kubevirtOperatorValues = map[string]interface{}{}
	err = merr.NewErrors(
		// --set containers.operator.image.imagePullPolicy=IfNotPresent
		strvals.ParseInto("containers.operator.image.imagePullPolicy=IfNotPresent", kubevirtOperatorValues),
	)
	if err != nil {
		return fmt.Errorf("failed to configure kubevirt-operator chart: %w", err)
	}
	_, err = installChart("kubevirt-operator", "testdata/kubevirt-operator", kubevirtOperatorValues)
	if err != nil {
		return fmt.Errorf("failed to load kubevirt-operator chart: %w", err)
	}
	// verifies kubvirt-operator installation
	err = wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		var deploymentClient = appsFactory.Apps().V1().Deployment()

		var virtOperatorDeployment, err = deploymentClient.Get(testHarvesterNamespace, "virt-operator", metav1.GetOptions{})
		if err != nil {
			GinkgoT().Logf("failed to query kubevirt-operator deployment, %v", err)
			return false, nil
		}
		if !isDeploymentReady(&virtOperatorDeployment.Status) {
			return false, nil
		}

		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to confirm kubevirt-operator installed components: %w", err)
	}

	// installs kubevirt
	var kubevirtValues = map[string]interface{}{}
	err = merr.NewErrors(
		// --set-string spec.configuration.developerConfiguration.useEmulation=true
		strvals.ParseIntoString("spec.configuration.developerConfiguration.useEmulation=true", kubevirtValues),
	)
	if err != nil {
		return fmt.Errorf("failed to configure kubevirt chart: %w", err)
	}
	_, err = installChart("kubevirt", "testdata/kubevirt", kubevirtValues)
	if err != nil {
		return fmt.Errorf("failed to load kubevirt chart: %w", err)
	}
	// verifies kubvirt installation
	err = wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		var deploymentClient = appsFactory.Apps().V1().Deployment()
		var daemonsetClient = appsFactory.Apps().V1().DaemonSet()

		var virtAPIDeployment, err = deploymentClient.Get(testHarvesterNamespace, "virt-api", metav1.GetOptions{})
		if err != nil {
			GinkgoT().Logf("failed to query virt-api deployment, %v", err)
			return false, nil
		}
		if !isDeploymentReady(&virtAPIDeployment.Status) {
			return false, nil
		}

		virtControllerDeployment, err := deploymentClient.Get(testHarvesterNamespace, "virt-controller", metav1.GetOptions{})
		if err != nil {
			GinkgoT().Logf("failed to query virt-controller deployment, %v", err)
			return false, nil
		}
		if !isDeploymentReady(&virtControllerDeployment.Status) {
			return false, nil
		}

		virtHandlerDaemonset, err := daemonsetClient.Get(testHarvesterNamespace, "virt-handler", metav1.GetOptions{})
		if err != nil {
			GinkgoT().Logf("failed to query virt-handler deployment, %v", err)
			return false, nil
		}
		if !isDaemonsetReady(&virtHandlerDaemonset.Status) {
			return false, nil
		}

		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to confirm kubevirt-operator installed components: %w", err)
	}

	return nil
}

// uninstallHarvesterBasicComponents uninstalls the basic components of harvester.
func uninstallHarvesterBasicComponents() error {
	// verifies the installed components
	k8sCfg, err := k8sClientCfg.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to gain kubernetes config, %v", err)
	}
	appsFactory, err := apps.NewFactoryFromConfig(k8sCfg)
	if err != nil {
		return fmt.Errorf("faield to create core dockerclient factory from kubernetes config: %w", err)
	}

	// uninstall cdi
	_, err = uninstallChart("cdi")
	if err != nil {
		GinkgoT().Logf("failed to uninstall cdi chart: %v", err)
	}
	// verifies cdi uninstallation
	err = wait.PollImmediate(3*time.Second, 5*time.Minute, func() (bool, error) {
		var deploymentClient = appsFactory.Apps().V1().Deployment()

		_, err := deploymentClient.Get(testHarvesterNamespace, "cdi-apiserver", metav1.GetOptions{})
		if err == nil || !apierrors.IsNotFound(err) {
			return false, nil
		}

		_, err = deploymentClient.Get(testHarvesterNamespace, "cdi-deployment", metav1.GetOptions{})
		if err == nil || !apierrors.IsNotFound(err) {
			return false, nil
		}

		_, err = deploymentClient.Get(testHarvesterNamespace, "cdi-uploadproxy", metav1.GetOptions{})
		if err == nil || !apierrors.IsNotFound(err) {
			return false, nil
		}

		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to confirm cdi uninstalled components: %w", err)
	}

	// uninstall kubevirt
	_, err = uninstallChart("kubevirt")
	if err != nil {
		GinkgoT().Logf("failed to uninstall kubevirt chart: %v", err)
	}
	// verifies kubevirt uninstallation
	err = wait.PollImmediate(3*time.Second, 5*time.Minute, func() (bool, error) {
		var deploymentClient = appsFactory.Apps().V1().Deployment()
		var daemonsetClient = appsFactory.Apps().V1().DaemonSet()

		_, err := deploymentClient.Get(testHarvesterNamespace, "virt-api", metav1.GetOptions{})
		if err == nil || !apierrors.IsNotFound(err) {
			return false, nil
		}

		_, err = deploymentClient.Get(testHarvesterNamespace, "virt-controller", metav1.GetOptions{})
		if err == nil || !apierrors.IsNotFound(err) {
			return false, nil
		}

		_, err = daemonsetClient.Get(testHarvesterNamespace, "virt-handler", metav1.GetOptions{})
		if err == nil || !apierrors.IsNotFound(err) {
			return false, nil
		}

		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to confirm cdi uninstalled components: %w", err)
	}

	// uninstall cdi-operator
	_, err = uninstallChart("cdi-operator")
	if err != nil {
		GinkgoT().Logf("failed to uninstall cdi-operator chart: %v", err)
	}

	// uninstall kubevirt-operator
	_, err = uninstallChart("kubevirt-operator")
	if err != nil {
		GinkgoT().Logf("failed to uninstall kubevirt-operator chart: %v", err)
	}

	return nil
}

// isDeploymentReady checks if the Deployment is ready.
func isDeploymentReady(status *v1.DeploymentStatus) bool {
	if status == nil {
		return false
	}
	return status.Replicas == status.AvailableReplicas
}

// isDaemonsetReady checks if the Daemonset is ready.
func isDaemonsetReady(status *v1.DaemonSetStatus) bool {
	if status == nil {
		return false
	}
	return status.NumberAvailable == status.NumberReady
}

// startMinioContainer starts a Minio container as the image storage service.
func startMinioContainer() (endpoint, accessKey, secretKey string, berr error) {
	// generate a random port
	var ports, err = fuzz.FreePorts(1)
	if err != nil {
		berr = fmt.Errorf("failed to get listening port of image storage service, %v", err)
		return
	}

	endpoint = fmt.Sprintf("http://localhost:%d", ports[0])
	accessKey = fuzz.String(10)
	secretKey = fuzz.String(20)

	// create docker client
	dClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv)
	if err != nil {
		berr = fmt.Errorf("failed to create docker client, %v", err)
		return
	}

	// create minio container
	minoCreatedResp, err := dClient.ContainerCreate(
		testCtx,
		&dockercontainerapi.Config{
			Image: "minio/minio:RELEASE.2020-08-08T04-50-06Z",
			Env: []string{
				fmt.Sprintf("MINIO_ACCESS_KEY=%s", accessKey),
				fmt.Sprintf("MINIO_SECRET_KEY=%s", secretKey),
			},
			Cmd: []string{
				"server",
				"/data",
			},
			ExposedPorts: nat.PortSet{
				"9000/tcp": struct{}{},
			},
		},
		&dockercontainerapi.HostConfig{
			AutoRemove: true,
			PortBindings: nat.PortMap{
				"9000/tcp": []nat.PortBinding{
					{
						HostIP:   "0.0.0.0",
						HostPort: strconv.Itoa(ports[0]),
					},
				},
			},
		},
		nil,
		"",
	)
	if err != nil {
		berr = fmt.Errorf("failed to create minio container, %v", err)
		return
	}
	if len(minoCreatedResp.Warnings) != 0 {
		for _, warning := range minoCreatedResp.Warnings {
			GinkgoT().Log(warning)
		}
	}
	testMinioContainerID = minoCreatedResp.ID

	// create minio container
	err = dClient.ContainerStart(testCtx, testMinioContainerID, dockerapi.ContainerStartOptions{})
	if err != nil {
		berr = fmt.Errorf("failed to start minio container, %v", err)
		return
	}

	return
}

// stopMinioContainer stops the Minio container.
func stopMinioContainer() error {
	if testMinioContainerID == "" {
		return nil
	}

	// create docker client
	dClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv)
	if err != nil {
		return fmt.Errorf("failed to create docker client, %v", err)
	}

	// stop minio container
	err = dClient.ContainerStop(testCtx, testMinioContainerID, nil)
	if err != nil {
		return fmt.Errorf("failed to stop minio container, %v", err)
	}

	// delete minio container
	_ = dClient.ContainerRemove(testCtx, testMinioContainerID, dockerapi.ContainerRemoveOptions{})

	return nil
}

// installChart installs the chart or upgrade the same name release.
func installChart(releaseName, chartDir string, values map[string]interface{}) (*release.Release, error) {
	_ = os.Setenv("HELM_NAMESPACE", testHarvesterNamespace)
	defer func() {
		_ = os.Unsetenv("HELM_NAMESPACE")
	}()

	var helmCfg = &action.Configuration{}
	if err := helmCfg.Init(
		cli.New().RESTClientGetter(),
		testHarvesterNamespace,
		"",
		func(format string, v ...interface{}) {
			GinkgoT().Logf(format, v...)
		},
	); err != nil {
		return nil, fmt.Errorf("failed to init helm configuration: %w", err)
	}

	var helmUpstall func(chart *chart.Chart, values map[string]interface{}) (*release.Release, error)
	var helmHistory = action.NewHistory(helmCfg)
	helmHistory.Max = 1
	if _, err := helmHistory.Run(releaseName); err != nil {
		if err != driver.ErrReleaseNotFound {
			return nil, fmt.Errorf("failed to get release history: %w", err)
		}

		var helmInstall = action.NewInstall(helmCfg)
		helmInstall.Namespace = testHarvesterNamespace
		helmInstall.CreateNamespace = true
		helmInstall.Atomic = true
		helmInstall.ReleaseName = releaseName

		helmUpstall = helmInstall.Run
	} else {
		var helmUpgrade = action.NewUpgrade(helmCfg)
		helmUpgrade.Namespace = testHarvesterNamespace
		helmUpgrade.MaxHistory = 1
		helmUpgrade.Atomic = true

		helmUpstall = func(chart *chart.Chart, values map[string]interface{}) (*release.Release, error) {
			return helmUpgrade.Run(releaseName, chart, values)
		}
	}

	var loadedChart, err = loader.LoadDir(chartDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}

	return helmUpstall(loadedChart, values)
}

// uninstallChart uninstalls the chart.
func uninstallChart(releaseName string) (*release.UninstallReleaseResponse, error) {
	_ = os.Setenv("HELM_NAMESPACE", testHarvesterNamespace)
	defer func() {
		_ = os.Unsetenv("HELM_NAMESPACE")
	}()

	var helmCfg = &action.Configuration{}
	if err := helmCfg.Init(
		cli.New().RESTClientGetter(),
		testHarvesterNamespace,
		"",
		func(format string, v ...interface{}) {
			GinkgoT().Logf(format, v...)
		},
	); err != nil {
		return nil, fmt.Errorf("failed to init helm configuration: %w", err)
	}

	var helmUninstall = action.NewUninstall(helmCfg)
	return helmUninstall.Run(releaseName)
}
