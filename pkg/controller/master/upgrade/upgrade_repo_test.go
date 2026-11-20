package upgrade

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8stesting "k8s.io/client-go/testing"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

// newTestRepo builds a Repo backed by fake clients with the given objects.
func newTestRepo(upgrade *harvesterv1.Upgrade, objs ...runtime.Object) *Repo {
	clientset := fake.NewSimpleClientset(objs...)
	h := &upgradeHandler{
		namespace:        harvesterSystemNamespace,
		pvcClient:        fakeclients.PersistentVolumeClaimClient(clientset.CoreV1().PersistentVolumeClaims),
		vmImageClient:    fakeclients.VirtualMachineImageClient(clientset.HarvesterhciV1beta1().VirtualMachineImages),
		deploymentClient: fakeclients.DeploymentClient(clientset.AppsV1().Deployments),
	}
	return NewUpgradeRepo(context.Background(), upgrade, h)
}

func TestApplyISOMount(t *testing.T) {
	cases := []struct {
		name        string
		mode        corev1.PersistentVolumeMode
		wantDevices bool // true = Block path; false = Filesystem path
	}{
		{name: "Filesystem", mode: corev1.PersistentVolumeFilesystem, wantDevices: false},
		{name: "Block", mode: corev1.PersistentVolumeBlock, wantDevices: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := corev1.Container{Name: "test"}
			applyISOMount(&c, tc.mode)

			assert.NotEmpty(t, c.Command)
			assert.NotEmpty(t, c.Args)
			if tc.wantDevices {
				assert.NotEmpty(t, c.VolumeDevices, "Block mode must use VolumeDevices")
				assert.Empty(t, c.VolumeMounts, "Block mode must not use VolumeMounts")
			} else {
				assert.NotEmpty(t, c.VolumeMounts, "Filesystem mode must use VolumeMounts")
				assert.Empty(t, c.VolumeDevices, "Filesystem mode must not use VolumeDevices")
			}
		})
	}
}

func TestCreateDeployment(t *testing.T) {
	upgrade := &harvesterv1.Upgrade{
		ObjectMeta: metav1.ObjectMeta{Name: "test-upgrade", Namespace: harvesterSystemNamespace},
		Spec:       harvesterv1.UpgradeSpec{Version: "v1.0.0"},
	}

	fsMode := corev1.PersistentVolumeFilesystem
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "test-upgrade", Namespace: harvesterSystemNamespace},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeMode: &fsMode},
	}
	vmImage := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{Name: "test-upgrade", Namespace: harvesterSystemNamespace},
	}

	repo := newTestRepo(upgrade, pvc)
	deployment, err := repo.CreateDeployment(vmImage)

	require.NoError(t, err)
	assert.Equal(t, int32(1), *deployment.Spec.Replicas, "must be single replica for RWO")
	assert.Equal(t, appsv1.RecreateDeploymentStrategyType, deployment.Spec.Strategy.Type, "must use Recreate for RWO")
	assert.Nil(t, deployment.Spec.Template.Spec.Affinity, "affinity is dead code with Replicas=1")
}

func TestCreateDeployment_PVCNotFound(t *testing.T) {
	upgrade := &harvesterv1.Upgrade{
		ObjectMeta: metav1.ObjectMeta{Name: "test-upgrade", Namespace: harvesterSystemNamespace},
	}
	vmImage := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{Name: "test-upgrade", Namespace: harvesterSystemNamespace},
	}

	repo := newTestRepo(upgrade)
	_, err := repo.CreateDeployment(vmImage)
	assert.Error(t, err, "must fail when PVC does not exist")
}

func TestCreateImageFromISO_StorageClassAnnotation(t *testing.T) {
	cases := []struct {
		name       string
		annotation string
		wantSCName string
	}{
		{
			name:       "annotation sets TargetStorageClassName",
			annotation: "my-custom-sc",
			wantSCName: "my-custom-sc",
		},
		{
			name:       "no annotation leaves TargetStorageClassName empty (CDI uses default SC)",
			annotation: "",
			wantSCName: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upgrade := &harvesterv1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-upgrade",
					Namespace: harvesterSystemNamespace,
				},
				Spec: harvesterv1.UpgradeSpec{Version: "v1.0.0"},
			}
			if tc.annotation != "" {
				upgrade.Annotations = map[string]string{
					util.AnnotationUpgradeStorageClass: tc.annotation,
				}
			}

			repo := newTestRepo(upgrade)
			image, err := repo.CreateImageFromISO("http://example.com/harvester.iso", "")

			require.NoError(t, err)
			assert.Equal(t, tc.wantSCName, image.Spec.TargetStorageClassName)
			// CDI must decide access mode from the StorageClass; never force it here.
			assert.Empty(t, image.Spec.StorageClassParameters, "must not force access mode or volume mode")
		})
	}
}

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
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Namespace: upgradeNamespace, Name: testUpgradeImage}},
	)
	handler := &upgradeHandler{
		ctx:              context.Background(),
		upgradeClient:    fakeclients.UpgradeClient(clientset.HarvesterhciV1beta1().Upgrades),
		pvcClient:        fakeclients.PersistentVolumeClaimClient(clientset.CoreV1().PersistentVolumeClaims),
		vmCache:          fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
		vmImageCache:     fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
		vmImageClient:    fakeclients.VirtualMachineImageClient(clientset.HarvesterhciV1beta1().VirtualMachineImages),
		deploymentClient: fakeclients.DeploymentClient(clientset.AppsV1().Deployments),
		serviceClient:    fakeclients.ServiceClient(clientset.CoreV1().Services),
		scClient:         fakeclients.StorageClassClient(clientset.StorageV1().StorageClasses),
		clusterCache:     fakeclients.ClusterCache(clientset.ProvisioningV1().Clusters),
	}
	return NewUpgradeRepo(handler.ctx, upgrade, handler), clientset
}

func TestRepoBootstrap(t *testing.T) {
	const deploymentUID types.UID = "existing-deployment-uid"
	const serviceUID types.UID = "existing-service-uid"
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
				deployment, err := repo.CreateDeployment(image)
				require.NoError(t, err)
				deployment.UID = deploymentUID
				_, err = repo.h.deploymentClient.Update(deployment)
				require.NoError(t, err)
			}
			if tc.serviceExists {
				service, err := repo.createService()
				require.NoError(t, err)
				service.UID = serviceUID
				_, err = repo.h.serviceClient.Update(service)
				require.NoError(t, err)
			}

			require.NoError(t, repo.Bootstrap())
			deployment, err := repo.h.deploymentClient.Get(upgradeNamespace, repo.GetDeploymentName(), metav1.GetOptions{})
			require.NoError(t, err)
			service, err := repo.h.serviceClient.Get(upgradeNamespace, repo.getRepoServiceName(), metav1.GetOptions{})
			require.NoError(t, err)
			if tc.deploymentExists {
				assert.Equal(t, deploymentUID, deployment.UID)
			}
			if tc.serviceExists {
				assert.Equal(t, serviceUID, service.UID)
			}

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
		},
		{
			name: "wrong upgrade label",
			mutate: func(obj metav1.Object) {
				obj.GetLabels()[harvesterUpgradeLabel] = "another-upgrade"
			},
		},
		{
			name: "wrong component label",
			mutate: func(obj metav1.Object) {
				obj.GetLabels()[harvesterUpgradeComponentLabel] = "another-component"
			},
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
					assert.Equal(t, StatePending, upgrade.Labels[upgradeCleanupLabel])
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
				}
				actual, err := clientset.Tracker().Get(resource, upgradeNamespace, metadata.GetName())
				require.NoError(t, err)
				assert.Equal(t, obj, actual)
			})
		}
	}
}

func TestRepoDeploymentReadinessUsesUpgradeOwner(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mutate   func(*appsv1.Deployment)
		prepared bool
	}{
		{"missing labels", func(d *appsv1.Deployment) { d.Labels = nil }, true},
		{"wrong labels", func(d *appsv1.Deployment) {
			d.Labels[harvesterUpgradeLabel] = "another-upgrade"
			d.Labels[harvesterUpgradeComponentLabel] = "another-component"
		}, true},
		{"missing owner", func(d *appsv1.Deployment) { d.OwnerReferences = nil }, false},
		{"old upgrade UID", func(d *appsv1.Deployment) { d.OwnerReferences[0].UID = "old-uid" }, false},
		{"wrong owner kind", func(d *appsv1.Deployment) { d.OwnerReferences[0].Kind = "ConfigMap" }, false},
		{"wrong owner API version", func(d *appsv1.Deployment) { d.OwnerReferences[0].APIVersion = "v1" }, false},
		{"different deployment", func(d *appsv1.Deployment) { d.Name = "another-deployment" }, false},
		{"not available", func(d *appsv1.Deployment) { d.Status.Conditions = nil }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, clientset := newTestUpgradeRepo()
			upgrade, err := repo.h.OnChanged(repo.upgrade.Name, repo.upgrade)
			require.NoError(t, err)
			require.Equal(t, StatePreparingRepo, upgrade.Labels[upgradeStateLabel])

			deployment, err := repo.h.deploymentClient.Get(upgradeNamespace, repo.GetDeploymentName(), metav1.GetOptions{})
			require.NoError(t, err)
			deployment.Status.Conditions = []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}}
			tc.mutate(deployment)
			h := &deploymentHandler{
				upgradeClient: repo.h.upgradeClient,
				upgradeCache:  fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades),
			}
			_, err = h.OnChanged("", deployment)
			require.NoError(t, err)
			actual, err := h.upgradeCache.Get(upgrade.Namespace, upgrade.Name)
			require.NoError(t, err)
			if tc.prepared {
				assert.Equal(t, StateRepoPrepared, actual.Labels[upgradeStateLabel])
				assert.True(t, harvesterv1.RepoProvisioned.IsTrue(actual))
			} else {
				assert.Equal(t, upgrade, actual)
			}
		})
	}
}

func TestRepoBootstrapGetFailure(t *testing.T) {
	for _, resource := range []string{"deployments", "services"} {
		t.Run(resource, func(t *testing.T) {
			repo, clientset := newTestUpgradeRepo()
			require.NoError(t, repo.Bootstrap())
			getErr := errors.New("failed to read existing resource")
			failGet := true
			clientset.PrependReactor("get", resource, func(_ k8stesting.Action) (bool, runtime.Object, error) {
				if failGet {
					failGet = false
					return true, nil, getErr
				}
				return false, nil, nil
			})
			_, err := repo.h.OnChanged(repo.upgrade.Name, repo.upgrade)
			require.ErrorIs(t, err, errGetExistingRepoResource)
			require.ErrorIs(t, err, getErr)
			upgrade, err := clientset.HarvesterhciV1beta1().Upgrades(upgradeNamespace).Get(context.Background(), repo.upgrade.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, repo.upgrade, upgrade)

			upgrade, err = repo.h.OnChanged(upgrade.Name, upgrade)
			require.NoError(t, err)
			assert.Equal(t, StatePreparingRepo, upgrade.Labels[upgradeStateLabel])
		})
	}
}

func TestRepoBootstrapRetriesTerminatingResource(t *testing.T) {
	for _, resource := range []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Version: "v1", Resource: "services"},
	} {
		t.Run(resource.Resource, func(t *testing.T) {
			repo, clientset := newTestUpgradeRepo()
			require.NoError(t, repo.Bootstrap())
			name := repo.GetDeploymentName()
			tracker := clientset.Tracker()
			obj, err := tracker.Get(resource, upgradeNamespace, name)
			require.NoError(t, err)
			metadata, err := meta.Accessor(obj)
			require.NoError(t, err)
			now := metav1.Now()
			metadata.SetDeletionTimestamp(&now)
			require.NoError(t, tracker.Update(resource, obj, upgradeNamespace))

			_, err = repo.h.OnChanged(repo.upgrade.Name, repo.upgrade)
			require.ErrorIs(t, err, errRepoResourceTerminating)
			upgrade, err := clientset.HarvesterhciV1beta1().Upgrades(upgradeNamespace).Get(context.Background(), repo.upgrade.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, repo.upgrade, upgrade)

			require.NoError(t, tracker.Delete(resource, upgradeNamespace, name))
			upgrade, err = repo.h.OnChanged(upgrade.Name, upgrade)
			require.NoError(t, err)
			require.Equal(t, StatePreparingRepo, upgrade.Labels[upgradeStateLabel])
			recreated, err := tracker.Get(resource, upgradeNamespace, name)
			require.NoError(t, err)
			metadata, err = meta.Accessor(recreated)
			require.NoError(t, err)
			assert.Nil(t, metadata.GetDeletionTimestamp())
			assert.True(t, repo.hasUpgradeOwner(metadata))
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

func TestUpgradeClaimsUploadedISO(t *testing.T) {
	pvcOwner := metav1.OwnerReference{APIVersion: "v1", Kind: "PersistentVolumeClaim", Name: "vm-disk", UID: "vm-disk-uid"}
	for _, tc := range []struct {
		name      string
		namespace string
		marked    bool
		owners    []metav1.OwnerReference
		wantClaim bool
	}{
		{name: "upgrade ISO", namespace: upgradeNamespace, marked: true, wantClaim: true},
		{name: "ordinary image", namespace: upgradeNamespace},
		{name: "PVC-owned ISO", namespace: upgradeNamespace, marked: true, owners: []metav1.OwnerReference{pvcOwner}},
		{name: "ISO in another namespace", namespace: "other", marked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := newTestUpgradeRepo()
			image := newTestExistingVirtualMachineImage(tc.namespace, "uploaded-iso")
			image.OwnerReferences = tc.owners
			if tc.marked {
				image.Annotations = map[string]string{util.AnnotationUpgradeImage: "True"}
			}
			_, err := repo.h.vmImageClient.Create(image)
			require.NoError(t, err)
			upgrade := newTestUpgradeBuilder().InitStatus().Build()
			upgrade.TypeMeta = repo.upgrade.TypeMeta
			upgrade.UID = repo.upgrade.UID
			upgrade.Spec.Image = image.Namespace + "/" + image.Name
			upgrade, err = repo.h.OnChanged(upgrade.Name, upgrade)
			require.NoError(t, err)
			actual, err := repo.GetImage(upgrade.Status.ImageID)
			require.NoError(t, err)
			assert.Equal(t, upgrade.Name, actual.Labels[harvesterUpgradeLabel])
			if tc.wantClaim {
				require.Equal(t, []metav1.OwnerReference{upgradeReference(upgrade)}, actual.OwnerReferences)
			} else {
				require.Equal(t, tc.owners, actual.OwnerReferences)
			}
			repo.upgrade = upgrade
			require.NoError(t, repo.Cleanup())
			_, err = repo.GetImage(upgrade.Status.ImageID)
			if tc.wantClaim {
				require.True(t, apierrors.IsNotFound(err))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRepoCleanupImageAfterVMRemoval(t *testing.T) {
	t.Run("image without owner", func(t *testing.T) {
		repo, _ := newTestUpgradeRepo()
		image, err := repo.GetImage(repo.upgrade.Status.ImageID)
		require.NoError(t, err)
		require.Empty(t, image.OwnerReferences)
		require.NoError(t, repo.Cleanup())
		actual, err := repo.GetImage(repo.upgrade.Status.ImageID)
		require.NoError(t, err)
		assert.Equal(t, image, actual)
	})
	for _, handoff := range []bool{false, true} {
		t.Run(fmt.Sprintf("PVC handoff=%t", handoff), func(t *testing.T) {
			repo, clientset := newTestUpgradeRepo()
			image, err := repo.GetImage(repo.upgrade.Status.ImageID)
			require.NoError(t, err)
			image = image.DeepCopy()
			image.OwnerReferences = []metav1.OwnerReference{upgradeReference(repo.upgrade)}
			_, err = repo.h.vmImageClient.Update(image)
			require.NoError(t, err)

			if handoff {
				_, err := repo.h.pvcClient.Create(&corev1.PersistentVolumeClaim{
					TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolumeClaim"},
					ObjectMeta: metav1.ObjectMeta{Namespace: upgradeNamespace, Name: "vm-disk", UID: "vm-disk-uid"},
				})
				require.NoError(t, err)
				now := metav1.Now()
				vm := &kubevirtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{
					Name: repo.GetVMName(), Namespace: upgradeNamespace, DeletionTimestamp: &now,
					Annotations: map[string]string{util.RemovedPVCsAnnotationKey: "vm-disk"},
				}}
				vms := clientset.KubevirtV1().VirtualMachines(upgradeNamespace)
				_, err = vms.Create(context.Background(), vm, metav1.CreateOptions{})
				require.NoError(t, err)
				clientset.ClearActions()
				require.NoError(t, repo.deleteVM())
				for _, action := range clientset.Actions() {
					require.False(t, action.Matches("delete", "virtualmachines"))
				}
				require.NoError(t, vms.Delete(context.Background(), vm.Name, metav1.DeleteOptions{}))
			}

			// The repo VM is absent when cleanup runs again.
			require.NoError(t, repo.Cleanup())
			image, err = repo.GetImage(repo.upgrade.Status.ImageID)
			if handoff {
				require.NoError(t, err)
				require.Equal(t, []metav1.OwnerReference{{
					APIVersion: "v1", Kind: "PersistentVolumeClaim", Name: "vm-disk", UID: "vm-disk-uid",
				}}, image.OwnerReferences)
			} else {
				require.True(t, apierrors.IsNotFound(err))
			}
		})
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
				metadata.SetResourceVersion("42")
				require.NoError(t, clientset.Tracker().Update(resource, obj, upgradeNamespace))

				injectedErr := errors.New("cleanup API failure")
				deleteCalls := 0
				clientset.PrependReactor("delete", resource.Resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
					deleteCalls++
					options := action.(k8stesting.DeleteAction).GetDeleteOptions()
					require.NotNil(t, options.Preconditions)
					require.NotNil(t, options.Preconditions.UID)
					assert.Equal(t, uid, *options.Preconditions.UID)
					require.NotNil(t, options.Preconditions.ResourceVersion)
					assert.Equal(t, metadata.GetResourceVersion(), *options.Preconditions.ResourceVersion)
					if failure == "delete" {
						owners := metadata.GetOwnerReferences()
						owners[0].UID = "another-upgrade-uid"
						metadata.SetOwnerReferences(owners)
						metadata.SetResourceVersion("43")
						require.NoError(t, clientset.Tracker().Update(resource, obj, upgradeNamespace))
						if *options.Preconditions.ResourceVersion != metadata.GetResourceVersion() {
							return true, nil, apierrors.NewConflict(resource.GroupResource(), metadata.GetName(), injectedErr)
						}
					}
					return false, nil, nil
				})
				if failure == "get" {
					clientset.PrependReactor("get", resource.Resource, func(_ k8stesting.Action) (bool, runtime.Object, error) {
						return true, nil, injectedErr
					})
				}
				if failure == "" {
					now := metav1.Now()
					metadata.SetDeletionTimestamp(&now)
					require.NoError(t, clientset.Tracker().Update(resource, obj, upgradeNamespace))
					require.NoError(t, remove())
					require.Zero(t, deleteCalls)
					metadata.SetDeletionTimestamp(nil)
					require.NoError(t, clientset.Tracker().Update(resource, obj, upgradeNamespace))
				}
				err = remove()
				if failure != "" {
					if failure == "delete" {
						require.True(t, apierrors.IsConflict(err))
						require.NoError(t, remove())
						actual, err := clientset.Tracker().Get(resource, upgradeNamespace, metadata.GetName())
						require.NoError(t, err)
						require.Equal(t, obj, actual)
					} else {
						require.ErrorIs(t, err, injectedErr)
					}
					_, err = clientset.Tracker().Get(resource, upgradeNamespace, metadata.GetName())
					require.NoError(t, err)
				} else {
					require.NoError(t, err)
					_, err = clientset.Tracker().Get(resource, upgradeNamespace, metadata.GetName())
					require.True(t, apierrors.IsNotFound(err))
				}
				if failure == "get" {
					assert.Zero(t, deleteCalls)
				} else {
					assert.Equal(t, 1, deleteCalls)
				}
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

type fakeUpgradeHeartbeat struct {
	ctlharvesterv1.UpgradeController
}

func (fakeUpgradeHeartbeat) EnqueueAfter(string, string, time.Duration) {}

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
				assert.Equal(t, StatePending, upgrade.Labels[upgradeCleanupLabel])
				assert.Equal(t, serviceErr.Error(), harvesterv1.UpgradeCompleted.GetReason(upgrade))
				return
			}

			// The Deployment becomes available before the Upgrade retry records PreparingRepo.
			deployments := clientset.AppsV1().Deployments(upgradeNamespace)
			deployment, err := deployments.Get(context.Background(), repo.GetDeploymentName(), metav1.GetOptions{})
			require.NoError(t, err)
			deployment.Status.Conditions = []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}}
			deployment, err = deployments.UpdateStatus(context.Background(), deployment, metav1.UpdateOptions{})
			require.NoError(t, err)
			handler := &deploymentHandler{
				upgradeClient: repo.h.upgradeClient,
				upgradeCache:  fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades),
			}
			_, err = handler.OnChanged("", deployment)
			require.NoError(t, err)
			upgrade, err = handler.upgradeCache.Get(upgradeNamespace, upgrade.Name)
			require.NoError(t, err)
			require.Empty(t, harvesterv1.RepoProvisioned.GetStatus(upgrade))

			var queued []types.NamespacedName
			repo.h.enqueueDeployment = func(namespace, name string) {
				queued = append(queued, types.NamespacedName{Namespace: namespace, Name: name})
			}
			repo.h.upgradeController = fakeUpgradeHeartbeat{}
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
			require.Empty(t, queued)

			_, err = repo.h.OnChanged(upgrade.Name, upgrade)
			require.NoError(t, err)
			require.Equal(t, []types.NamespacedName{{Namespace: deployment.Namespace, Name: deployment.Name}}, queued)
			_, err = handler.OnChanged(queued[0].String(), deployment)
			require.NoError(t, err)
			upgrade, err = handler.upgradeCache.Get(upgradeNamespace, upgrade.Name)
			require.NoError(t, err)
			assert.Equal(t, StateRepoPrepared, upgrade.Labels[upgradeStateLabel])
			assert.True(t, harvesterv1.RepoProvisioned.IsTrue(upgrade))
		})
	}
}
