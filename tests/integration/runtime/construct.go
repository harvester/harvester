package runtime

import (
	"context"
	"fmt"

	restclient "k8s.io/client-go/rest"

	"github.com/rancher/harvester/tests/framework/client"
	"github.com/rancher/harvester/tests/framework/env"
	"github.com/rancher/harvester/tests/framework/helm"
	"github.com/rancher/harvester/tests/framework/ready"
)

// Construct prepares runtime if "SKIP_HARVESTER_INSTALLATION" is not "true".
func Construct(ctx context.Context, kubeConfig *restclient.Config) error {
	if env.IsSkipHarvesterInstallation() {
		return nil
	}

	// create namespace
	err := client.CreateNamespace(kubeConfig, testHarvesterNamespace)
	if err != nil {
		return fmt.Errorf("failed to create target namespace, %v", err)
	}

	// install harvester chart
	err = installHarvesterChart(ctx, kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to install harvester chart, %w", err)
	}

	return nil
}

// installHarvesterChart installs the basic components of harvester.
func installHarvesterChart(ctx context.Context, kubeConfig *restclient.Config) error {
	// chart values patches
	patches := map[string]interface{}{
		"replicas":               0,
		"minio.service.type":     "NodePort",
		"minio.mode":             "standalone",
		"minio.persistence.size": "5Gi",
	}
	if env.IsE2ETestsEnabled() {
		patches["longhorn.enabled"] = "true"
	}

	if env.IsUsingEmulation() {
		patches["kubevirt.spec.configuration.developerConfiguration.useEmulation"] = "true"
	}

	// install chart
	_, err := helm.InstallChart(testChartReleaseName, testHarvesterNamespace, testChartDir, patches)
	if err != nil {
		return fmt.Errorf("failed to install harvester chart: %w", err)
	}

	// verifies chart installation
	harvesterReadyCondition, err := ready.NewNamespaceCondition(kubeConfig, testHarvesterNamespace)
	if err != nil {
		return fmt.Errorf("faield to create namespace ready condition from kubernetes config: %w", err)
	}
	harvesterReadyCondition.AddDeploymentsReady(testDeploymentManifest...)
	harvesterReadyCondition.AddDaemonSetsReady(testDaemonSetManifest...)

	if env.IsE2ETestsEnabled() {
		longhornReadyCondition, err := ready.NewNamespaceCondition(kubeConfig, testLonghornNamespace)
		if err != nil {
			return fmt.Errorf("faield to create namespace ready condition from kubernetes config: %w", err)
		}
		longhornReadyCondition.AddDeploymentsReady(longhornDeploymentManifest...)
		longhornReadyCondition.AddDaemonSetsReady(longhornDaemonSetManifest...)

		if err := longhornReadyCondition.Wait(ctx); err != nil {
			return err
		}
	}

	if err := harvesterReadyCondition.Wait(ctx); err != nil {
		return err
	}

	return nil
}
