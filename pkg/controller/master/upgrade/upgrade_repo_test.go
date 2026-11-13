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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8stesting "k8s.io/client-go/testing"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func newTestUpgradeRepo() (*Repo, *fake.Clientset) {
	upgrade := newTestUpgradeBuilder().InitStatus().
		WithImage(testUpgradeImage).
		ImageIDStatus(fmt.Sprintf("%s/%s", upgradeNamespace, testUpgradeImage)).
		ImageReadyCondition(corev1.ConditionTrue, "", "").Build()
	upgrade.APIVersion = "harvesterhci.io/v1beta1"
	upgrade.Kind = "Upgrade"
	upgrade.UID = "current-upgrade-uid"
	clientset := fake.NewSimpleClientset(
		upgrade,
		newTestExistingVirtualMachineImage(upgradeNamespace, testUpgradeImage),
		newNodeBuilder(testNodeName).Build(),
	)
	handler := &upgradeHandler{
		ctx:              context.Background(),
		upgradeClient:    fakeclients.UpgradeClient(clientset.HarvesterhciV1beta1().Upgrades),
		nodeCache:        fakeclients.NodeCache(clientset.CoreV1().Nodes),
		vmCache:          fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
		vmImageCache:     fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
		deploymentClient: fakeclients.DeploymentClient(clientset.AppsV1().Deployments),
		serviceClient:    fakeclients.ServiceClient(clientset.CoreV1().Services),
		scClient:         fakeclients.StorageClassClient(clientset.StorageV1().StorageClasses),
		clusterCache:     fakeclients.ClusterCache(clientset.ProvisioningV1().Clusters),
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

func TestRepoBootstrapExistingMetadata(t *testing.T) {
	type testCase struct {
		name    string
		mutate  func(metav1.Object)
		wantErr string
	}
	testCases := []testCase{
		{
			name: "missing labels",
			mutate: func(obj metav1.Object) {
				obj.SetLabels(nil)
			},
			wantErr: "label",
		},
		{
			name: "wrong upgrade label",
			mutate: func(obj metav1.Object) {
				obj.GetLabels()[harvesterUpgradeLabel] = "another-upgrade"
			},
			wantErr: harvesterUpgradeLabel,
		},
		{
			name: "wrong component label",
			mutate: func(obj metav1.Object) {
				obj.GetLabels()[harvesterUpgradeComponentLabel] = "another-component"
			},
			wantErr: harvesterUpgradeComponentLabel,
		},
		{
			name: "missing owner",
			mutate: func(obj metav1.Object) {
				obj.SetOwnerReferences(nil)
			},
			wantErr: "owner reference",
		},
		{
			name: "same upgrade name with old UID",
			mutate: func(obj metav1.Object) {
				obj.GetOwnerReferences()[0].UID = "old-upgrade-uid"
			},
			wantErr: "owner reference",
		},
		{
			name: "wrong owner name",
			mutate: func(obj metav1.Object) {
				obj.GetOwnerReferences()[0].Name = "another-upgrade"
			},
			wantErr: "owner reference",
		},
		{
			name: "wrong owner kind",
			mutate: func(obj metav1.Object) {
				obj.GetOwnerReferences()[0].Kind = "VirtualMachineImage"
			},
			wantErr: "owner reference",
		},
		{
			name: "wrong owner API version",
			mutate: func(obj metav1.Object) {
				obj.GetOwnerReferences()[0].APIVersion = "another.example/v1"
			},
			wantErr: "owner reference",
		},
		{
			name: "additional metadata",
			mutate: func(obj metav1.Object) {
				obj.GetLabels()["example.com/extra"] = "value"
				obj.SetAnnotations(map[string]string{"example.com/extra": "value"})
				obj.SetOwnerReferences(append([]metav1.OwnerReference{{
					APIVersion: "v1", Kind: "ConfigMap", Name: "another-owner", UID: "another-uid",
				}}, obj.GetOwnerReferences()...))
			},
		},
	}
	for _, resource := range []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Version: "v1", Resource: "services"},
	} {
		cases := append([]testCase{}, testCases...)
		if resource.Resource == "services" {
			for _, value := range []string{"", "another-creator"} {
				cases = append(cases, testCase{
					name: "creator=" + value,
					mutate: func(obj metav1.Object) {
						delete(obj.GetLabels(), util.LabelVMCreator)
						if value != "" {
							obj.GetLabels()[util.LabelVMCreator] = value
						}
					},
					wantErr: util.LabelVMCreator,
				})
			}
		}
		for _, tc := range cases {
			t.Run(resource.Resource+"/"+tc.name, func(t *testing.T) {
				repo, clientset := newTestUpgradeRepo()
				require.NoError(t, repo.Bootstrap())
				obj, err := clientset.Tracker().Get(resource, upgradeNamespace, repo.GetDeploymentName())
				require.NoError(t, err)
				metadata, err := meta.Accessor(obj)
				require.NoError(t, err)
				tc.mutate(metadata)
				require.NoError(t, clientset.Tracker().Update(resource, obj, upgradeNamespace))

				err = repo.Bootstrap()
				if tc.wantErr == "" {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, tc.wantErr)
					assert.Contains(t, err.Error(), metadata.GetName())
					_, err = repo.h.OnChanged(repo.upgrade.Name, repo.upgrade)
					require.NoError(t, err)
					upgrade, err := clientset.HarvesterhciV1beta1().Upgrades(upgradeNamespace).Get(context.Background(), repo.upgrade.Name, metav1.GetOptions{})
					require.NoError(t, err)
					assert.True(t, harvesterv1.UpgradeCompleted.IsFalse(upgrade))
					assert.Empty(t, harvesterv1.RepoProvisioned.GetStatus(upgrade))

					// Model an already removed image so the next reconciliation can
					// exercise repo cleanup without the legacy image deletion path.
					require.NoError(t, clientset.HarvesterhciV1beta1().VirtualMachineImages(upgradeNamespace).
						Delete(context.Background(), testUpgradeImage, metav1.DeleteOptions{}))
					// Stop after Repo.Cleanup(), before unrelated cluster cleanup.
					clusterErr := errors.New("cluster cleanup unavailable")
					clientset.PrependReactor("get", "clusters", func(_ k8stesting.Action) (bool, runtime.Object, error) {
						return true, nil, clusterErr
					})
					_, err = repo.h.OnChanged(upgrade.Name, upgrade)
					require.ErrorIs(t, err, clusterErr)
					actual, err := clientset.Tracker().Get(resource, upgradeNamespace, metadata.GetName())
					if tc.wantErr == "owner reference" {
						require.NoError(t, err)
						assert.Equal(t, obj, actual)
					} else {
						// Incorrect labels do not prevent cleaning up our own resources.
						assert.True(t, apierrors.IsNotFound(err))
					}
					return
				}
				actual, err := clientset.Tracker().Get(resource, upgradeNamespace, metadata.GetName())
				require.NoError(t, err)
				assert.Equal(t, obj, actual)
			})
		}
	}
}

func TestRepoBootstrapGetFailure(t *testing.T) {
	for _, resource := range []string{"deployments", "services"} {
		t.Run(resource, func(t *testing.T) {
			repo, clientset := newTestUpgradeRepo()
			require.NoError(t, repo.Bootstrap())
			getErr := errors.New("failed to read existing resource")
			clientset.PrependReactor("get", resource, func(_ k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, getErr
			})
			require.ErrorIs(t, repo.Bootstrap(), getErr)
		})
	}
}

func TestRepoCleanupPreservesResourcesOwnedByAnotherUpgrade(t *testing.T) {
	repo, clientset := newTestUpgradeRepo()
	require.NoError(t, repo.Bootstrap())

	for _, resource := range []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Version: "v1", Resource: "services"},
	} {
		name := repo.GetDeploymentName()
		obj, err := clientset.Tracker().Get(resource, upgradeNamespace, name)
		require.NoError(t, err)
		metadata, err := meta.Accessor(obj)
		require.NoError(t, err)
		ownerReferences := metadata.GetOwnerReferences()
		ownerReferences[0].UID = "another-upgrade-uid"
		metadata.SetOwnerReferences(ownerReferences)
		require.NoError(t, clientset.Tracker().Update(resource, obj, upgradeNamespace))
	}

	require.NoError(t, repo.deleteDeployment())
	require.NoError(t, repo.deleteService())

	for _, resource := range []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Version: "v1", Resource: "services"},
	} {
		_, err := clientset.Tracker().Get(resource, upgradeNamespace, repo.GetDeploymentName())
		require.NoError(t, err)
	}
}

func TestRepoDeleteResource(t *testing.T) {
	for _, resource := range []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Version: "v1", Resource: "services"},
	} {
		for _, failure := range []string{"", "get", "delete"} {
			t.Run(resource.Resource+"/"+failure, func(t *testing.T) {
				repo, clientset := newTestUpgradeRepo()
				remove := repo.deleteDeployment
				if resource.Resource == "services" {
					remove = repo.deleteService
				}
				// Already absent resources need no cleanup.
				require.NoError(t, remove())
				require.NoError(t, repo.Bootstrap())
				obj, err := clientset.Tracker().Get(resource, upgradeNamespace, repo.GetDeploymentName())
				require.NoError(t, err)
				metadata, err := meta.Accessor(obj)
				require.NoError(t, err)
				uid := types.UID("repo-resource-uid")
				metadata.SetUID(uid)
				require.NoError(t, clientset.Tracker().Update(resource, obj, upgradeNamespace))

				injectedErr := errors.New("cleanup API failure")
				deleteCalled := false
				clientset.PrependReactor("delete", resource.Resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
					deleteCalled = true
					options := action.(k8stesting.DeleteAction).GetDeleteOptions()
					require.NotNil(t, options.Preconditions)
					require.NotNil(t, options.Preconditions.UID)
					assert.Equal(t, uid, *options.Preconditions.UID)
					if failure == "delete" {
						return true, nil, injectedErr
					}
					return false, nil, nil
				})
				if failure == "get" {
					clientset.PrependReactor("get", resource.Resource, func(_ k8stesting.Action) (bool, runtime.Object, error) {
						return true, nil, injectedErr
					})
				}
				err = remove()
				if failure != "" {
					require.ErrorIs(t, err, injectedErr)
					_, err = clientset.Tracker().Get(resource, upgradeNamespace, metadata.GetName())
					require.NoError(t, err)
				} else {
					require.NoError(t, err)
					_, err = clientset.Tracker().Get(resource, upgradeNamespace, metadata.GetName())
					require.True(t, apierrors.IsNotFound(err))
				}
				assert.Equal(t, failure != "get", deleteCalled)
			})
		}
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
