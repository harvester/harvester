package virtualmachineinstance

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_GetNonLiveMigratableVMIs(t *testing.T) {
	var testCases = []struct {
		name   string
		nodes  []*corev1.Node
		vmis   []*kubevirtv1.VirtualMachineInstance
		output []string
		err    error
	}{
		{
			name: "single-node cluster",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
				},
			},
			vmis: []*kubevirtv1.VirtualMachineInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm1",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm2",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
					},
				},
			},
			output: []string{"default/vm1", "default/vm2"},
			err:    nil,
		},
		{
			name: "witness node",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							util.HarvesterWitnessNodeLabelKey: "true",
						},
					},
				},
			},
			vmis: []*kubevirtv1.VirtualMachineInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm1",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm2",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
					},
				},
			},
			output: []string{"default/vm1", "default/vm2"},
			err:    nil,
		},
		{
			name: "witness node and all live migratable",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							util.HarvesterWitnessNodeLabelKey: "true",
						},
					},
				},
			},
			vmis: []*kubevirtv1.VirtualMachineInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm1",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm2",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node2",
					},
				},
			},
			output: nil,
			err:    nil,
		},
		{
			name: "all live migratable",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
					},
				},
			},
			vmis: []*kubevirtv1.VirtualMachineInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm1",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm2",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node2",
					},
				},
			},
			output: nil,
			err:    nil,
		},
		{
			name: "vm1 non-live migratable",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"node-role.kubernetes.io/control-plane": "true",
						},
						Name: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
					},
				},
			},
			vmis: []*kubevirtv1.VirtualMachineInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm1",
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Affinity: &corev1.Affinity{
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "node-role.kubernetes.io/control-plane",
													Operator: corev1.NodeSelectorOpIn,
													Values: []string{
														"true",
													},
												},
											},
										},
									},
								},
							},
						},
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm2",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node2",
					},
				},
			},
			output: []string{"default/vm1"},
			err:    nil,
		},
		{
			name: "all live migratable but node2 unschedulable",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
					},
					Spec: corev1.NodeSpec{
						Unschedulable: true,
					},
				},
			},
			vmis: []*kubevirtv1.VirtualMachineInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm1",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm2",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node2",
					},
				},
			},
			output: []string{"default/vm1"},
			err:    nil,
		},
		{
			name: "vm2 has nodeSelector",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
					},
				},
			},
			vmis: []*kubevirtv1.VirtualMachineInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm1",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm2",
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						NodeSelector: map[string]string{
							"kubernetes.io/hostname": "node2",
						},
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node2",
					},
				},
			},
			output: []string{"default/vm2"},
			err:    nil,
		},
		{
			name: "all live migratable in 3-node cluster",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
					},
				},
			},
			vmis: []*kubevirtv1.VirtualMachineInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm1",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm2",
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node3",
					},
				},
			},
			output: nil,
			err:    nil,
		},
		{
			name: "vm1 in a cluster network consists of node2 & node3",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"network.harvesterhci.io/provider": "true",
						},
						Name: "node2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"network.harvesterhci.io/provider": "true",
						},
						Name: "node3",
					},
				},
			},
			vmis: []*kubevirtv1.VirtualMachineInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm1",
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Affinity: &corev1.Affinity{
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "network.harvesterhci.io/provider",
													Operator: corev1.NodeSelectorOpIn,
													Values: []string{
														"true",
													},
												},
											},
										},
									},
								},
							},
						},
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node2",
					},
				},
			},
			output: nil,
			err:    nil,
		},
		{
			name: "all vms non-live migratable",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"node-role.kubernetes.io/control-plane": "true",
						},
						Name: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"node-role.kubernetes.io/control-plane": "true",
							"network.harvesterhci.io/provider":      "true",
						},
						Name: "node2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"node-role.kubernetes.io/control-plane": "true",
							"network.harvesterhci.io/provider":      "true",
							"test-key":                              "test-value",
						},
						Name: "node3",
					},
					Spec: corev1.NodeSpec{
						Unschedulable: true,
					},
				},
			},
			vmis: []*kubevirtv1.VirtualMachineInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm1",
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Affinity: &corev1.Affinity{
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "network.harvesterhci.io/provider",
													Operator: corev1.NodeSelectorOpIn,
													Values: []string{
														"true",
													},
												},
											},
										},
									},
								},
							},
						},
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm2",
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							Devices: kubevirtv1.Devices{
								HostDevices: []kubevirtv1.HostDevice{},
							},
						},
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "vm3",
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Affinity: &corev1.Affinity{
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "network.harvesterhci.io/provider",
													Operator: corev1.NodeSelectorOpDoesNotExist,
												},
											},
										},
									},
								},
							},
						},
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						NodeName: "node1",
					},
				},
			},
			output: []string{"default/vm1", "default/vm2", "default/vm3"},
			err:    nil,
		},
	}

	for _, tc := range testCases {
		vmis, err := GetAllNonLiveMigratableVMINames(tc.vmis, tc.nodes)
		assert.Equal(t, tc.err, err, tc.name)
		assert.Equal(t, tc.output, vmis, tc.name)
	}
}

func Test_ListByNode(t *testing.T) {
	clientSet := fake.NewSimpleClientset()
	vmiCache := fakeclients.VirtualMachineInstanceCache(clientSet.KubevirtV1().VirtualMachineInstances)
	vmis := []*kubevirtv1.VirtualMachineInstance{
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test01",
				Labels: labels.Set{
					kubevirtv1.NodeNameLabel: "node1",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test02",
				Labels: labels.Set{
					kubevirtv1.NodeNameLabel: "node2",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test03",
				Labels: labels.Set{
					kubevirtv1.NodeNameLabel: "node1",
					"foo":                    "bar",
				},
			},
		},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node1",
			Namespace: "default",
		},
	}

	for _, vmi := range vmis {
		if _, err := clientSet.KubevirtV1().VirtualMachineInstances("default").Create(context.TODO(), vmi, metav1.CreateOptions{}); vmi != nil && err != nil {
			assert.Nil(t, err, "failed to create fake vmi", vmi.Name)
		}
	}

	req1, err := labels.NewRequirement("foo", selection.In, []string{"bar", "baz"})
	if err != nil {
		assert.Nil(t, err, "failed to create requirement")
	}

	req2, err := labels.NewRequirement("foo", selection.Exists, nil)
	if err != nil {
		assert.Nil(t, err, "failed to create requirement")
	}

	var testCases = []struct {
		selector      labels.Selector
		expectedCount int
	}{
		{
			selector:      labels.Everything(),
			expectedCount: 2,
		},
		{
			selector:      labels.Set{"foo": "bar"}.AsSelector(),
			expectedCount: 1,
		},
		{
			selector:      labels.NewSelector().Add(*req1),
			expectedCount: 1,
		},
		{
			selector:      labels.NewSelector().Add(*req2),
			expectedCount: 1,
		},
	}

	for _, tc := range testCases {
		res, err := ListByNode(node, tc.selector, vmiCache)
		assert.Len(t, res, tc.expectedCount)
		assert.NoError(t, err)
	}
}
