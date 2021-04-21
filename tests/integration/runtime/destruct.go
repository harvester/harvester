package runtime

import (
	"context"
	"fmt"

	restclient "k8s.io/client-go/rest"

	"github.com/harvester/harvester/tests/framework/env"
	"github.com/harvester/harvester/tests/framework/helm"
	"github.com/harvester/harvester/tests/framework/ready"
)

// Destruct releases the runtime if "SKIP_HARVESTER_INSTALLATION" is not "true".
func Destruct(ctx context.Context, kubeConfig *restclient.Config) error {
	if env.IsKeepingHarvesterInstallation() || env.IsSkipHarvesterInstallation() {
		return nil
	}

	// uninstall harvester chart
	err := uninstallHarvesterCharts(ctx, kubeConfig)
	if err != nil {
		return err
	}

	return nil
}

// uninstallHarvesterCharts uninstalls the basic components of harvester.
func uninstallHarvesterCharts(ctx context.Context, kubeConfig *restclient.Config) error {
	// uninstall chart
	_, err := helm.UninstallChart(testChartReleaseName, testHarvesterNamespace)
	if err != nil {
		return fmt.Errorf("failed to uninstall harvester chart: %v", err)
	}

	// verifies chart uninstallation
	namespaceReadyCondition, err := ready.NewNamespaceCondition(kubeConfig, testHarvesterNamespace)
	if err != nil {
		return fmt.Errorf("faield to create namespace ready condition from kubernetes config: %w", err)
	}
	namespaceReadyCondition.AddDeploymentsClean(testDeploymentManifest...)
	namespaceReadyCondition.AddDaemonSetsClean(testDaemonSetManifest...)

	return namespaceReadyCondition.Wait(ctx)
}
