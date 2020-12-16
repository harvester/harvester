package virtualmachine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"kubevirt.io/client-go/api/v1alpha3"

	"github.com/rancher/harvester/pkg/generated/clientset/versioned/fake"
	virtualmachinetype "github.com/rancher/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1alpha3"
	v1alpha3ctl "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1alpha3"
)

func TestSetDefaultManagementNetworkMacAddress(t *testing.T) {
	type input struct {
		key string
		vmi *v1alpha3.VirtualMachineInstance
		vm  *v1alpha3.VirtualMachine
	}
	type output struct {
		vmi *v1alpha3.VirtualMachineInstance
		vm  *v1alpha3.VirtualMachine
		err error
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "ignore nil resource",
			given: input{
				key: "",
				vmi: nil,
			},
			expected: output{
				vmi: nil,
				err: nil,
			},
		},
		{
			name: "ignore deleted resource",
			given: input{
				key: "default/test",
				vmi: &v1alpha3.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: v1alpha3.VirtualMachineInstanceSpec{},
				},
			},
			expected: output{
				vmi: &v1alpha3.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: v1alpha3.VirtualMachineInstanceSpec{},
				},
				err: nil,
			},
		},
		{
			name: "set mac address",
			given: input{
				key: "default/test",
				vm: &v1alpha3.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
					},
					Spec: v1alpha3.VirtualMachineSpec{
						Template: &v1alpha3.VirtualMachineInstanceTemplateSpec{
							Spec: v1alpha3.VirtualMachineInstanceSpec{
								Networks: []v1alpha3.Network{
									{
										Name: "default",
										NetworkSource: v1alpha3.NetworkSource{
											Pod: &v1alpha3.PodNetwork{},
										},
									},
								},
								Domain: v1alpha3.DomainSpec{
									Devices: v1alpha3.Devices{
										Interfaces: []v1alpha3.Interface{
											{
												Name: "default",
											},
										},
									},
								},
							},
						},
					},
				},
				vmi: &v1alpha3.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vmi-uid",
					},
					Spec: v1alpha3.VirtualMachineInstanceSpec{},
					Status: v1alpha3.VirtualMachineInstanceStatus{
						Interfaces: []v1alpha3.VirtualMachineInstanceNetworkInterface{
							{
								IP:   "172.16.0.100",
								MAC:  "00:00:00:00:00",
								Name: "default",
							},
							{
								IP:   "172.16.0.101",
								MAC:  "00:01:02:03:04",
								Name: "nic-1",
							},
						},
						Phase: v1alpha3.Running,
					},
				},
			},
			expected: output{
				vmi: &v1alpha3.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vmi-uid",
					},
					Spec: v1alpha3.VirtualMachineInstanceSpec{},
					Status: v1alpha3.VirtualMachineInstanceStatus{
						Interfaces: []v1alpha3.VirtualMachineInstanceNetworkInterface{
							{
								IP:   "172.16.0.100",
								MAC:  "00:00:00:00:00",
								Name: "default",
							},
							{
								IP:   "172.16.0.101",
								MAC:  "00:01:02:03:04",
								Name: "nic-1",
							},
						},
						Phase: v1alpha3.Running,
					},
				},
				vm: &v1alpha3.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
					},
					Spec: v1alpha3.VirtualMachineSpec{
						Template: &v1alpha3.VirtualMachineInstanceTemplateSpec{
							Spec: v1alpha3.VirtualMachineInstanceSpec{
								Networks: []v1alpha3.Network{
									{
										Name: "default",
										NetworkSource: v1alpha3.NetworkSource{
											Pod: &v1alpha3.PodNetwork{},
										},
									},
								},
								Domain: v1alpha3.DomainSpec{
									Devices: v1alpha3.Devices{
										Interfaces: []v1alpha3.Interface{
											{
												Name:       "default",
												MacAddress: "00:00:00:00:00",
											},
											{
												Name:       "nic-1",
												MacAddress: "00:01:02:03:04",
											},
										},
									},
								},
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
		if tc.given.vmi != nil {
			var err = clientset.Tracker().Add(tc.given.vmi)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}
		if tc.given.vm != nil {
			var err = clientset.Tracker().Add(tc.given.vm)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}

		var ctrl = &VMNetworkController{
			vmClient:  fakeVMClient(clientset.KubevirtV1alpha3().VirtualMachines),
			vmCache:   fakeVMCache(clientset.KubevirtV1alpha3().VirtualMachines),
			vmiClient: fakeVMIClient(clientset.KubevirtV1alpha3().VirtualMachineInstances),
		}

		var actual output
		actual.vmi, actual.err = ctrl.SetDefaultNetworkMacAddress(tc.given.key, tc.given.vmi)
		if tc.given.vmi != nil && tc.given.vm != nil {
			actual.vm, actual.err = ctrl.vmClient.Get(tc.given.vm.Namespace, tc.given.vm.Name, metav1.GetOptions{})
			assert.Nil(t, actual.err, "mock resource should get from fake VM controller")
			for _, vmIface := range actual.vm.Spec.Template.Spec.Domain.Devices.Interfaces {
				for _, iface := range tc.given.vmi.Status.Interfaces {
					if iface.Name == vmIface.Name {
						assert.Equal(t, iface.MAC, vmIface.MacAddress)
					}
				}
			}
		}

		assert.Equal(t, tc.expected.vmi, actual.vmi, "case %q", tc.name)
	}
}

type fakeVMClient func(string) virtualmachinetype.VirtualMachineInterface

func (c fakeVMClient) Create(vm *v1alpha3.VirtualMachine) (*v1alpha3.VirtualMachine, error) {
	return c(vm.Namespace).Create(context.TODO(), vm, metav1.CreateOptions{})
}

func (c fakeVMClient) Update(vm *v1alpha3.VirtualMachine) (*v1alpha3.VirtualMachine, error) {
	return c(vm.Namespace).Update(context.TODO(), vm, metav1.UpdateOptions{})
}

func (c fakeVMClient) UpdateStatus(vm *v1alpha3.VirtualMachine) (*v1alpha3.VirtualMachine, error) {
	panic("implement me")
}

func (c fakeVMClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c fakeVMClient) Get(namespace, name string, options metav1.GetOptions) (*v1alpha3.VirtualMachine, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c fakeVMClient) List(namespace string, opts metav1.ListOptions) (*v1alpha3.VirtualMachineList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c fakeVMClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c fakeVMClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha3.VirtualMachine, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type fakeVMCache func(string) virtualmachinetype.VirtualMachineInterface

func (c fakeVMCache) Get(namespace, name string) (*v1alpha3.VirtualMachine, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeVMCache) List(namespace string, selector labels.Selector) ([]*v1alpha3.VirtualMachine, error) {
	panic("implement me")
}

func (c fakeVMCache) AddIndexer(indexName string, indexer v1alpha3ctl.VirtualMachineIndexer) {
	panic("implement me")
}

func (c fakeVMCache) GetByIndex(indexName, key string) ([]*v1alpha3.VirtualMachine, error) {
	panic("implement me")
}

type fakeVMIClient func(string) virtualmachinetype.VirtualMachineInstanceInterface

func (c fakeVMIClient) Create(vm *v1alpha3.VirtualMachineInstance) (*v1alpha3.VirtualMachineInstance, error) {
	return c(vm.Namespace).Create(context.TODO(), vm, metav1.CreateOptions{})
}

func (c fakeVMIClient) Update(vm *v1alpha3.VirtualMachineInstance) (*v1alpha3.VirtualMachineInstance, error) {
	return c(vm.Namespace).Update(context.TODO(), vm, metav1.UpdateOptions{})
}

func (c fakeVMIClient) UpdateStatus(vm *v1alpha3.VirtualMachineInstance) (*v1alpha3.VirtualMachineInstance, error) {
	panic("implement me")
}

func (c fakeVMIClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c fakeVMIClient) Get(namespace, name string, options metav1.GetOptions) (*v1alpha3.VirtualMachineInstance, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c fakeVMIClient) List(namespace string, opts metav1.ListOptions) (*v1alpha3.VirtualMachineInstanceList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c fakeVMIClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c fakeVMIClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha3.VirtualMachineInstance, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}
