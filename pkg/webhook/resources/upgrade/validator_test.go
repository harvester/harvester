package upgrade

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/harvester/harvester/pkg/util"
)

const (
	node1    = "node1"
	node2    = "node2"
	machine1 = "machine1"
	machine2 = "machine2"
)

func Test_isNodeMachineMatching(t *testing.T) {
	tests := []struct {
		name        string
		nodes       []*corev1.Node
		machines    []*clusterv1.Machine
		expectError bool
		errorKey    string
	}{
		{
			name:        "no node was listed",
			nodes:       []*corev1.Node{},
			expectError: true,
			errorKey:    "no node was listed",
		},
		{
			name: "count mismatch 1 node 0 machine",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: node1,
					},
				},
			},
			expectError: true,
			errorKey:    "do not match",
		},
		{
			name: "count mismatch 1 node 2 machines",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: node1,
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: machine1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: machine2,
					},
				},
			},
			expectError: true,
			errorKey:    "do not match",
		},
		{
			name: "machine has empty NodeRef",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: node1,
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: machine1,
					},
				},
			},
			expectError: true,
			errorKey:    "empty NodeRef",
		},
		{
			name: "node has no labels",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: node1,
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: machine1,
					},
					Status: clusterv1.MachineStatus{
						NodeRef: &corev1.ObjectReference{
							Name: node1,
						},
					},
				},
			},
			expectError: true,
			errorKey:    "no labels",
		},
		{
			name: "node has empty label",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   node1,
						Labels: map[string]string{},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: machine1,
					},
					Status: clusterv1.MachineStatus{
						NodeRef: &corev1.ObjectReference{
							Name: node1,
						},
					},
				},
			},
			expectError: true,
			errorKey:    "no expected label",
		},
		{
			name: "node has no expected label (false value)",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: node1,
						Labels: map[string]string{
							util.HarvesterManagedNodeLabelKey: "false",
						},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: machine1,
					},
					Status: clusterv1.MachineStatus{
						NodeRef: &corev1.ObjectReference{
							Name: node1,
						},
					},
				},
			},
			expectError: true,
			errorKey:    "no expected label",
		},
		{
			name: "node has no nnnotations",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: node1,
						Labels: map[string]string{
							util.HarvesterManagedNodeLabelKey: "true",
						},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: machine1,
					},
					Status: clusterv1.MachineStatus{
						NodeRef: &corev1.ObjectReference{
							Name: node1,
						},
					},
				},
			},
			expectError: true,
			errorKey:    "no nnnotations",
		},
		{
			name: "node has empty nnnotations",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: node1,
						Labels: map[string]string{
							util.HarvesterManagedNodeLabelKey: "true",
						},
						Annotations: map[string]string{},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: machine1,
					},
					Status: clusterv1.MachineStatus{
						NodeRef: &corev1.ObjectReference{
							Name: node1,
						},
					},
				},
			},
			expectError: true,
			errorKey:    "no expected annotation",
		},
		{
			name: "node refers to none-existing machine",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: node1,
						Labels: map[string]string{
							util.HarvesterManagedNodeLabelKey: "true",
						},
						Annotations: map[string]string{
							clusterv1.MachineAnnotation: machine2,
						},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: machine1,
					},
					Status: clusterv1.MachineStatus{
						NodeRef: &corev1.ObjectReference{
							Name: node1,
						},
					},
				},
			},
			expectError: true,
			errorKey:    "machine does not exist",
		},
		{
			name: "node refers to machine, but machine refers to other node",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: node1,
						Labels: map[string]string{
							util.HarvesterManagedNodeLabelKey: "true",
						},
						Annotations: map[string]string{
							clusterv1.MachineAnnotation: machine1,
						},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: machine1,
					},
					Status: clusterv1.MachineStatus{
						NodeRef: &corev1.ObjectReference{
							Name: node2,
						},
					},
				},
			},
			expectError: true,
			errorKey:    "machine refers to another node",
		},
		{
			name: "two machines refer to same node",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: node1,
						Labels: map[string]string{
							util.HarvesterManagedNodeLabelKey: "true",
						},
						Annotations: map[string]string{
							clusterv1.MachineAnnotation: machine1,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: node2,
						Labels: map[string]string{
							util.HarvesterManagedNodeLabelKey: "true",
						},
						Annotations: map[string]string{
							clusterv1.MachineAnnotation: machine2,
						},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: machine1,
					},
					Status: clusterv1.MachineStatus{
						NodeRef: &corev1.ObjectReference{
							Name: node1,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: machine2,
					},
					Status: clusterv1.MachineStatus{
						NodeRef: &corev1.ObjectReference{
							Name: node1,
						},
					},
				},
			},
			expectError: true,
			errorKey:    "machine refers to another node",
		},
		{
			name: "good match",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: node1,
						Labels: map[string]string{
							util.HarvesterManagedNodeLabelKey: "true",
						},
						Annotations: map[string]string{
							clusterv1.MachineAnnotation: machine1,
						},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: machine1,
					},
					Status: clusterv1.MachineStatus{
						NodeRef: &corev1.ObjectReference{
							Name: node1,
						},
					},
				},
			},
			expectError: false,
			errorKey:    "",
		},
		{
			name: "dangling machine",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: node1,
						Labels: map[string]string{
							util.HarvesterManagedNodeLabelKey: "true",
						},
						Annotations: map[string]string{
							clusterv1.MachineAnnotation: machine1,
						},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: machine1,
					},
					Status: clusterv1.MachineStatus{
						NodeRef: &corev1.ObjectReference{
							Name: node1,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: machine2,
					},
					Status: clusterv1.MachineStatus{
						NodeRef: &corev1.ObjectReference{
							Name: node2,
						},
					},
				},
			},

			expectError: true,
			errorKey:    "do not match",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := isNodeMachineMatching(tc.nodes, tc.machines)
			if tc.expectError {
				assert.NotNil(t, err, tc.name)
				assert.True(t, strings.Contains(err.Error(), tc.errorKey), tc.name)
			} else {
				assert.Nil(t, err, tc.name)
			}
		})
	}
}
