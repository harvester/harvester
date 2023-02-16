package vm

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	corefake "k8s.io/client-go/kubernetes/fake"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/controller/master/migration"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	kubevirttype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
	kubevirtctrl "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestMigrateAction(t *testing.T) {
	type input struct {
		namespace  string
		name       string
		nodeName   string
		vmInstance *kubevirtv1.VirtualMachineInstance
	}
	type output struct {
		vmInstanceMigrations []*kubevirtv1.VirtualMachineInstanceMigration
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
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{},
				err:                  apierrors.NewNotFound(kubevirtv1.Resource("virtualmachineinstances"), "test"),
			},
		},
		{
			name: "VMI is not running",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Pending,
					},
				},
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{},
				err:                  errors.New("The VM is not in running state"),
			},
		},
		{
			name: "VMI's ready status is false",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Running,
						Conditions: []kubevirtv1.VirtualMachineInstanceCondition{
							{
								Type:   kubevirtv1.VirtualMachineInstanceReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{},
				err:                  errors.New("Can't migrate the VM, the VM is not in ready status"),
			},
		},
		{
			name: "VMI's ready status is unknown",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Running,
						Conditions: []kubevirtv1.VirtualMachineInstanceCondition{
							{
								Type:   kubevirtv1.VirtualMachineInstanceReady,
								Status: corev1.ConditionUnknown,
							},
						},
					},
				},
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{},
				err:                  errors.New("Can't migrate the VM, the VM is not in ready status"),
			},
		},
		{
			name: "Migration is triggered",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Running,
						Conditions: []kubevirtv1.VirtualMachineInstanceCondition{
							{
								Type:   kubevirtv1.VirtualMachineInstanceReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:    "default",
							GenerateName: "test-",
						},
						Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{
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
		vmInstance          *kubevirtv1.VirtualMachineInstance
		vmInstanceMigration *kubevirtv1.VirtualMachineInstanceMigration
	}
	type output struct {
		vmInstanceMigrations []*kubevirtv1.VirtualMachineInstanceMigration
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
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase:          kubevirtv1.Pending,
						MigrationState: nil,
					},
				},
				vmInstanceMigration: nil,
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{},
				err:                  errors.New("The VM is not in migrating state"),
			},
		},
		{
			name: "VMI migration is completed",
			given: input{
				namespace: "default",
				name:      "test",
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Pending,
						MigrationState: &kubevirtv1.VirtualMachineInstanceMigrationState{
							Completed: true,
						},
					},
				},
				vmInstanceMigration: &kubevirtv1.VirtualMachineInstanceMigration{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-migration",
					},
					Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{
						VMIName: "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceMigrationStatus{
						Phase: kubevirtv1.MigrationSucceeded,
					},
				},
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "test-migration",
						},
						Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{
							VMIName: "test",
						},
						Status: kubevirtv1.VirtualMachineInstanceMigrationStatus{
							Phase: kubevirtv1.MigrationSucceeded,
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
				vmInstance: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							util.AnnotationMigrationTarget: "test-uid",
							util.AnnotationMigrationState:  migration.StateMigrating,
						},
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Running,
						MigrationState: &kubevirtv1.VirtualMachineInstanceMigrationState{
							Completed:    false,
							MigrationUID: "test-uid",
						},
					},
				},
				vmInstanceMigration: &kubevirtv1.VirtualMachineInstanceMigration{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-migration",
						UID:       "test-uid",
					},
					Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{
						VMIName: "test",
					},
					Status: kubevirtv1.VirtualMachineInstanceMigrationStatus{
						Phase: kubevirtv1.MigrationRunning,
					},
				},
			},
			expected: output{
				vmInstanceMigrations: []*kubevirtv1.VirtualMachineInstanceMigration{},
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

func TestAddVolume(t *testing.T) {
	type input struct {
		namespace string
		name      string
		input     AddVolumeInput
		pvc       *corev1.PersistentVolumeClaim
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
			name: "Volume source not found",
			given: input{
				namespace: "default",
				name:      "test",
				input: AddVolumeInput{
					DiskName:         "disk",
					VolumeSourceName: "not-exist",
				},
			},
			expected: output{
				err: errors.New("persistentvolumeclaims \"not-exist\" not found"),
			},
		},
	}

	for _, tc := range testCases {
		var coreclientset = corefake.NewSimpleClientset()
		if tc.given.pvc != nil {
			err := coreclientset.Tracker().Add(tc.given.pvc)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}

		var handler = &vmActionHandler{
			pvcCache: fakeclients.PersistentVolumeClaimCache(coreclientset.CoreV1().PersistentVolumeClaims),
		}

		var actual output
		actual.err = handler.addVolume(context.Background(), tc.given.namespace, tc.given.namespace, tc.given.input)

		if tc.expected.err != nil && actual.err != nil {
			//errors from pkg/errors track stacks so we only compare the error string here
			assert.Equal(t, tc.expected.err.Error(), actual.err.Error(), "case %q", tc.name)
		} else {
			assert.Equal(t, tc.expected.err, actual.err, "case %q", tc.name)
		}
	}
}

func TestRemoveVolume(t *testing.T) {
	type input struct {
		namespace string
		name      string
		input     RemoveVolumeInput
		vm        *kubevirtv1.VirtualMachine
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
			name: "VM instance not found",
			given: input{
				namespace: "default",
				name:      "test",
				input: RemoveVolumeInput{
					DiskName: "test",
				},
			},
			expected: output{
				err: errors.New("virtualmachines.kubevirt.io \"test\" not found"),
			},
		},
		{
			name: "Hotplug disk not found",
			given: input{
				namespace: "default",
				name:      "test",
				input: RemoveVolumeInput{
					DiskName: "not-exist",
				},
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: nil,
							},
						},
					},
				},
			},
			expected: output{
				err: errors.New("Disk `not-exist` not found in virtual machine `default/test`"),
			},
		},
	}

	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		var coreclientset = corefake.NewSimpleClientset()
		if tc.given.vm != nil {
			err := clientset.Tracker().Add(tc.given.vm)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}

		var handler = &vmActionHandler{
			vmCache:  fakeVirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
			pvcCache: fakeclients.PersistentVolumeClaimCache(coreclientset.CoreV1().PersistentVolumeClaims),
		}

		var actual output
		actual.err = handler.removeVolume(context.Background(), tc.given.namespace, tc.given.name, tc.given.input)

		if tc.expected.err != nil && actual.err != nil {
			//errors from pkg/errors track stacks so we only compare the error string here
			assert.Equal(t, tc.expected.err.Error(), actual.err.Error(), "case %q", tc.name)
		} else {
			assert.Equal(t, tc.expected.err, actual.err, "case %q", tc.name)
		}
	}
}

type fakeVirtualMachineCache func(string) kubevirttype.VirtualMachineInterface

func (c fakeVirtualMachineCache) Get(namespace, name string) (*kubevirtv1.VirtualMachine, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeVirtualMachineCache) List(namespace string, selector labels.Selector) ([]*kubevirtv1.VirtualMachine, error) {
	panic("implement me")
}

func (c fakeVirtualMachineCache) AddIndexer(indexName string, indexer kubevirtctrl.VirtualMachineIndexer) {
	panic("implement me")
}

func (c fakeVirtualMachineCache) GetByIndex(indexName, key string) ([]*kubevirtv1.VirtualMachine, error) {
	panic("implement me")
}

type fakeVirtualMachineInstanceCache func(string) kubevirttype.VirtualMachineInstanceInterface

func (c fakeVirtualMachineInstanceCache) Get(namespace, name string) (*kubevirtv1.VirtualMachineInstance, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeVirtualMachineInstanceCache) List(namespace string, selector labels.Selector) ([]*kubevirtv1.VirtualMachineInstance, error) {
	panic("implement me")
}

func (c fakeVirtualMachineInstanceCache) AddIndexer(indexName string, indexer kubevirtctrl.VirtualMachineInstanceIndexer) {
	panic("implement me")
}

func (c fakeVirtualMachineInstanceCache) GetByIndex(indexName, key string) ([]*kubevirtv1.VirtualMachineInstance, error) {
	panic("implement me")
}

type fakeVirtualMachineInstanceClient func(string) kubevirttype.VirtualMachineInstanceInterface

func (c fakeVirtualMachineInstanceClient) Create(virtualMachine *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	return c(virtualMachine.Namespace).Create(context.TODO(), virtualMachine, metav1.CreateOptions{})
}

func (c fakeVirtualMachineInstanceClient) Update(virtualMachine *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	return c(virtualMachine.Namespace).Update(context.TODO(), virtualMachine, metav1.UpdateOptions{})
}

func (c fakeVirtualMachineInstanceClient) UpdateStatus(virtualMachine *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	return c(virtualMachine.Namespace).UpdateStatus(context.TODO(), virtualMachine, metav1.UpdateOptions{})
}

func (c fakeVirtualMachineInstanceClient) Delete(namespace, name string, opts *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *opts)
}

func (c fakeVirtualMachineInstanceClient) Get(namespace, name string, opts metav1.GetOptions) (*kubevirtv1.VirtualMachineInstance, error) {
	return c(namespace).Get(context.TODO(), name, opts)
}

func (c fakeVirtualMachineInstanceClient) List(namespace string, opts metav1.ListOptions) (*kubevirtv1.VirtualMachineInstanceList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c fakeVirtualMachineInstanceClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c fakeVirtualMachineInstanceClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *kubevirtv1.VirtualMachineInstance, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type fakeVirtualMachineInstanceMigrationCache func(string) kubevirttype.VirtualMachineInstanceMigrationInterface

func (c fakeVirtualMachineInstanceMigrationCache) Get(namespace, name string) (*kubevirtv1.VirtualMachineInstanceMigration, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeVirtualMachineInstanceMigrationCache) List(namespace string, selector labels.Selector) ([]*kubevirtv1.VirtualMachineInstanceMigration, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*kubevirtv1.VirtualMachineInstanceMigration, 0, len(list.Items))
	for _, vmim := range list.Items {
		result = append(result, &vmim)
	}
	return result, err
}

func (c fakeVirtualMachineInstanceMigrationCache) AddIndexer(indexName string, indexer kubevirtctrl.VirtualMachineInstanceMigrationIndexer) {
	panic("implement me")
}

func (c fakeVirtualMachineInstanceMigrationCache) GetByIndex(indexName, key string) ([]*kubevirtv1.VirtualMachineInstanceMigration, error) {
	panic("implement me")
}

type fakeVirtualMachineInstanceMigrationClient func(string) kubevirttype.VirtualMachineInstanceMigrationInterface

func (c fakeVirtualMachineInstanceMigrationClient) Create(virtualMachine *kubevirtv1.VirtualMachineInstanceMigration) (*kubevirtv1.VirtualMachineInstanceMigration, error) {
	return c(virtualMachine.Namespace).Create(context.TODO(), virtualMachine, metav1.CreateOptions{})
}

func (c fakeVirtualMachineInstanceMigrationClient) Update(virtualMachine *kubevirtv1.VirtualMachineInstanceMigration) (*kubevirtv1.VirtualMachineInstanceMigration, error) {
	return c(virtualMachine.Namespace).Update(context.TODO(), virtualMachine, metav1.UpdateOptions{})
}

func (c fakeVirtualMachineInstanceMigrationClient) UpdateStatus(virtualMachine *kubevirtv1.VirtualMachineInstanceMigration) (*kubevirtv1.VirtualMachineInstanceMigration, error) {
	return c(virtualMachine.Namespace).UpdateStatus(context.TODO(), virtualMachine, metav1.UpdateOptions{})
}

func (c fakeVirtualMachineInstanceMigrationClient) Delete(namespace, name string, opts *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *opts)
}

func (c fakeVirtualMachineInstanceMigrationClient) Get(namespace, name string, opts metav1.GetOptions) (*kubevirtv1.VirtualMachineInstanceMigration, error) {
	return c(namespace).Get(context.TODO(), name, opts)
}

func (c fakeVirtualMachineInstanceMigrationClient) List(namespace string, opts metav1.ListOptions) (*kubevirtv1.VirtualMachineInstanceMigrationList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c fakeVirtualMachineInstanceMigrationClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c fakeVirtualMachineInstanceMigrationClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *kubevirtv1.VirtualMachineInstanceMigration, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}
