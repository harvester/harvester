package node

import (
	"fmt"
	"testing"

	"github.com/rancher/wrangler/v3/pkg/webhook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	batchv1 "k8s.io/api/batch/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"

	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/harvester/harvester/pkg/webhook/config"
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

func TestValidateCPUManagerOperation(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	validator := &nodeValidator{
		nodeCache: fakeclients.NodeCache(clientset.CoreV1().Nodes),
		jobCache:  fakeclients.JobCache(clientset.BatchV1().Jobs),
		vmiCache:  fakeclients.VirtualMachineInstanceCache(clientset.KubevirtV1().VirtualMachineInstances),
	}

	assert.Equal(t, werror.NewBadRequest("Failed to retrieve cpu-manager-update-status from annotation: invalid policy"),
		validator.validateCPUManagerOperation(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "node1",
				Annotations: map[string]string{util.AnnotationCPUManagerUpdateStatus: `{"status": "running", "policy": "foo"}`},
			},
		}))

	assert.Equal(t, werror.NewBadRequest("The witness node is unable to update the CPU manager policy."),
		validator.validateCPUManagerOperation(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "node1",
				Annotations: map[string]string{util.AnnotationCPUManagerUpdateStatus: `{"status": "requested", "policy": "static"}`},
				Labels:      map[string]string{util.HarvesterWitnessNodeLabelKey: "true"},
			},
		}))

	assert.Nil(t, validator.validateCPUManagerOperation(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1",
		},
	}))

	assert.Nil(t, validator.validateCPUManagerOperation(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "node1",
			Annotations: map[string]string{util.AnnotationCPUManagerUpdateStatus: `{"status": "running", "policy": "static"}`},
		},
	}))
}

func TestCheckCPUManagerLabel(t *testing.T) {
	nodeWithLabels := func(labels map[string]string) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
			},
		}
	}

	testCases := []struct {
		name   string
		policy ctlnode.CPUManagerPolicy
		node   *corev1.Node
		errMsg string
	}{
		{
			name:   "invalid update: invalid nil label",
			policy: ctlnode.CPUManagerNonePolicy,
			node:   &corev1.Node{},
			errMsg: "label cpumanager not found",
		},
		{
			name:   "invalid update: invalid label value",
			policy: ctlnode.CPUManagerNonePolicy,
			node:   nodeWithLabels(map[string]string{kubevirtv1.CPUManager: ""}),
			errMsg: "label cpumanager not found",
		},
		{
			name:   "invalid update: the CPU manager policy remains unchanged, both are static",
			policy: ctlnode.CPUManagerStaticPolicy,
			node:   nodeWithLabels(map[string]string{kubevirtv1.CPUManager: "true"}),
			errMsg: "current cpu manager policy is already the same as requested value: static",
		},
		{
			name:   "invalid update: the CPU manager policy remains unchanged, both are none",
			policy: ctlnode.CPUManagerNonePolicy,
			node:   nodeWithLabels(map[string]string{kubevirtv1.CPUManager: "false"}),
			errMsg: "current cpu manager policy is already the same as requested value: none",
		},
		{
			name:   "valid update: the CPU manager policy changed",
			policy: ctlnode.CPUManagerStaticPolicy,
			node:   nodeWithLabels(map[string]string{kubevirtv1.CPUManager: "false"}),
			errMsg: "",
		},
		{
			name:   "valid update: the CPU manager policy changed",
			policy: ctlnode.CPUManagerNonePolicy,
			node:   nodeWithLabels(map[string]string{kubevirtv1.CPUManager: "true"}),
			errMsg: "",
		},
	}
	for _, tc := range testCases {
		err := checkCPUManagerLabel(tc.node, tc.policy)
		if tc.errMsg != "" {
			assert.NotNil(t, err, tc.name)
			assert.Equal(t, tc.errMsg, err.Error(), tc.name)
		} else {
			assert.Nil(t, err, tc.name)
		}
	}
}

func TestCheckCPUManagerJobs(t *testing.T) {
	node0 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-0",
		},
	}
	testCases := []struct {
		name   string
		node   *corev1.Node
		jobs   []*batchv1.Job
		errMsg string
	}{
		{
			name:   "valid update: empty jobs",
			node:   node0,
			jobs:   []*batchv1.Job{},
			errMsg: "",
		},
		{
			name: "invalid update: one cpumanager running jobs on node-0",
			node: node0,
			jobs: []*batchv1.Job{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "job-1",
						Labels: map[string]string{
							util.LabelCPUManagerUpdateNode: "node-0",
						},
					},
				},
			},
			errMsg: "there is other job job-1 updating the cpu manager policy for this node node-0",
		},
		{
			name: "valid update: no cpumanager running jobs on node-0",
			node: node0,
			jobs: []*batchv1.Job{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "job1",
						Labels: map[string]string{
							util.LabelCPUManagerUpdateNode: "node-0",
						},
					},
					Status: batchv1.JobStatus{
						Conditions: []batchv1.JobCondition{
							{
								Type:   batchv1.JobComplete,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "job2",
						Labels: map[string]string{
							util.LabelCPUManagerUpdateNode: "node-0",
						},
					},
					Status: batchv1.JobStatus{
						Conditions: []batchv1.JobCondition{
							{
								Type:   batchv1.JobFailed,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "job3",
						Labels: map[string]string{
							util.LabelCPUManagerUpdateNode: "node-1",
						},
					},
				},
			},
			errMsg: "",
		},
	}
	for _, tc := range testCases {
		clientset := fake.NewSimpleClientset()
		for _, job := range tc.jobs {
			err := clientset.Tracker().Add(job)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}
		jobCache := fakeclients.JobCache(clientset.BatchV1().Jobs)
		err := checkCurrentNodeCPUManagerJobs(tc.node, jobCache)
		if tc.errMsg != "" {
			assert.NotNil(t, err, tc.name)
			assert.Equal(t, tc.errMsg, err.Error(), tc.name)
		} else {
			assert.Nil(t, err, tc.name)
		}
	}
}

func TestCheckMasterNodeJobs(t *testing.T) {
	masterNode0 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-0",
			Labels: map[string]string{
				util.KubeMasterNodeLabelKey: "true",
			},
		},
	}
	masterNode1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			Labels: map[string]string{
				util.KubeMasterNodeLabelKey: "true",
			},
		},
	}
	workerNode1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
	}
	testCases := []struct {
		name        string
		currentNode *corev1.Node
		nodes       []*corev1.Node
		jobs        []*batchv1.Job
		errMsg      string
	}{
		{
			name: "valid update: node-0 not a master node",
			currentNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-0",
				},
			},
			nodes:  []*corev1.Node{},
			jobs:   []*batchv1.Job{},
			errMsg: "",
		},
		{
			name:        "valid update: only one master node",
			currentNode: masterNode0,
			nodes:       []*corev1.Node{masterNode0},
			jobs:        []*batchv1.Job{},
			errMsg:      "",
		},
		{
			name:        "valid update: no other master node update",
			currentNode: masterNode0,
			nodes:       []*corev1.Node{masterNode0, workerNode1},
			jobs: []*batchv1.Job{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							util.LabelCPUManagerUpdateNode: "node-1",
						},
					},
				},
			},
			errMsg: "",
		},
		{
			name:        "valid update: no other master node update",
			currentNode: masterNode0,
			nodes:       []*corev1.Node{masterNode0, masterNode1},
			jobs:        []*batchv1.Job{},
			errMsg:      "",
		},
		{
			name:        "invalid update: other master also update cpu manager",
			currentNode: masterNode0,
			nodes:       []*corev1.Node{masterNode0, masterNode1},
			jobs: []*batchv1.Job{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "job-1",
						Labels: map[string]string{
							util.LabelCPUManagerUpdateNode: "node-1",
						},
					},
				},
			},
			errMsg: "the node you are trying to update the cpu manager policy is a master node, and only one master node can be updated at a time, while job job-1 is updating the policy for other master nodes",
		},
	}
	for _, tc := range testCases {
		clientset := fake.NewSimpleClientset()
		for _, node := range tc.nodes {
			err := clientset.Tracker().Add(node)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}
		for _, job := range tc.jobs {
			err := clientset.Tracker().Add(job)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}
		nodeCache := fakeclients.NodeCache(clientset.CoreV1().Nodes)
		jobCache := fakeclients.JobCache(clientset.BatchV1().Jobs)
		err := checkMasterNodeJobs(tc.currentNode, nodeCache, jobCache)
		if tc.errMsg != "" {
			assert.NotNil(t, err, tc.name)
			assert.Equal(t, tc.errMsg, err.Error(), tc.name)
		} else {
			assert.Nil(t, err, tc.name)
		}
	}
}

func TestCheckCPUPinningVMIs(t *testing.T) {
	node0 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-0",
		},
	}
	testCases := []struct {
		name   string
		node   *corev1.Node
		policy ctlnode.CPUManagerPolicy
		vmis   []*kubevirtv1.VirtualMachineInstance
		errMsg string
	}{
		{
			name:   "valid udpate: no need to do validation when update policy to static",
			node:   node0,
			policy: ctlnode.CPUManagerStaticPolicy,
			vmis:   []*kubevirtv1.VirtualMachineInstance{},
			errMsg: "",
		},
		{
			name:   "valid udpate: no cpu pinning enabled vm in node-0",
			node:   node0,
			policy: ctlnode.CPUManagerNonePolicy,
			vmis: []*kubevirtv1.VirtualMachineInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vm-0",
						Labels: map[string]string{
							util.LabelNodeNameKey: "node-0",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vm-1",
						Labels: map[string]string{
							util.LabelNodeNameKey: "node-1",
						},
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							CPU: &kubevirtv1.CPU{
								DedicatedCPUPlacement: true,
							},
						},
					},
				},
			},
			errMsg: "",
		},
		{
			name:   "invalid udpate: there is a cpu pinning enabled vm in node-0",
			node:   node0,
			policy: ctlnode.CPUManagerNonePolicy,
			vmis: []*kubevirtv1.VirtualMachineInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vm-0",
						Labels: map[string]string{
							util.LabelNodeNameKey: "node-0",
						},
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							CPU: &kubevirtv1.CPU{
								DedicatedCPUPlacement: true,
							},
						},
					},
				},
			},
			errMsg: "there should not be any running VMs with CPU pinning when disabling the CPU manager",
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		for _, vmi := range tc.vmis {
			err := clientset.Tracker().Add(vmi)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}
		vmiCache := fakeclients.VirtualMachineInstanceCache(clientset.KubevirtV1().VirtualMachineInstances)
		err := checkCPUPinningVMIs(tc.node, tc.policy, vmiCache)
		if tc.errMsg != "" {
			assert.NotNil(t, err, tc.name)
			assert.Equal(t, tc.errMsg, err.Error(), tc.name)
		} else {
			assert.Nil(t, err, tc.name)
		}
	}
}

func Test_validateWitnessRoleChange(t *testing.T) {
	tests := []struct {
		name        string
		oldNode     *corev1.Node
		newNode     *corev1.Node
		expectedErr bool
	}{
		{
			name: "user should not be able to remove witness node taint from a node",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Labels: map[string]string{
						util.HarvesterWitnessNodeLabelKey: "true",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "node-role.kubernetes.io/etcd",
							Value:  "true",
							Effect: corev1.TaintEffectNoExecute,
						},
					},
				},
			},
			newNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Labels: map[string]string{
						util.HarvesterWitnessNodeLabelKey: "true",
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "user should not be able to remove witness node label from a node",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Labels: map[string]string{
						util.HarvesterWitnessNodeLabelKey: "true",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "node-role.kubernetes.io/etcd",
							Value:  "true",
							Effect: corev1.TaintEffectNoExecute,
						},
					},
				},
			},
			newNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "node-role.kubernetes.io/etcd",
							Value:  "true",
							Effect: corev1.TaintEffectNoExecute,
						},
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "user should be able to add and remove unrelated label/taint from a node",
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Labels: map[string]string{
						util.HarvesterWitnessNodeLabelKey: "true",
						"oldLabel":                        "oldValue",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "node-role.kubernetes.io/etcd",
							Value:  "true",
							Effect: corev1.TaintEffectNoExecute,
						},
						{
							Key:    "oldkey",
							Value:  "oldValue",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			newNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Labels: map[string]string{
						util.HarvesterWitnessNodeLabelKey: "true",
						"newLabel":                        "newValue",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "node-role.kubernetes.io/etcd",
							Value:  "true",
							Effect: corev1.TaintEffectNoExecute,
						},
						{
							Key:    "newKey",
							Value:  "newValue",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			expectedErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWitnessRoleChange(tt.oldNode, tt.newNode)
			if tt.expectedErr {
				assert.NotNil(t, err, tt.name)
			} else {
				assert.Nil(t, err, tt.name)
			}
		})
	}
}

func TestValidateNodeCreateDuringUpgrade(t *testing.T) {
	type testCase struct {
		name          string
		node          *corev1.Node
		existingNodes []*corev1.Node
		upgrades      []*harvesterv1.Upgrade
		isController  bool
		expectedError bool
		errorContains string
	}

	testCases := []testCase{
		{
			name: "allow node creation when in-progress upgrade has no latest label",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgrade",
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
			},
			isController:  false,
			expectedError: false,
		},
		{
			name: "allow node creation when in-progress upgrade latest label is false",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgrade",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "false",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
			},
			isController:  false,
			expectedError: false,
		},
		{
			name: "allow node creation when latest upgrade override is true and non-latest has no override",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "latest-upgrade",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "true",
						},
						Annotations: map[string]string{
							util.AnnotationAllowNodeJoin: "true",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "old-upgrade",
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
			},
			isController:  false,
			expectedError: false,
		},
		{
			name: "allow node creation when no upgrade is active",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades:      []*harvesterv1.Upgrade{},
			isController:  false,
			expectedError: false,
		},
		{
			name: "allow node creation when upgrade is completed (True)",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgrade",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "true",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "True",
							},
						},
					},
				},
			},
			isController:  false,
			expectedError: false,
		},
		{
			name: "allow node creation when upgrade failed (False)",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgrade",
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "False",
							},
						},
					},
				},
			},
			isController:  false,
			expectedError: false,
		},
		{
			name: "block node creation when upgrade is active (Unknown status)",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgrade",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "true",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
			},
			isController:  false,
			expectedError: true,
			errorContains: "because the upgrade \"harvester-system/test-upgrade\" is currently in progress.",
		},
		{
			name: "allow node creation when upgrade is active but override annotation is set",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgrade",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "true",
						},
						Annotations: map[string]string{
							util.AnnotationAllowNodeJoin: "true",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
			},
			isController:  false,
			expectedError: false,
		},
		{
			name: "allow node creation when override annotation is set to TRUE (uppercase)",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgrade",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "true",
						},
						Annotations: map[string]string{
							util.AnnotationAllowNodeJoin: "TRUE",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
			},
			isController:  false,
			expectedError: false,
		},
		{
			name: "allow node creation when override annotation is set to 1",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgrade",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "true",
						},
						Annotations: map[string]string{
							util.AnnotationAllowNodeJoin: "1",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
			},
			isController:  false,
			expectedError: false,
		},
		{
			name: "allow node creation when override annotation is set to t",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgrade",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "true",
						},
						Annotations: map[string]string{
							util.AnnotationAllowNodeJoin: "t",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
			},
			isController:  false,
			expectedError: false,
		},
		{
			name: "block node creation when override annotation is set to false",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgrade",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "true",
						},
						Annotations: map[string]string{
							util.AnnotationAllowNodeJoin: "false",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
			},
			isController:  false,
			expectedError: true,
			errorContains: "because the upgrade \"harvester-system/test-upgrade\" is currently in progress.",
		},
		{
			name: "block node creation when override annotation has invalid value",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgrade",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "true",
						},
						Annotations: map[string]string{
							util.AnnotationAllowNodeJoin: "invalid",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
			},
			isController:  false,
			expectedError: true,
			errorContains: "and has an invalid value \"invalid\" for annotation",
		},
		{
			name: "allow node creation from controller during active upgrade",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgrade",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "true",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
			},
			isController:  true,
			expectedError: false,
		},
		{
			name: "allow create on already-registered node during active upgrade",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "existing-node",
				},
			},
			existingNodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "existing-node",
					},
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-upgrade",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "true",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
			},
			isController:  false,
			expectedError: false,
		},
		{
			name: "allow node creation when multiple latest upgrades exist and first allows override",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "upgrade-1",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "true",
						},
						Annotations: map[string]string{
							util.AnnotationAllowNodeJoin: "true",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "upgrade-2",
						Namespace: util.HarvesterSystemNamespaceName,
						// No annotation - should block
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
			},
			isController:  false,
			expectedError: false,
		},
		{
			name: "allow node creation when multiple upgrades active and first has allow override",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "upgrade-1",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "true",
						},
						Annotations: map[string]string{
							util.AnnotationAllowNodeJoin: "true",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "upgrade-2",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "true",
						},
						Annotations: map[string]string{
							util.AnnotationAllowNodeJoin: "1",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
			},
			isController:  false,
			expectedError: false,
		},
		{
			name: "block node creation when multiple latest upgrades exist and first denies override",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-node",
				},
			},
			upgrades: []*harvesterv1.Upgrade{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "upgrade-1",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "true",
						},
						Annotations: map[string]string{
							util.AnnotationAllowNodeJoin: "false",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "upgrade-2",
						Namespace: util.HarvesterSystemNamespaceName,
						Labels: map[string]string{
							util.LabelHarvesterLatestUpgrade: "true",
						},
						Annotations: map[string]string{
							util.AnnotationAllowNodeJoin: "true",
						},
					},
					Status: harvesterv1.UpgradeStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Completed",
								Status: "Unknown",
							},
						},
					},
				},
			},
			isController:  false,
			expectedError: true,
			errorContains: "because the upgrade \"harvester-system/upgrade-1\" is currently in progress.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()

			for _, node := range tc.existingNodes {
				err := clientset.Tracker().Add(node)
				assert.Nil(t, err)
			}

			for _, upgrade := range tc.upgrades {
				err := clientset.Tracker().Add(upgrade)
				assert.Nil(t, err)
			}

			validator := &nodeValidator{
				nodeCache:    fakeclients.NodeCache(clientset.CoreV1().Nodes),
				upgradeCache: fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades),
			}

			var req *types.Request
			if tc.isController {
				req = types.NewRequest(
					&webhook.Request{
						AdmissionRequest: admissionv1.AdmissionRequest{
							UserInfo: authenticationv1.UserInfo{
								Username: "system:serviceaccount:harvester-system:harvester",
							},
						},
					},
					&config.Options{
						HarvesterControllerUsername: "system:serviceaccount:harvester-system:harvester",
					},
				)
			} else {
				req = &types.Request{
					Request: &webhook.Request{},
				}
			}

			err := validator.Create(req, tc.node)

			if tc.expectedError {
				assert.NotNil(t, err, "expected error but got nil")
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				var admitErr werror.AdmitError
				require.ErrorAs(t, err, &admitErr)
				assert.Equal(t, metav1.StatusReasonConflict, admitErr.AsResult().Reason)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateNodeCreateDuringUpgradeCacheError(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	// Simulate an error when listing upgrades.
	clientset.PrependReactor("list", "upgrades", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("simulated cache error")
	})

	validator := &nodeValidator{
		nodeCache:    fakeclients.NodeCache(clientset.CoreV1().Nodes),
		upgradeCache: fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades),
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "new-node",
		},
	}

	req := &types.Request{
		Request: &webhook.Request{},
	}

	err := validator.Create(req, node)

	assert.Error(t, err, "expected error when upgrade cache fails")
	assert.Contains(t, err.Error(), "failed to list upgrades")
	assert.Contains(t, err.Error(), "simulated cache error")

	var admitErr werror.AdmitError
	require.ErrorAs(t, err, &admitErr)
	assert.Equal(t, metav1.StatusReasonInternalError, admitErr.AsResult().Reason)
}
