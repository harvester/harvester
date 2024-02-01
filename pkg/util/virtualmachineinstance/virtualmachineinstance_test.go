package virtualmachineinstance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
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
