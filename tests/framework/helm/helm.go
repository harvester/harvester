package helm

import (
	"fmt"
	"os"

	"github.com/onsi/ginkgo/v2"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	"helm.sh/helm/v3/pkg/strvals"
)

var (
	logf = ginkgo.GinkgoT().Logf
)

func NewHelmConfig(namespace string) (*action.Configuration, error) {
	helmCfg := &action.Configuration{}
	err := helmCfg.Init(cli.New().RESTClientGetter(), namespace, "", logf)
	if err != nil {
		return helmCfg, err
	}
	return helmCfg, nil
}

// InstallChart installs the chart or upgrade the same name release.
func InstallChart(releaseName string, namespace string, chartDir string,
	patches map[string]interface{}) (*release.Release, error) {
	_ = os.Setenv("HELM_NAMESPACE", namespace)
	defer func() {
		_ = os.Unsetenv("HELM_NAMESPACE")
	}()

	helmCfg, err := NewHelmConfig(namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to init helm configuration: %w", err)
	}

	var helmRun func(chart *chart.Chart, values map[string]interface{}) (*release.Release, error)

	helmHistory := action.NewHistory(helmCfg)
	helmHistory.Max = 1
	if _, err := helmHistory.Run(releaseName); err != nil {
		if err != driver.ErrReleaseNotFound {
			return nil, fmt.Errorf("failed to get release history: %w", err)
		}

		helmInstall := action.NewInstall(helmCfg)
		helmInstall.Namespace = namespace
		helmInstall.CreateNamespace = true
		helmInstall.ReleaseName = releaseName

		helmRun = helmInstall.Run
	} else {
		helmUpgrade := action.NewUpgrade(helmCfg)
		helmUpgrade.Namespace = namespace
		helmUpgrade.MaxHistory = 1

		helmRun = func(chart *chart.Chart, values map[string]interface{}) (*release.Release, error) {
			return helmUpgrade.Run(releaseName, chart, values)
		}
	}

	// load chart
	loadedChart, err := loader.LoadDir(chartDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}

	// apply patches
	for key, value := range patches {
		patchStr := fmt.Sprintf("%s=%v", key, value)
		if err := strvals.ParseInto(patchStr, loadedChart.Values); err != nil {
			return nil, fmt.Errorf("failed to parse into chart value: %w", err)
		}
	}

	return helmRun(loadedChart, loadedChart.Values)
}

// UninstallChart uninstalls the chart.
func UninstallChart(releaseName string, namespace string) (*release.UninstallReleaseResponse, error) {
	_ = os.Setenv("HELM_NAMESPACE", namespace)
	defer func() {
		_ = os.Unsetenv("HELM_NAMESPACE")
	}()

	helmCfg, err := NewHelmConfig(namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to init helm configuration: %w", err)
	}

	return action.NewUninstall(helmCfg).Run(releaseName)
}
