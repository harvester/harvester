package upgrade

import (
	"context"
	"testing"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

// newTestRepo builds a Repo backed by fake clients with the given objects.
func newTestRepo(upgrade *harvesterv1.Upgrade, objs ...k8sruntime.Object) *Repo {
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
