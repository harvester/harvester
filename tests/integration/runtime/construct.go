package runtime

import (
	"context"
	"fmt"

	restclient "k8s.io/client-go/rest"

	"github.com/harvester/harvester/tests/framework/client"
	"github.com/harvester/harvester/tests/framework/env"
	"github.com/harvester/harvester/tests/framework/helm"
	"github.com/harvester/harvester/tests/framework/ready"
)

// Construct prepares runtime if "SKIP_HARVESTER_INSTALLATION" is not "true".
func Construct(ctx context.Context, kubeConfig *restclient.Config) error {
	if env.IsSkipHarvesterInstallation() {
		return nil
	}

	// create namespaces
	var err error
	namespaces := []string{testHarvesterNamespace, testLonghornNamespace, testCattleNamespace}
	for _, namespace := range namespaces {
		err = client.CreateNamespace(kubeConfig, namespace)
		if err != nil {
			return fmt.Errorf("failed to create target namespace %s, %v", namespace, err)
		}
	}

	err = createCRDs(ctx, kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to create CRDs, %v", err)
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
		"replicas":                             0,
		"harvester-network-controller.enabled": true,
	}

	// webhook
	patches["webhook.controllerUser"] = "kubernetes-admin"
	patches["webhook.image.imagePullPolicy"] = "Never"
	repo, tag := env.GetWebhookImage()
	if repo != "" {
		patches["webhook.image.repository"] = repo
		patches["webhook.image.tag"] = tag
		patches["webhook.debug"] = true
	}

	if !env.IsE2ETestsEnabled() {
		patches["longhorn.enabled"] = "false"
	}

	if env.IsUsingEmulation() {
		patches["kubevirt.spec.configuration.developerConfiguration.useEmulation"] = "true"
	}

	// install crd chart
	_, err := helm.InstallChart(testCRDChartReleaseName, testHarvesterNamespace, testCRDChartDir, nil)
	if err != nil {
		return fmt.Errorf("failed to install harvester-crd chart: %w", err)
	}

	// install chart
	_, err = helm.InstallChart(testChartReleaseName, testHarvesterNamespace, testChartDir, patches)
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
