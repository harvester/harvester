package upgrade

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func newTestUpgradeRepo() (*Repo, *fake.Clientset) {
	upgrade := newTestUpgradeBuilder().InitStatus().
		WithImage(testUpgradeImage).
		ImageIDStatus(fmt.Sprintf("%s/%s", upgradeNamespace, testUpgradeImage)).
		ImageReadyCondition(corev1.ConditionTrue, "", "").Build()
	clientset := fake.NewSimpleClientset(
		upgrade,
		newTestExistingVirtualMachineImage(upgradeNamespace, testUpgradeImage),
		newNodeBuilder(testNodeName).Build(),
	)
	handler := &upgradeHandler{
		ctx:              context.Background(),
		upgradeClient:    fakeclients.UpgradeClient(clientset.HarvesterhciV1beta1().Upgrades),
		nodeCache:        fakeclients.NodeCache(clientset.CoreV1().Nodes),
		vmImageCache:     fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
		deploymentClient: fakeclients.DeploymentClient(clientset.AppsV1().Deployments),
		serviceClient:    fakeclients.ServiceClient(clientset.CoreV1().Services),
	}
	return NewUpgradeRepo(handler.ctx, upgrade, handler), clientset
}

func TestRepoBootstrap(t *testing.T) {
	testCases := []struct {
		name             string
		deploymentExists bool
		serviceExists    bool
	}{
		{name: "create both resources"},
		{name: "create service when deployment exists", deploymentExists: true},
		{name: "create deployment when service exists", serviceExists: true},
		{name: "reuse both resources", deploymentExists: true, serviceExists: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := newTestUpgradeRepo()
			if tc.deploymentExists {
				image, err := repo.GetImage(repo.upgrade.Status.ImageID)
				require.NoError(t, err)
				_, err = repo.CreateDeployment(image)
				require.NoError(t, err)
			}
			if tc.serviceExists {
				_, err := repo.createService()
				require.NoError(t, err)
			}

			require.NoError(t, repo.Bootstrap())
			deployment, err := repo.h.deploymentClient.Get(upgradeNamespace, repo.GetDeploymentName(), metav1.GetOptions{})
			require.NoError(t, err)
			service, err := repo.h.serviceClient.Get(upgradeNamespace, repo.getRepoServiceName(), metav1.GetOptions{})
			require.NoError(t, err)

			require.NoError(t, repo.Bootstrap())
			actualDeployment, err := repo.h.deploymentClient.Get(upgradeNamespace, repo.GetDeploymentName(), metav1.GetOptions{})
			require.NoError(t, err)
			actualService, err := repo.h.serviceClient.Get(upgradeNamespace, repo.getRepoServiceName(), metav1.GetOptions{})
			require.NoError(t, err)
			assert.Equal(t, deployment, actualDeployment)
			assert.Equal(t, service, actualService)
		})
	}
}

func TestRepoBootstrapRetry(t *testing.T) {
	testCases := []struct {
		name             string
		resource         string
		deploymentExists bool
	}{
		{name: "deployment creation failure", resource: "deployments"},
		{name: "service creation failure", resource: "services"},
		{name: "service creation failure with existing deployment", resource: "services", deploymentExists: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repo, clientset := newTestUpgradeRepo()
			if tc.deploymentExists {
				image, err := repo.GetImage(repo.upgrade.Status.ImageID)
				require.NoError(t, err)
				_, err = repo.CreateDeployment(image)
				require.NoError(t, err)
			}
			createErr := errors.New("temporary create failure")
			attempts := 0
			clientset.PrependReactor("create", tc.resource, func(_ k8stesting.Action) (bool, runtime.Object, error) {
				attempts++
				if attempts == 1 {
					return true, nil, createErr
				}
				return false, nil, nil
			})

			require.ErrorIs(t, repo.Bootstrap(), createErr)
			_, err := repo.h.serviceClient.Get(upgradeNamespace, repo.getRepoServiceName(), metav1.GetOptions{})
			require.True(t, apierrors.IsNotFound(err))
			_, err = repo.h.deploymentClient.Get(upgradeNamespace, repo.GetDeploymentName(), metav1.GetOptions{})
			if tc.resource == "deployments" {
				require.True(t, apierrors.IsNotFound(err))
			} else {
				require.NoError(t, err)
			}

			require.NoError(t, repo.Bootstrap())
			_, err = repo.h.deploymentClient.Get(upgradeNamespace, repo.GetDeploymentName(), metav1.GetOptions{})
			require.NoError(t, err)
			_, err = repo.h.serviceClient.Get(upgradeNamespace, repo.getRepoServiceName(), metav1.GetOptions{})
			require.NoError(t, err)
			assert.Equal(t, 2, attempts)
		})
	}
}

func TestUpgradeBootstrapServiceFailure(t *testing.T) {
	for _, updateConflict := range []bool{false, true} {
		t.Run(fmt.Sprintf("update conflict=%t", updateConflict), func(t *testing.T) {
			repo, clientset := newTestUpgradeRepo()
			serviceErr := errors.New("temporary service creation failure")
			serviceAttempts := 0
			clientset.PrependReactor("create", "services", func(_ k8stesting.Action) (bool, runtime.Object, error) {
				serviceAttempts++
				if serviceAttempts == 1 {
					return true, nil, serviceErr
				}
				return false, nil, nil
			})
			if updateConflict {
				updateAttempts := 0
				clientset.PrependReactor("update", "upgrades", func(_ k8stesting.Action) (bool, runtime.Object, error) {
					updateAttempts++
					if updateAttempts == 1 {
						return true, nil, apierrors.NewConflict(
							schema.GroupResource{Group: "harvesterhci.io", Resource: "upgrades"},
							repo.upgrade.Name, errors.New("resource version changed"))
					}
					return false, nil, nil
				})
			}

			_, err := repo.h.OnChanged(repo.upgrade.Name, repo.upgrade)
			if updateConflict {
				require.True(t, apierrors.IsConflict(err))
			} else {
				require.NoError(t, err)
			}
			upgrade, err := clientset.HarvesterhciV1beta1().Upgrades(upgradeNamespace).Get(context.Background(), repo.upgrade.Name, metav1.GetOptions{})
			require.NoError(t, err)
			assert.Empty(t, harvesterv1.RepoProvisioned.GetStatus(upgrade))
			_, err = repo.h.serviceClient.Get(upgradeNamespace, repo.getRepoServiceName(), metav1.GetOptions{})
			require.True(t, apierrors.IsNotFound(err))

			if !updateConflict {
				assert.True(t, harvesterv1.UpgradeCompleted.IsFalse(upgrade))
				assert.Equal(t, StateFailed, upgrade.Labels[upgradeStateLabel])
				assert.Equal(t, serviceErr.Error(), harvesterv1.UpgradeCompleted.GetReason(upgrade))
				return
			}

			// Retry with the persisted Upgrade, not the failed update's in-memory copy.
			require.True(t, harvesterv1.UpgradeCompleted.IsUnknown(upgrade))
			_, err = repo.h.OnChanged(upgrade.Name, upgrade)
			require.NoError(t, err)
			upgrade, err = clientset.HarvesterhciV1beta1().Upgrades(upgradeNamespace).Get(context.Background(), upgrade.Name, metav1.GetOptions{})
			require.NoError(t, err)
			assert.Equal(t, StatePreparingRepo, upgrade.Labels[upgradeStateLabel])
			assert.True(t, harvesterv1.RepoProvisioned.IsUnknown(upgrade))
			assert.True(t, harvesterv1.UpgradeCompleted.IsUnknown(upgrade))
			_, err = repo.h.serviceClient.Get(upgradeNamespace, repo.getRepoServiceName(), metav1.GetOptions{})
			require.NoError(t, err)
			assert.Equal(t, 2, serviceAttempts)
		})
	}
}
