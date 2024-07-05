package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	batchv1 "k8s.io/api/batch/v1"

	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	werror "github.com/harvester/harvester/pkg/webhook/error"

	k8sfake "k8s.io/client-go/kubernetes/fake"
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
	k8sclientset := k8sfake.NewSimpleClientset()
	client := fake.NewSimpleClientset()

	validator := &nodeValidator{
		nodeCache: fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
		jobCache:  fakeclients.JobCache(k8sclientset.BatchV1().Jobs),
		vmiCache:  fakeclients.VirtualMachineInstanceCache(client.KubevirtV1().VirtualMachineInstances),
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
				Labels:      map[string]string{ctlnode.HarvesterWitnessNodeLabelKey: "true"},
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
		k8sclientset := k8sfake.NewSimpleClientset()
		for _, job := range tc.jobs {
			err := k8sclientset.Tracker().Add(job)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}
		jobCache := fakeclients.JobCache(k8sclientset.BatchV1().Jobs)
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
				ctlnode.KubeMasterNodeLabelKey: "true",
			},
		},
	}
	masterNode1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			Labels: map[string]string{
				ctlnode.KubeMasterNodeLabelKey: "true",
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
		k8sclientset := k8sfake.NewSimpleClientset()
		for _, node := range tc.nodes {
			err := k8sclientset.Tracker().Add(node)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}
		for _, job := range tc.jobs {
			err := k8sclientset.Tracker().Add(job)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}
		nodeCache := fakeclients.NodeCache(k8sclientset.CoreV1().Nodes)
		jobCache := fakeclients.JobCache(k8sclientset.BatchV1().Jobs)
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
