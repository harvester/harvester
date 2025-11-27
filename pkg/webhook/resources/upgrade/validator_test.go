package upgrade

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvesterFake "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
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

func TestUpgradeValidator_validatePauseMapAnnotation(t *testing.T) {
	givenNodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-0",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
			},
		},
	}

	var testCases = []struct {
		name      string
		upgrade   *harvesterv1.Upgrade
		expectErr bool
	}{
		{
			name: "empty string",
			upgrade: &harvesterv1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						util.AnnotationNodeUpgradePauseMap: "",
					},
				},
			},
			expectErr: true,
		},
		{
			name: "bad json string",
			upgrade: &harvesterv1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						util.AnnotationNodeUpgradePauseMap: "{\"node-0\"}",
					},
				},
			},
			expectErr: true,
		},
		{
			name: "empty pause map",
			upgrade: &harvesterv1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						util.AnnotationNodeUpgradePauseMap: "{}",
					},
				},
			},
			expectErr: false,
		},
		{
			name: "unpause nodes in pause map",
			upgrade: &harvesterv1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						util.AnnotationNodeUpgradePauseMap: "{\"node-0\":\"unpause\",\"node-1\":\"pause\"}",
					},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid desire action for nodes in pause map",
			upgrade: &harvesterv1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						util.AnnotationNodeUpgradePauseMap: "{\"node-0\":\"pause\",\"node-1\":\"restart\"}",
					},
				},
			},
			expectErr: true,
		},
		{
			name: "nodes in pause map are a subset of cluster nodes",
			upgrade: &harvesterv1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						util.AnnotationNodeUpgradePauseMap: "{\"node-0\":\"pause\"}",
					},
				},
			},
			expectErr: false,
		},
		{
			name: "some nodes in pause map do not exist",
			upgrade: &harvesterv1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						util.AnnotationNodeUpgradePauseMap: "{\"node-0\":\"pause\",\"node-100\":\"pause\"}",
					},
				},
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var nodes []runtime.Object
			for _, node := range givenNodes {
				nodes = append(nodes, node)
			}
			k8sclientset := k8sfake.NewSimpleClientset(nodes...)
			validator := &upgradeValidator{
				nodes: fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
			}

			err := validator.validatePauseMapAnnotation(tc.upgrade)

			assert.Equal(t, tc.expectErr, err != nil, tc.name)
		})
	}
}

func getTestAddon(enabled bool) *harvesterv1.Addon {
	return &harvesterv1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "addon1",
		},
		Spec: harvesterv1.AddonSpec{
			Repo:          "repo1",
			Chart:         "chart1",
			Version:       "version1",
			Enabled:       enabled,
			ValuesContent: "sample",
		},
	}
}

func setAddonState(addon *harvesterv1.Addon, state harvesterv1.AddonState) {
	if addon == nil {
		return
	}
	addon.Status.Status = state
}

func Test_validateAddons(t *testing.T) {
	var testCases = []struct {
		name          string
		enabled       bool
		addonState    harvesterv1.AddonState
		expectedError bool
	}{
		{
			name:          "good: addon is enabled, and deployed",
			enabled:       true,
			expectedError: false,
			addonState:    harvesterv1.AddonDeployed,
		},
		{
			name:          "bad: addon is enabled, but with no state",
			enabled:       true,
			expectedError: true,
		},
		{
			name:          "bad: addon is enabled, but with enabling state",
			enabled:       true,
			expectedError: true,
			addonState:    harvesterv1.AddonEnabling,
		},
		{
			name:          "bad: addon is enabled, but with init state",
			enabled:       true,
			expectedError: true,
			addonState:    harvesterv1.AddonInitState,
		},
		{
			name:          "bad: addon is enabled, but with updating state",
			enabled:       true,
			expectedError: true,
			addonState:    harvesterv1.AddonUpdating,
		},
		{
			name:          "bad: addon is enabled, but with invalid state",
			enabled:       true,
			expectedError: true,
			addonState:    "invalid",
		},
		{
			name:          "good: addon is disabled, no state, it is from initial",
			enabled:       false,
			expectedError: false,
		},
		{
			name:          "good: addon is disabled, and with disabled state",
			enabled:       false,
			expectedError: false,
			addonState:    harvesterv1.AddonDisabled,
		},
		{
			name:          "bad: addon is disabled, but with disabling state",
			enabled:       false,
			expectedError: true,
			addonState:    harvesterv1.AddonDisabling,
		},
		{
			name:          "bad: addon is disabled, but with invalid state",
			enabled:       false,
			expectedError: true,
			addonState:    "invalid",
		},
	}

	for _, tc := range testCases {
		harvesterClientSet := harvesterFake.NewSimpleClientset()
		fakeAddonCache := fakeclients.AddonCache(harvesterClientSet.HarvesterhciV1beta1().Addons)

		validator := NewValidator(nil, fakeAddonCache, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "").(*upgradeValidator)
		addon := getTestAddon(tc.enabled)
		if tc.addonState != "" {
			setAddonState(addon, tc.addonState)
		}
		err := harvesterClientSet.Tracker().Add(addon)
		assert.Nil(t, err)

		err = validator.checkAddons()
		if tc.expectedError {
			assert.NotNil(t, err, tc.name)
		} else {
			assert.Nil(t, err, tc.name)
		}
	}
}
