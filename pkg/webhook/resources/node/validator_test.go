package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
)

func TestValidateCordonAndMaintenanceMode(t *testing.T) {
	var testCases = []struct {
		name          string
		oldNode       *corev1.Node
		newNode       *corev1.Node
		nodeList      []*corev1.Node
		expectedError bool
	}{
		{
			name: "user can cordon a node when there is another available node",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
			},
			newNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: corev1.NodeSpec{
					Unschedulable: true,
				},
			},
			nodeList: []*corev1.Node{
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
			expectedError: false,
		},
		{
			name: "user can enable maintenance mode when there is another available node",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
			},
			newNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						ctlnode.MaintainStatusAnnotationKey: ctlnode.MaintainStatusRunning,
					},
				},
			},
			nodeList: []*corev1.Node{
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
			expectedError: false,
		},
		{
			name: "user cannot cordon a node when the other node is in maintenance mode",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
			},
			newNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: corev1.NodeSpec{
					Unschedulable: true,
				},
			},
			nodeList: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Annotations: map[string]string{
							ctlnode.MaintainStatusAnnotationKey: ctlnode.MaintainStatusComplete,
						},
					},
				},
			},
			expectedError: true,
		},
		{
			name: "user cannot cordon a node when the other node is unschedulable",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
			},
			newNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: corev1.NodeSpec{
					Unschedulable: true,
				},
			},
			nodeList: []*corev1.Node{
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
			expectedError: true,
		},
		{
			name: "user cannot enable maintenance mode when the other node is in maintenance mode",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
			},
			newNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						ctlnode.MaintainStatusAnnotationKey: ctlnode.MaintainStatusRunning,
					},
				},
			},
			nodeList: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Annotations: map[string]string{
							ctlnode.MaintainStatusAnnotationKey: ctlnode.MaintainStatusRunning,
						},
					},
				},
			},
			expectedError: true,
		},
		{
			name: "user cannot enable maintenance mode when the other node is unschedulable",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
			},
			newNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						ctlnode.MaintainStatusAnnotationKey: ctlnode.MaintainStatusRunning,
					},
				},
			},
			nodeList: []*corev1.Node{
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
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		err := validateCordonAndMaintenanceMode(tc.oldNode, tc.newNode, tc.nodeList)
		if tc.expectedError {
			assert.NotNil(t, err, tc.name)
		} else {
			assert.Nil(t, err, tc.name)
		}
	}
}
