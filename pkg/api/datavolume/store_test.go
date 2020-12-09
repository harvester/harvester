package datavolume

import (
	"context"
	"fmt"
	"testing"

	"github.com/rancher/harvester/pkg/ref"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kv1alpha3 "kubevirt.io/client-go/api/v1alpha3"
	cdiapis "kubevirt.io/containerized-data-importer/pkg/apis/core/v1beta1"

	"github.com/rancher/harvester/pkg/generated/clientset/versioned/fake"
	cditype "github.com/rancher/harvester/pkg/generated/clientset/versioned/typed/cdi.kubevirt.io/v1beta1"
	cdiv1beta1 "github.com/rancher/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
)

func TestDataVolumeDelete(t *testing.T) {
	type input struct {
		key        string
		dataVolume *cdiapis.DataVolume
	}
	type output struct {
		err error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "dataVolume not found",
			given: input{
				key: "dataVolume name",
				dataVolume: &cdiapis.DataVolume{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
				},
			},
			expected: output{
				err: fmt.Errorf("failed to get dv bar, datavolumes.cdi.kubevirt.io \"bar\" not found"),
			},
		},
		{
			name: "dataVolume test no delete because of owned",
			given: input{
				key: "dataVolume name",
				dataVolume: &cdiapis.DataVolume{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-no-delete-owned",
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: kv1alpha3.VirtualMachineGroupVersionKind.Kind,
								Name: "foo",
							},
						},
					},
				},
			},
			expected: output{
				err: fmt.Errorf("can not delete the volume test-no-delete-owned which is currently owned by these VMs: foo"),
			},
		},
		{
			name: "dataVolume test no delete because of attached",
			given: input{
				key: "dataVolume name",
				dataVolume: &cdiapis.DataVolume{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-no-delete-attached",
						Annotations: map[string]string{
							ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachine","refs":["default/foo"]}]`,
						},
					},
				},
			},
			expected: output{
				err: fmt.Errorf("can not delete the volume test-no-delete-attached which is currently attached by these VMs: default/foo"),
			},
		},
		{
			name: "dataVolume test delete",
			given: input{
				key: "dataVolume name",
				dataVolume: &cdiapis.DataVolume{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-no-delete",
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "delete",
								Name: "delete",
							},
						},
					},
				},
			},
			expected: output{
				err: nil,
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		if tc.given.dataVolume != nil {
			err := clientset.Tracker().Add(tc.given.dataVolume)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}

		var store = &dvStore{
			dvCache: fakeDataVolumeCache(clientset.CdiV1beta1().DataVolumes),
		}

		var actual output
		if tc.name == "dataVolume not found" {
			actual.err = store.canDelete("foo", "bar")
		} else {
			actual.err = store.canDelete(tc.given.dataVolume.Namespace, tc.given.dataVolume.Name)
		}
		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

type fakeDataVolumeCache func(string) cditype.DataVolumeInterface

func (c fakeDataVolumeCache) Get(namespace, name string) (*cdiapis.DataVolume, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeDataVolumeCache) List(namespace string, selector labels.Selector) ([]*cdiapis.DataVolume, error) {
	panic("implement me")
}

func (c fakeDataVolumeCache) AddIndexer(indexName string, indexer cdiv1beta1.DataVolumeIndexer) {
	panic("implement me")
}

func (c fakeDataVolumeCache) GetByIndex(indexName, key string) ([]*cdiapis.DataVolume, error) {
	panic("implement me")
}
