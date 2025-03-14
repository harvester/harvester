package virtualmachine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/rancher/wrangler/v3/pkg/generic"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	k8sfakeclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	kubevirtv1 "kubevirt.io/api/core/v1"
	vmwatch "kubevirt.io/kubevirt/pkg/virt-controller/watch"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	virtualmachinetype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestSyncMacAddressAndControllerRevision(t *testing.T) {
	type input struct {
		key string
		vmi *kubevirtv1.VirtualMachineInstance
		vm  *kubevirtv1.VirtualMachine
		cr  *appsv1.ControllerRevision
	}
	type output struct {
		vmi     *kubevirtv1.VirtualMachineInstance
		vm      *kubevirtv1.VirtualMachine
		err     error
		vmMacs  map[string]string
		vmiMacs map[string]string
		crMacs  map[string]string
	}

	defaultNetwork := "default"
	secondNetwork := "data"
	vmiMacs := map[string]string{
		defaultNetwork: "00:00:00:00:00",
		secondNetwork:  "00:01:02:03:04",
	}
	vmMacs := map[string]string{
		defaultNetwork: "00:00:00:00:dd",
		secondNetwork:  "00:01:02:03:ee",
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
				cr:  nil,
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
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{},
				},
				cr: &appsv1.ControllerRevision{},
			},
			expected: output{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{},
				},
				err: nil,
			},
		},
		{
			name: "vmi has same mac when vm set related mac",
			given: input{
				key: "default/test",
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vm-uid",
						Generation: 1,
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Networks: []kubevirtv1.Network{
									{
										Name: defaultNetwork,
										NetworkSource: kubevirtv1.NetworkSource{
											Pod: &kubevirtv1.PodNetwork{},
										},
									},
									{
										Name: "data",
									},
								},
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Interfaces: []kubevirtv1.Interface{
											{
												Name:       defaultNetwork,
												MacAddress: vmMacs[defaultNetwork],
											},
											{
												Name:       secondNetwork,
												MacAddress: vmMacs[secondNetwork],
											},
										},
									},
								},
							},
						},
					},
					Status: kubevirtv1.VirtualMachineStatus{
						ObservedGeneration: 1,
					},
				},
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vmi-uid",
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
							{
								IP:   "172.16.0.100",
								MAC:  vmMacs[defaultNetwork],
								Name: defaultNetwork,
							},
							{
								IP:   "172.16.1.100",
								MAC:  vmMacs[secondNetwork],
								Name: secondNetwork,
							},
						},
						Phase:                      kubevirtv1.Running,
						VirtualMachineRevisionName: "test-random-id",
					},
				},
			},
			expected: output{
				err:     nil,
				vmiMacs: vmMacs,
				vmMacs:  vmMacs,
				crMacs:  vmMacs,
			},
		},
		{
			name: "save vmi mac address back to vm when vm does not set related mac",
			given: input{
				key: "default/test",
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vm-uid",
						Generation: 2, // simulate the mac is backfilled to vm
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Networks: []kubevirtv1.Network{
									{
										Name: defaultNetwork,
										NetworkSource: kubevirtv1.NetworkSource{
											Pod: &kubevirtv1.PodNetwork{},
										},
									},
									{
										Name: secondNetwork,
									},
								},
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Interfaces: []kubevirtv1.Interface{
											{
												Name: defaultNetwork,
											},
											{
												Name: secondNetwork,
											},
										},
									},
								},
							},
						},
					},
					Status: kubevirtv1.VirtualMachineStatus{
						ObservedGeneration: 1, // upstream will update the ObservedGeneration to be same as Generation
					},
				},
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vmi-uid",
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
							{
								IP:   "172.16.0.100",
								MAC:  vmiMacs[defaultNetwork],
								Name: defaultNetwork,
							},
							{
								IP:   "172.16.1.100",
								MAC:  vmiMacs[secondNetwork],
								Name: secondNetwork,
							},
						},
						Phase:                      kubevirtv1.Running,
						VirtualMachineRevisionName: "test-random-id",
					},
				},
				cr: &appsv1.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-random-id",
						Namespace: "default",
					},
				},
			},
			expected: output{
				err:     nil,
				vmiMacs: vmiMacs,
				vmMacs:  vmiMacs,
				crMacs:  vmiMacs,
			},
		},
		{
			name: "do not save vmi mac address back to vm when vm has set mac but vm has different mac",
			given: input{
				key: "default/test",
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vm-uid",
						Generation: 1,
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Networks: []kubevirtv1.Network{
									{
										Name: defaultNetwork,
										NetworkSource: kubevirtv1.NetworkSource{
											Pod: &kubevirtv1.PodNetwork{},
										},
									},
									{
										Name: secondNetwork,
									},
								},
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Interfaces: []kubevirtv1.Interface{
											{
												Name:       defaultNetwork,
												MacAddress: vmMacs[defaultNetwork],
											},
											{
												Name:       secondNetwork,
												MacAddress: vmMacs[secondNetwork],
											},
										},
									},
								},
							},
						},
					},
					Status: kubevirtv1.VirtualMachineStatus{
						ObservedGeneration: 1,
					},
				},
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vmi-uid",
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
							{
								IP:   "172.16.0.100",
								MAC:  vmiMacs[defaultNetwork],
								Name: defaultNetwork,
							},
							{
								IP:   "172.16.1.100",
								MAC:  vmiMacs[secondNetwork],
								Name: secondNetwork,
							},
						},
						Phase:                      kubevirtv1.Running,
						VirtualMachineRevisionName: "test-random-id",
					},
				},
				cr: &appsv1.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-random-id",
						Namespace: "default",
					},
				},
			},
			expected: output{
				err:     nil,
				vmiMacs: vmiMacs,
				vmMacs:  vmMacs,
				crMacs:  vmMacs,
			},
		},
	}

	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		var k8sfake = k8sfakeclient.NewSimpleClientset()
		if tc.given.vmi != nil {
			var err = clientset.Tracker().Add(tc.given.vmi)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}
		if tc.given.vm != nil {
			var err = clientset.Tracker().Add(tc.given.vm)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}

		if tc.given.cr != nil && tc.given.vm != nil {
			vmspec := tc.given.vm.Spec
			bts, err := patchVMRevisionViaVMSpec(&vmspec)
			assert.Nil(t, err, "expect no error trying to marshal vmspec")
			tc.given.cr.Data.Raw = bts

			err = k8sfake.Tracker().Add(tc.given.cr)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}

		var ctrl = &VMNetworkController{
			vmClient:      fakeVMClient(clientset.KubevirtV1().VirtualMachines),
			vmCache:       fakeVMCache(clientset.KubevirtV1().VirtualMachines),
			vmiController: fakeVMIClient(clientset.KubevirtV1().VirtualMachineInstances),
			crCache:       fakeclients.ControllerRevisionCache(k8sfake.AppsV1().ControllerRevisions),
			crClient:      fakeclients.ControllerRevisionClient(k8sfake.AppsV1().ControllerRevisions),
		}

		_, err := ctrl.SyncMacAddressAndControllerRevision(tc.given.key, tc.given.vmi)
		assert.Nil(t, err, "error during reconcile of SetDefaultNetworkMacAddress %v %s", err, tc.name)
		if tc.given.vmi != nil && tc.given.vm != nil && tc.given.vmi.DeletionTimestamp != nil && tc.given.vm.DeletionTimestamp != nil {
			vm, err := ctrl.vmClient.Get(tc.given.vm.Namespace, tc.given.vm.Name, metav1.GetOptions{})
			assert.Nil(t, err, "mock resource should get from fake VM controller, error: %v", err)
			vmi, err := ctrl.vmiController.Get(tc.given.vmi.Namespace, tc.given.vmi.Name, metav1.GetOptions{})
			assert.Nil(t, err, "mock resource should get from fake VMI controller, error: %v", err)
			// vm has expected MACs
			for _, vmIface := range vm.Spec.Template.Spec.Domain.Devices.Interfaces {
				mac := tc.expected.vmMacs[vmIface.Name]
				assert.Equal(t, vmIface.MacAddress, mac, "case %q", tc.name)
			}
			// vmi has expected MACs
			for _, vmiIface := range vmi.Status.Interfaces {
				mac := tc.expected.vmiMacs[vmiIface.Name]
				assert.Equal(t, vmiIface.MAC, mac, "case %q", tc.name)
			}
			// ControllerRevision has expected MACs
			crObj, err := ctrl.crClient.Get(tc.given.cr.Namespace, tc.given.cr.Name, metav1.GetOptions{})
			assert.Nil(t, err, "mock resource should get from fake ControllerRevisionClient, error: %v", err)
			revisionSpec := &vmwatch.VirtualMachineRevisionData{}
			err = json.Unmarshal(crObj.Data.Raw, revisionSpec)
			assert.Nil(t, err, "mock resource should be successfully json Unmarshaled, error: %v", err)
			for _, rcIface := range revisionSpec.Spec.Template.Spec.Domain.Devices.Interfaces {
				mac := tc.expected.crMacs[rcIface.Name]
				assert.Equal(t, rcIface.MacAddress, mac, "case %q", tc.name)
			}
		}
	}
}

type fakeVMClient func(string) virtualmachinetype.VirtualMachineInterface

func (c fakeVMClient) Create(vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	return c(vm.Namespace).Create(context.TODO(), vm, metav1.CreateOptions{})
}

func (c fakeVMClient) Update(vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	return c(vm.Namespace).Update(context.TODO(), vm, metav1.UpdateOptions{})
}

func (c fakeVMClient) UpdateStatus(*kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	panic("implement me")
}

func (c fakeVMClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c fakeVMClient) Get(namespace, name string, options metav1.GetOptions) (*kubevirtv1.VirtualMachine, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c fakeVMClient) List(namespace string, opts metav1.ListOptions) (*kubevirtv1.VirtualMachineList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c fakeVMClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c fakeVMClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *kubevirtv1.VirtualMachine, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

func (c fakeVMClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*kubevirtv1.VirtualMachine, *kubevirtv1.VirtualMachineList], error) {
	panic("implement me")
}

type fakeVMCache func(string) virtualmachinetype.VirtualMachineInterface

func (c fakeVMCache) Get(namespace, name string) (*kubevirtv1.VirtualMachine, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeVMCache) List(_ string, _ labels.Selector) ([]*kubevirtv1.VirtualMachine, error) {
	panic("implement me")
}

func (c fakeVMCache) AddIndexer(_ string, _ generic.Indexer[*kubevirtv1.VirtualMachine]) {
	panic("implement me")
}

func (c fakeVMCache) GetByIndex(_, _ string) ([]*kubevirtv1.VirtualMachine, error) {
	panic("implement me")
}

type fakeVMIClient func(string) virtualmachinetype.VirtualMachineInstanceInterface

func (c fakeVMIClient) Create(vm *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	return c(vm.Namespace).Create(context.TODO(), vm, metav1.CreateOptions{})
}

func (c fakeVMIClient) Update(vm *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	return c(vm.Namespace).Update(context.TODO(), vm, metav1.UpdateOptions{})
}

func (c fakeVMIClient) UpdateStatus(*kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	panic("implement me")
}

func (c fakeVMIClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c fakeVMIClient) Get(namespace, name string, options metav1.GetOptions) (*kubevirtv1.VirtualMachineInstance, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c fakeVMIClient) List(namespace string, opts metav1.ListOptions) (*kubevirtv1.VirtualMachineInstanceList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c fakeVMIClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c fakeVMIClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *kubevirtv1.VirtualMachineInstance, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

func (c fakeVMIClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*kubevirtv1.VirtualMachineInstance, *kubevirtv1.VirtualMachineInstanceList], error) {
	panic("implement me")
}

func (c fakeVMIClient) Informer() cache.SharedIndexInformer {
	panic("implement me")
}

func (c fakeVMIClient) GroupVersionKind() schema.GroupVersionKind {
	panic("implement me")
}

func (c fakeVMIClient) AddGenericHandler(_ context.Context, _ string, _ generic.Handler) {
	panic("implement me")
}

func (c fakeVMIClient) AddGenericRemoveHandler(_ context.Context, _ string, _ generic.Handler) {
	panic("implement me")
}

func (c fakeVMIClient) Updater() generic.Updater {
	panic("implement me")
}

func (c fakeVMIClient) OnChange(_ context.Context, _ string, _ generic.ObjectHandler[*kubevirtv1.VirtualMachineInstance]) {
	panic("implement me")
}

func (c fakeVMIClient) OnRemove(_ context.Context, _ string, _ generic.ObjectHandler[*kubevirtv1.VirtualMachineInstance]) {
	panic("implement me")
}

func (c fakeVMIClient) Enqueue(_, _ string) {
	panic("implement me")
}

func (c fakeVMIClient) EnqueueAfter(_, _ string, _ time.Duration) {
	// do nothing
}

func (c fakeVMIClient) Cache() generic.CacheInterface[*kubevirtv1.VirtualMachineInstance] {
	panic("implement me")
}
