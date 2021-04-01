package vm

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	kubevirtapis "kubevirt.io/client-go/api/v1"

	"github.com/rancher/harvester/pkg/controller/master/migration"
	"github.com/rancher/harvester/pkg/generated/clientset/versioned/fake"
	kubevirttype "github.com/rancher/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
	kubevirtctrl "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/rancher/harvester/pkg/util"
)

func TestMigrateAction(t *testing.T) {
	type input struct {
		namespace  string
		name       string
		nodeName   string
		vmInstance *kubevirtapis.VirtualMachineInstance
	}
	type output struct {
		vmInstanceMigrations []*kubevirtapis.VirtualMachineInstanceMigration
		err                  error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "VMI not found",
			given: input{
				namespace:  "default",
				name:       "test",
				vmInstance: nil,
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtapis.VirtualMachineInstanceMigration{},
				err:                  apierrors.NewNotFound(kubevirtapis.Resource("virtualmachineinstances"), "test"),
			},
		},
		{
			name: "VMI is not running",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtapis.VirtualMachineInstanceStatus{
						Phase: kubevirtapis.Pending,
					},
				},
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtapis.VirtualMachineInstanceMigration{},
				err:                  errors.New("The VM is not in running state"),
			},
		},
		{
			name: "Migration is triggered",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtapis.VirtualMachineInstanceStatus{
						Phase: kubevirtapis.Running,
					},
				},
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtapis.VirtualMachineInstanceMigration{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:    "default",
							GenerateName: "test-",
						},
						Spec: kubevirtapis.VirtualMachineInstanceMigrationSpec{
							VMIName: "test",
						},
					},
				},
				err: nil,
			},
		},
	}

	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		if tc.given.vmInstance != nil {
			err := clientset.Tracker().Add(tc.given.vmInstance)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}

		var handler = &vmActionHandler{
			vmis:      fakeVirtualMachineInstanceClient(clientset.KubevirtV1().VirtualMachineInstances),
			vmiCache:  fakeVirtualMachineInstanceCache(clientset.KubevirtV1().VirtualMachineInstances),
			vmims:     fakeVirtualMachineInstanceMigrationClient(clientset.KubevirtV1().VirtualMachineInstanceMigrations),
			vmimCache: fakeVirtualMachineInstanceMigrationCache(clientset.KubevirtV1().VirtualMachineInstanceMigrations),
		}

		var actual output
		var err error
		actual.err = handler.migrate(context.Background(), tc.given.namespace, tc.given.name, tc.given.nodeName)
		actual.vmInstanceMigrations, err = handler.vmimCache.List(tc.given.namespace, labels.Everything())
		assert.Nil(t, err, "List should return no error")

		assert.Equal(t, tc.expected.vmInstanceMigrations, actual.vmInstanceMigrations, "case %q", tc.name)
		if tc.expected.err != nil && actual.err != nil {
			//errors from pkg/errors track stacks so we only compare the error string here
			assert.Equal(t, tc.expected.err.Error(), actual.err.Error(), "case %q", tc.name)
		} else {
			assert.Equal(t, tc.expected.err, actual.err, "case %q", tc.name)
		}
	}

}

func TestAbortMigrateAction(t *testing.T) {
	type input struct {
		namespace           string
		name                string
		vmInstance          *kubevirtapis.VirtualMachineInstance
		vmInstanceMigration *kubevirtapis.VirtualMachineInstanceMigration
	}
	type output struct {
		vmInstanceMigrations []*kubevirtapis.VirtualMachineInstanceMigration
		err                  error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "VMI migration is not started",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtapis.VirtualMachineInstanceStatus{
						Phase:          kubevirtapis.Pending,
						MigrationState: nil,
					},
				},
				vmInstanceMigration: nil,
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtapis.VirtualMachineInstanceMigration{},
				err:                  errors.New("The VM is not in migrating state"),
			},
		},
		{
			name: "VMI migration is completed",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtapis.VirtualMachineInstanceStatus{
						Phase: kubevirtapis.Pending,
						MigrationState: &kubevirtapis.VirtualMachineInstanceMigrationState{
							Completed: true,
						},
					},
				},
				vmInstanceMigration: &kubevirtapis.VirtualMachineInstanceMigration{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-migration",
					},
					Spec: kubevirtapis.VirtualMachineInstanceMigrationSpec{
						VMIName: "test",
					},
					Status: kubevirtapis.VirtualMachineInstanceMigrationStatus{
						Phase: kubevirtapis.MigrationSucceeded,
					},
				},
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtapis.VirtualMachineInstanceMigration{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "test-migration",
						},
						Spec: kubevirtapis.VirtualMachineInstanceMigrationSpec{
							VMIName: "test",
						},
						Status: kubevirtapis.VirtualMachineInstanceMigrationStatus{
							Phase: kubevirtapis.MigrationSucceeded,
						},
					},
				},
				err: errors.New("The VM is not in migrating state"),
			},
		},
		{
			name: "Abort VMI migration successfully",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							util.AnnotationMigrationTarget: "test-uid",
							util.AnnotationMigrationState:  migration.StateMigrating,
						},
					},
					Status: kubevirtapis.VirtualMachineInstanceStatus{
						Phase: kubevirtapis.Running,
						MigrationState: &kubevirtapis.VirtualMachineInstanceMigrationState{
							Completed:    false,
							MigrationUID: "test-uid",
						},
					},
				},
				vmInstanceMigration: &kubevirtapis.VirtualMachineInstanceMigration{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-migration",
						UID:       "test-uid",
					},
					Spec: kubevirtapis.VirtualMachineInstanceMigrationSpec{
						VMIName: "test",
					},
					Status: kubevirtapis.VirtualMachineInstanceMigrationStatus{
						Phase: kubevirtapis.MigrationRunning,
					},
				},
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtapis.VirtualMachineInstanceMigration{},
				err:                  nil,
			},
		},
	}

	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		if tc.given.vmInstance != nil {
			err := clientset.Tracker().Add(tc.given.vmInstance)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}
		if tc.given.vmInstanceMigration != nil {
			err := clientset.Tracker().Add(tc.given.vmInstanceMigration)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}

		var handler = &vmActionHandler{
			vmis:      fakeVirtualMachineInstanceClient(clientset.KubevirtV1().VirtualMachineInstances),
			vmiCache:  fakeVirtualMachineInstanceCache(clientset.KubevirtV1().VirtualMachineInstances),
			vmims:     fakeVirtualMachineInstanceMigrationClient(clientset.KubevirtV1().VirtualMachineInstanceMigrations),
			vmimCache: fakeVirtualMachineInstanceMigrationCache(clientset.KubevirtV1().VirtualMachineInstanceMigrations),
		}

		var actual output
		var err error
		actual.err = handler.abortMigration(tc.given.namespace, tc.given.name)
		actual.vmInstanceMigrations, err = handler.vmimCache.List(tc.given.namespace, labels.Everything())
		assert.Nil(t, err, "List should return no error")

		assert.Equal(t, tc.expected.vmInstanceMigrations, actual.vmInstanceMigrations, "case %q", tc.name)
		if tc.expected.err != nil && actual.err != nil {
			//errors from pkg/errors track stacks so we only compare the error string here
			assert.Equal(t, tc.expected.err.Error(), actual.err.Error(), "case %q", tc.name)
		} else {
			assert.Equal(t, tc.expected.err, actual.err, "case %q", tc.name)
		}
	}

}

type fakeVirtualMachineInstanceCache func(string) kubevirttype.VirtualMachineInstanceInterface

func (c fakeVirtualMachineInstanceCache) Get(namespace, name string) (*kubevirtapis.VirtualMachineInstance, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeVirtualMachineInstanceCache) List(namespace string, selector labels.Selector) ([]*kubevirtapis.VirtualMachineInstance, error) {
	panic("implement me")
}

func (c fakeVirtualMachineInstanceCache) AddIndexer(indexName string, indexer kubevirtctrl.VirtualMachineInstanceIndexer) {
	panic("implement me")
}

func (c fakeVirtualMachineInstanceCache) GetByIndex(indexName, key string) ([]*kubevirtapis.VirtualMachineInstance, error) {
	panic("implement me")
}

type fakeVirtualMachineInstanceClient func(string) kubevirttype.VirtualMachineInstanceInterface

func (c fakeVirtualMachineInstanceClient) Create(virtualMachine *kubevirtapis.VirtualMachineInstance) (*kubevirtapis.VirtualMachineInstance, error) {
	return c(virtualMachine.Namespace).Create(context.TODO(), virtualMachine, metav1.CreateOptions{})
}

func (c fakeVirtualMachineInstanceClient) Update(virtualMachine *kubevirtapis.VirtualMachineInstance) (*kubevirtapis.VirtualMachineInstance, error) {
	return c(virtualMachine.Namespace).Update(context.TODO(), virtualMachine, metav1.UpdateOptions{})
}

func (c fakeVirtualMachineInstanceClient) UpdateStatus(virtualMachine *kubevirtapis.VirtualMachineInstance) (*kubevirtapis.VirtualMachineInstance, error) {
	return c(virtualMachine.Namespace).UpdateStatus(context.TODO(), virtualMachine, metav1.UpdateOptions{})
}

func (c fakeVirtualMachineInstanceClient) Delete(namespace, name string, opts *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *opts)
}

func (c fakeVirtualMachineInstanceClient) Get(namespace, name string, opts metav1.GetOptions) (*kubevirtapis.VirtualMachineInstance, error) {
	return c(namespace).Get(context.TODO(), name, opts)
}

func (c fakeVirtualMachineInstanceClient) List(namespace string, opts metav1.ListOptions) (*kubevirtapis.VirtualMachineInstanceList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c fakeVirtualMachineInstanceClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c fakeVirtualMachineInstanceClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *kubevirtapis.VirtualMachineInstance, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type fakeVirtualMachineInstanceMigrationCache func(string) kubevirttype.VirtualMachineInstanceMigrationInterface

func (c fakeVirtualMachineInstanceMigrationCache) Get(namespace, name string) (*kubevirtapis.VirtualMachineInstanceMigration, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeVirtualMachineInstanceMigrationCache) List(namespace string, selector labels.Selector) ([]*kubevirtapis.VirtualMachineInstanceMigration, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*kubevirtapis.VirtualMachineInstanceMigration, 0, len(list.Items))
	for _, vmim := range list.Items {
		result = append(result, &vmim)
	}
	return result, err
}

func (c fakeVirtualMachineInstanceMigrationCache) AddIndexer(indexName string, indexer kubevirtctrl.VirtualMachineInstanceMigrationIndexer) {
	panic("implement me")
}

func (c fakeVirtualMachineInstanceMigrationCache) GetByIndex(indexName, key string) ([]*kubevirtapis.VirtualMachineInstanceMigration, error) {
	panic("implement me")
}

type fakeVirtualMachineInstanceMigrationClient func(string) kubevirttype.VirtualMachineInstanceMigrationInterface

func (c fakeVirtualMachineInstanceMigrationClient) Create(virtualMachine *kubevirtapis.VirtualMachineInstanceMigration) (*kubevirtapis.VirtualMachineInstanceMigration, error) {
	return c(virtualMachine.Namespace).Create(context.TODO(), virtualMachine, metav1.CreateOptions{})
}

func (c fakeVirtualMachineInstanceMigrationClient) Update(virtualMachine *kubevirtapis.VirtualMachineInstanceMigration) (*kubevirtapis.VirtualMachineInstanceMigration, error) {
	return c(virtualMachine.Namespace).Update(context.TODO(), virtualMachine, metav1.UpdateOptions{})
}

func (c fakeVirtualMachineInstanceMigrationClient) UpdateStatus(virtualMachine *kubevirtapis.VirtualMachineInstanceMigration) (*kubevirtapis.VirtualMachineInstanceMigration, error) {
	return c(virtualMachine.Namespace).UpdateStatus(context.TODO(), virtualMachine, metav1.UpdateOptions{})
}

func (c fakeVirtualMachineInstanceMigrationClient) Delete(namespace, name string, opts *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *opts)
}

func (c fakeVirtualMachineInstanceMigrationClient) Get(namespace, name string, opts metav1.GetOptions) (*kubevirtapis.VirtualMachineInstanceMigration, error) {
	return c(namespace).Get(context.TODO(), name, opts)
}

func (c fakeVirtualMachineInstanceMigrationClient) List(namespace string, opts metav1.ListOptions) (*kubevirtapis.VirtualMachineInstanceMigrationList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c fakeVirtualMachineInstanceMigrationClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c fakeVirtualMachineInstanceMigrationClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *kubevirtapis.VirtualMachineInstanceMigration, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}
