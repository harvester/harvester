package setting

import (
	"context"
	"testing"

	catalogv1 "github.com/rancher/rancher/pkg/apis/catalog.cattle.io/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterhciv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_UpdateSupportBundleImage(t *testing.T) {
	var (
		clientset = fake.NewSimpleClientset()
		namespace = "default"
	)

	_, err := clientset.HarvesterhciV1beta1().Settings().Create(
		context.TODO(),
		&harvesterhciv1beta1.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name: settings.SupportBundleImageName,
			},
		},
		metav1.CreateOptions{},
	)
	assert.Nil(t, err, "failed to create setting")

	err = UpdateSupportBundleImage(
		fakeclients.HarvesterSettingClient(clientset.HarvesterhciV1beta1().Settings),
		fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		&catalogv1.App{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test.name",
				Namespace: namespace,
			},
			Spec: catalogv1.ReleaseSpec{
				Chart: &catalogv1.Chart{
					Values: map[string]interface{}{
						"support-bundle-kit": map[string]interface{}{
							"image": map[string]interface{}{
								"repository":      "test-repository",
								"tag":             "v3.3",
								"imagePullPolicy": "IfNotPresent",
							},
						},
					},
				},
			},
		},
	)
	assert.Nil(t, err, "failed to update setting")

	s, err := clientset.HarvesterhciV1beta1().Settings().Get(
		context.TODO(),
		settings.SupportBundleImageName,
		metav1.GetOptions{})

	assert.Nil(t, err, "failed to get setting")
	assert.Equal(t, "{\"repository\":\"test-repository\",\"tag\":\"v3.3\",\"imagePullPolicy\":\"IfNotPresent\"}", s.Default)

	// image tag is empty, do not update
	err = UpdateSupportBundleImage(
		fakeclients.HarvesterSettingClient(clientset.HarvesterhciV1beta1().Settings),
		fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		&catalogv1.App{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test.name",
				Namespace: namespace,
			},
			Spec: catalogv1.ReleaseSpec{
				Chart: &catalogv1.Chart{
					Values: map[string]interface{}{
						"support-bundle-kit": map[string]interface{}{
							"image": map[string]interface{}{
								"repository":      "",
								"tag":             "",
								"imagePullPolicy": "IfNotPresent",
							},
						},
					},
				},
			},
		},
	)
	assert.Nil(t, err, "failed to update setting")
	s, err = clientset.HarvesterhciV1beta1().Settings().Get(
		context.TODO(),
		settings.SupportBundleImageName,
		metav1.GetOptions{})

	assert.Nil(t, err, "failed to get setting")
	// keeps unchanged
	assert.Equal(t, "{\"repository\":\"test-repository\",\"tag\":\"v3.3\",\"imagePullPolicy\":\"IfNotPresent\"}", s.Default)

	// image map is empty, do not update
	err = UpdateSupportBundleImage(
		fakeclients.HarvesterSettingClient(clientset.HarvesterhciV1beta1().Settings),
		fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		&catalogv1.App{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test.name",
				Namespace: namespace,
			},
			Spec: catalogv1.ReleaseSpec{
				Chart: &catalogv1.Chart{
					Values: map[string]interface{}{
						"support-bundle-kit": map[string]interface{}{
							"image": map[string]interface{}{},
						},
					},
				},
			},
		},
	)
	assert.Nil(t, err, "failed to update setting")
	s, err = clientset.HarvesterhciV1beta1().Settings().Get(
		context.TODO(),
		settings.SupportBundleImageName,
		metav1.GetOptions{})

	assert.Nil(t, err, "failed to get setting")
	// keeps unchanged
	assert.Equal(t, "{\"repository\":\"test-repository\",\"tag\":\"v3.3\",\"imagePullPolicy\":\"IfNotPresent\"}", s.Default)

	// invalid key from app
	err = UpdateSupportBundleImage(
		fakeclients.HarvesterSettingClient(clientset.HarvesterhciV1beta1().Settings),
		fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		&catalogv1.App{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test.name",
				Namespace: namespace,
			},
			Spec: catalogv1.ReleaseSpec{
				Chart: &catalogv1.Chart{
					Values: map[string]interface{}{
						"support-bundle-kit-error-name": map[string]interface{}{
							"image": map[string]interface{}{},
						},
					},
				},
			},
		},
	)
	assert.Nil(t, err, "failed to update setting")
	s, err = clientset.HarvesterhciV1beta1().Settings().Get(
		context.TODO(),
		settings.SupportBundleImageName,
		metav1.GetOptions{})

	assert.Nil(t, err, "failed to get setting")
	// keeps unchanged
	assert.Equal(t, "{\"repository\":\"test-repository\",\"tag\":\"v3.3\",\"imagePullPolicy\":\"IfNotPresent\"}", s.Default)

	// empty chart from app
	err = UpdateSupportBundleImage(
		fakeclients.HarvesterSettingClient(clientset.HarvesterhciV1beta1().Settings),
		fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		&catalogv1.App{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test.name",
				Namespace: namespace,
			},
			Spec: catalogv1.ReleaseSpec{},
		},
	)
	assert.Nil(t, err, "failed to update setting")
	s, err = clientset.HarvesterhciV1beta1().Settings().Get(
		context.TODO(),
		settings.SupportBundleImageName,
		metav1.GetOptions{})

	assert.Nil(t, err, "failed to get setting")
	// keeps unchanged
	assert.Equal(t, "{\"repository\":\"test-repository\",\"tag\":\"v3.3\",\"imagePullPolicy\":\"IfNotPresent\"}", s.Default)
}
