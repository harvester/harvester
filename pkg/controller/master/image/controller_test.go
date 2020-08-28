package image

import (
	"context"
	"testing"

	"github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/generated/clientset/versioned/fake"
	typesv1alpha1 "github.com/rancher/harvester/pkg/generated/clientset/versioned/typed/harvester.cattle.io/v1alpha1"
	ctrlapis "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
)

func TestImageHandler_OnChanged(t *testing.T) {
	type input struct {
		key   string
		image *v1alpha1.VirtualMachineImage
	}
	type output struct {
		image *v1alpha1.VirtualMachineImage
		err   error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "nil resource",
			given: input{
				key:   "",
				image: nil,
			},
			expected: output{
				image: nil,
				err:   nil,
			},
		},
		{
			name: "deleted resource",
			given: input{
				key: "image name",
				image: &v1alpha1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						DeletionTimestamp: &metav1.Time{},
					},
				},
			},
			expected: output{
				image: &v1alpha1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						DeletionTimestamp: &metav1.Time{},
					},
				},
				err: nil,
			},
		},
		{
			name: "status reset on url change",
			given: input{
				key: "image name",
				image: &v1alpha1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: v1alpha1.VirtualMachineImageSpec{
						DisplayName: "display-test",
						URL:         "https://test-2.com",
					},
					Status: v1alpha1.VirtualMachineImageStatus{
						AppliedURL: "https://test-1.com",
						Progress:   50,
						Conditions: []v1alpha1.Condition{
							{
								Type:   v1alpha1.ImageImported,
								Status: corev1.ConditionUnknown,
							},
						},
					},
				},
			},
			expected: output{
				image: &v1alpha1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: v1alpha1.VirtualMachineImageSpec{
						DisplayName: "display-test",
						URL:         "https://test-2.com",
					},
					Status: v1alpha1.VirtualMachineImageStatus{
						AppliedURL: "",
						Progress:   0,
						Conditions: []v1alpha1.Condition{
							{
								Type:   v1alpha1.ImageImported,
								Status: "",
							},
						},
					},
				},
				err: nil,
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		if tc.given.image != nil {
			err := clientset.Tracker().Add(tc.given.image)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}

		var handler = &Handler{
			images:     fakeVirtualMachineImageClient(clientset.HarvesterV1alpha1().VirtualMachineImages),
			imageCache: fakeVirtualMachineImageCache(clientset.HarvesterV1alpha1().VirtualMachineImages),
		}
		var actual output
		actual.image, actual.err = handler.OnImageChanged(tc.given.key, tc.given.image)
		if actual.image != nil {
			for i := range actual.image.Status.Conditions {
				actual.image.Status.Conditions[i].LastUpdateTime = ""
				actual.image.Status.Conditions[i].LastTransitionTime = ""
			}
		}

		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

type fakeVirtualMachineImageClient func(string) typesv1alpha1.VirtualMachineImageInterface

func (c fakeVirtualMachineImageClient) Create(image *v1alpha1.VirtualMachineImage) (*v1alpha1.VirtualMachineImage, error) {
	return c(image.Namespace).Create(context.TODO(), image, metav1.CreateOptions{})
}

func (c fakeVirtualMachineImageClient) Update(image *v1alpha1.VirtualMachineImage) (*v1alpha1.VirtualMachineImage, error) {
	return c(image.Namespace).Update(context.TODO(), image, metav1.UpdateOptions{})
}

func (c fakeVirtualMachineImageClient) UpdateStatus(image *v1alpha1.VirtualMachineImage) (*v1alpha1.VirtualMachineImage, error) {
	return c(image.Namespace).UpdateStatus(context.TODO(), image, metav1.UpdateOptions{})
}

func (c fakeVirtualMachineImageClient) Delete(namespace, name string, opts *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *opts)
}

func (c fakeVirtualMachineImageClient) Get(namespace, name string, opts metav1.GetOptions) (*v1alpha1.VirtualMachineImage, error) {
	return c(namespace).Get(context.TODO(), name, opts)
}

func (c fakeVirtualMachineImageClient) List(namespace string, opts metav1.ListOptions) (*v1alpha1.VirtualMachineImageList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c fakeVirtualMachineImageClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c fakeVirtualMachineImageClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.VirtualMachineImage, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type fakeVirtualMachineImageCache func(string) typesv1alpha1.VirtualMachineImageInterface

func (c fakeVirtualMachineImageCache) Get(namespace, name string) (*v1alpha1.VirtualMachineImage, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeVirtualMachineImageCache) List(namespace string, selector labels.Selector) (ret []*v1alpha1.VirtualMachineImage, err error) {
	panic("implement me")
}

func (c fakeVirtualMachineImageCache) AddIndexer(indexName string, indexer ctrlapis.VirtualMachineImageIndexer) {
	panic("implement me")
}

func (c fakeVirtualMachineImageCache) GetByIndex(indexName, key string) (ret []*v1alpha1.VirtualMachineImage, err error) {
	panic("implement me")
}
