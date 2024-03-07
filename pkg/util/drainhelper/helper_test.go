package drainhelper

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/drain"

	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

var (
	cpNode1 = &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-cp-1",
			Labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "true",
			},
			Annotations: make(map[string]string),
		},
	}

	cpNode2 = &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-cp-2",
			Labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "true",
				"node-role.kubernetes.io/etcd":          "true",
			},
			Annotations: make(map[string]string),
		},
	}

	cpNode3 = &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-cp-3",
			Labels: map[string]string{
				"node-role.kubernetes.io/etcd": "true",
			},
			Annotations: make(map[string]string),
		},
	}

	testNode = &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-58rk8",
		},
	}
)

func Test_defaultDrainHelper(t *testing.T) {
	assert := require.New(t)
	cfg := &rest.Config{
		Host: "localhost",
	}
	dh, err := defaultDrainHelper(context.TODO(), cfg)
	assert.NoError(err, "expected no error during generation of node helper")
	assert.NotNil(dh, "expected to get a valid drain client")
	assert.True(dh.IgnoreAllDaemonSets, "expected drain helper to ignore daemonsets")
	assert.Equal(defaultGracePeriodSeconds, dh.GracePeriodSeconds, "expected grace period seconds to match predefined constant")
	assert.True(dh.DeleteEmptyDirData, "expected to skip deletion of empty data directory")
	assert.True(dh.Force, "expected force to be set")
	assert.Equal(defaultSkipPodLabels, dh.PodSelector, "expected drain handler pod labels to match const")
}

func Test_meetsControlPlaneRequirementsHA(t *testing.T) {
	assert := require.New(t)
	k8sclientset := k8sfake.NewSimpleClientset(testNode, cpNode1, cpNode2, cpNode3)

	nodeCache := fakeclients.NodeCache(k8sclientset.CoreV1().Nodes)
	err := DrainPossible(nodeCache, testNode)
	assert.NoError(err, "expected no error while checking testNode")

	err = DrainPossible(nodeCache, cpNode2)
	assert.NoError(err, "expected no error while checking cpNode2")
}

func Test_failsControlPlaneRequirementsHA(t *testing.T) {
	assert := require.New(t)
	k8sclientset := k8sfake.NewSimpleClientset(testNode, cpNode1, cpNode2, cpNode3)

	cpNode1.Annotations = map[string]string{
		ctlnode.MaintainStatusAnnotationKey: ctlnode.MaintainStatusRunning,
	}

	nodeCache := fakeclients.NodeCache(k8sclientset.CoreV1().Nodes)
	nodeClient := fakeclients.NodeClient(k8sclientset.CoreV1().Nodes)

	_, err := nodeClient.Update(cpNode1)
	assert.NoError(err, "expected no error while updating cpNode1")

	err = DrainPossible(nodeCache, cpNode2)
	assert.True(errors.Is(err, errHAControlPlaneNode), "expected error while checking cpNode2")
}

func Test_failsControlPlaneRequirementsSingleNode(t *testing.T) {
	assert := require.New(t)
	nodeObjects := []runtime.Object{testNode, cpNode2}

	k8sclientset := k8sfake.NewSimpleClientset(nodeObjects...)

	nodeCache := fakeclients.NodeCache(k8sclientset.CoreV1().Nodes)

	err := DrainPossible(nodeCache, cpNode1)
	assert.Error(err, "expected error while trying to place cpNode1 in maintenance mode")
	assert.True(errors.Is(err, errSingleControlPlaneNode), "expected error singleControlPlaneNodeError")
}

func Test_meetsWorkerRequirement(t *testing.T) {
	assert := require.New(t)
	k8sclientset := k8sfake.NewSimpleClientset(testNode, cpNode1, cpNode2, cpNode3)
	nodeCache := fakeclients.NodeCache(k8sclientset.CoreV1().Nodes)
	err := DrainPossible(nodeCache, testNode)
	assert.NoError(err, "expected no error while place worker node in drain")
}

func Test_maintainModeStrategyFilter_Skip(t *testing.T) {
	assert := require.New(t)
	status := maintainModeStrategyFilter(corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "foo",
		Namespace: "xyz",
		Labels: map[string]string{
			util.LabelMaintainModeStrategy: util.MaintainModeStrategyShutdown,
		},
	}})
	assert.False(status.Delete)
	assert.Equal(status.Reason, drain.PodDeleteStatusTypeSkip)
}

func Test_maintainModeStrategyFilter_Okay_1(t *testing.T) {
	assert := require.New(t)
	status := maintainModeStrategyFilter(corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "foo",
		Namespace: "xyz",
	}})
	assert.True(status.Delete)
	assert.Equal(status.Reason, drain.PodDeleteStatusTypeOkay)
}

func Test_maintainModeStrategyFilter_Okay_2(t *testing.T) {
	assert := require.New(t)
	status := maintainModeStrategyFilter(corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "foo",
		Namespace: "xyz",
		Labels: map[string]string{
			util.LabelMaintainModeStrategy: util.MaintainModeStrategyMigrate,
		},
	}})
	assert.True(status.Delete)
	assert.Equal(status.Reason, drain.PodDeleteStatusTypeOkay)
}
