package catalog

import (
	"context"
	"testing"

	catalogv1 "github.com/rancher/rancher/pkg/apis/catalog.cattle.io/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corefake "k8s.io/client-go/kubernetes/fake"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_FetchAppImage(t *testing.T) {
	var (
		clientset     = fake.NewSimpleClientset()
		coreclientset = corefake.NewSimpleClientset()
		namespace     = "default"
	)

	if _, err := coreclientset.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}, metav1.CreateOptions{}); err != nil {
		assert.Nil(t, err, "failed to create namespace")
	}

	tests := []struct {
		desc              string
		name              string
		values            map[string]interface{}
		keys              []string
		expectedImageName string
		expectedError     bool
	}{
		{
			desc: "Normal Case with float type",
			name: "test",
			values: map[string]interface{}{
				"generalJob": map[string]interface{}{
					"image": map[string]interface{}{
						"repository":      "test-repository",
						"tag":             12.2,
						"imagePullPolicy": "IfNotPresent",
					},
				},
			},
			keys:              []string{"generalJob", "image"},
			expectedImageName: "test-repository:12.2",
			expectedError:     false,
		},
		{
			desc: "Normal Case with string type",
			name: "test2",
			values: map[string]interface{}{
				"generalJob": map[string]interface{}{
					"image": map[string]interface{}{
						"repository":      "test-repository",
						"tag":             "v12.20",
						"imagePullPolicy": "IfNotPresent",
					},
				},
			},
			keys:              []string{"generalJob", "image"},
			expectedImageName: "test-repository:v12.20",
			expectedError:     false,
		},
		{
			desc:              "Not Found Case",
			name:              "test3",
			values:            map[string]interface{}{},
			keys:              []string{"generalJob", "image"},
			expectedImageName: "",
			expectedError:     true,
		},
		{
			desc: "Weird Case which should not happen in general",
			name: "test4",
			values: map[string]interface{}{
				"generalJob": map[string]interface{}{
					"image": map[string]interface{}{
						"repository":      "",
						"tag":             "",
						"imagePullPolicy": "",
					},
				},
			},
			keys:              []string{"generalJob", "image"},
			expectedImageName: "",
			expectedError:     false,
		},
		{
			desc: "Weird Case 02 which should not happen in general",
			name: "test5",
			values: map[string]interface{}{
				"generalJob": map[string]interface{}{
					"image": map[string]interface{}{
						"repository":      "",
						"tag":             "v2",
						"imagePullPolicy": "",
					},
				},
			},
			keys:              []string{"generalJob", "image"},
			expectedImageName: "",
			expectedError:     false,
		},
		{
			desc: "Weird Case 03 which should not happen in general",
			name: "test6",
			values: map[string]interface{}{
				"generalJob": map[string]interface{}{
					"image": map[string]interface{}{
						"repository":      "test-repository",
						"tag":             "",
						"imagePullPolicy": "",
					},
				},
			},
			keys:              []string{"generalJob", "image"},
			expectedImageName: "",
			expectedError:     false,
		},
	}

	for _, test := range tests {

		if _, err := clientset.CatalogV1().Apps(namespace).Create(context.Background(), &catalogv1.App{
			ObjectMeta: metav1.ObjectMeta{
				Name:      test.name,
				Namespace: namespace,
			},
			Spec: catalogv1.ReleaseSpec{
				Chart: &catalogv1.Chart{
					Values: test.values,
				},
			},
		}, metav1.CreateOptions{}); err != nil {
			assert.Nil(t, err, "failed to create app", test.desc)
		}

		image, err := FetchAppChartImage(fakeclients.AppCache(clientset.CatalogV1().Apps), namespace, test.name, test.keys)
		if err != nil {
			if test.expectedError {
				assert.NotNil(t, err, "expectedImageName error", test.desc)
				continue
			}
			assert.Nil(t, err, "failed to fetch image", test.desc)
		}

		assert.Equal(t, test.expectedImageName, image.ImageName(), test.desc)
	}
}
