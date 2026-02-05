package readyz

import (
	"testing"

	"github.com/harvester/go-common/common"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	longhornTypes "github.com/longhorn/longhorn-manager/types"
	rkev1 "github.com/rancher/rancher/pkg/apis/rke.cattle.io/v1"
	"github.com/rancher/rancher/pkg/auth/tokens/hashers"
	rkev1controller "github.com/rancher/rancher/pkg/generated/controllers/rke.cattle.io/v1"
	"github.com/rancher/wrangler/v3/pkg/genericcondition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	corefake "k8s.io/client-go/kubernetes/fake"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

func TestClusterReady(t *testing.T) {
	readyCondition := corev1.PodCondition{
		Type:   corev1.PodReady,
		Status: corev1.ConditionTrue,
	}
	notReadyCondition := corev1.PodCondition{
		Type:   corev1.PodReady,
		Status: corev1.ConditionFalse,
	}

	longhornLabels := labels.Set(longhornTypes.GetManagerLabels())
	virtControllerLabels := labels.Set{kubevirtv1.AppLabel: "virt-controller"}

	tests := []struct {
		name                string
		rkeControlPlane     *rkev1.RKEControlPlane
		pods                []*corev1.Pod
		expectedReady       bool
		expectedMsgContains string
	}{
		{
			name:                "RKE control plane not found",
			rkeControlPlane:     nil,
			pods:                nil,
			expectedReady:       false,
			expectedMsgContains: "rkeControlPlane not found",
		},
		{
			name: "RKE control plane not ready",
			rkeControlPlane: buildMockRKEControlPlane(
				util.LocalClusterName,
				util.FleetLocalNamespaceName,
				[]genericcondition.GenericCondition{
					{Type: "Ready", Status: corev1.ConditionFalse},
				},
			),
			pods:                nil,
			expectedReady:       false,
			expectedMsgContains: "rkeControlPlane is not ready",
		},
		{
			name: "No longhorn manager pods",
			rkeControlPlane: buildMockRKEControlPlane(
				util.LocalClusterName,
				util.FleetLocalNamespaceName,
				[]genericcondition.GenericCondition{
					{Type: "Ready", Status: corev1.ConditionTrue},
				},
			),
			pods:                []*corev1.Pod{},
			expectedReady:       false,
			expectedMsgContains: "longhorn-manager pods not ready",
		},
		{
			name: "Longhorn manager pod not ready",
			rkeControlPlane: buildMockRKEControlPlane(
				util.LocalClusterName,
				util.FleetLocalNamespaceName,
				[]genericcondition.GenericCondition{
					{Type: "Ready", Status: corev1.ConditionTrue},
				},
			),
			pods: []*corev1.Pod{
				buildMockPod(
					"longhorn-manager-1",
					common.LonghornSystemNamespaceName,
					longhornLabels,
					corev1.PodPending,
					[]corev1.PodCondition{notReadyCondition},
				),
			},
			expectedReady:       false,
			expectedMsgContains: "longhorn-manager pods not ready",
		},
		{
			name: "Virt controller pods not ready",
			rkeControlPlane: buildMockRKEControlPlane(
				util.LocalClusterName,
				util.FleetLocalNamespaceName,
				[]genericcondition.GenericCondition{
					{Type: "Ready", Status: corev1.ConditionTrue},
				},
			),
			pods: []*corev1.Pod{
				buildMockPod(
					"longhorn-manager-1",
					common.LonghornSystemNamespaceName,
					longhornLabels,
					corev1.PodRunning,
					[]corev1.PodCondition{readyCondition},
				),
				buildMockPod(
					"virt-controller-1",
					common.HarvesterSystemNamespaceName,
					virtControllerLabels,
					corev1.PodPending,
					[]corev1.PodCondition{notReadyCondition},
				),
			},
			expectedReady:       false,
			expectedMsgContains: "virt-controller pods not ready",
		},
		{
			name: "All components ready - single pods",
			rkeControlPlane: buildMockRKEControlPlane(
				util.LocalClusterName,
				util.FleetLocalNamespaceName,
				[]genericcondition.GenericCondition{
					{Type: "Ready", Status: corev1.ConditionTrue},
				},
			),
			pods: []*corev1.Pod{
				buildMockPod(
					"longhorn-manager-1",
					common.LonghornSystemNamespaceName,
					longhornLabels,
					corev1.PodRunning,
					[]corev1.PodCondition{readyCondition},
				),
				buildMockPod(
					"virt-controller-1",
					common.HarvesterSystemNamespaceName,
					virtControllerLabels,
					corev1.PodRunning,
					[]corev1.PodCondition{readyCondition},
				),
			},
			expectedReady:       true,
			expectedMsgContains: "",
		},
		{
			name: "All components ready - multiple pods with some not ready",
			rkeControlPlane: buildMockRKEControlPlane(
				util.LocalClusterName,
				util.FleetLocalNamespaceName,
				[]genericcondition.GenericCondition{
					{Type: "Ready", Status: corev1.ConditionTrue},
				},
			),
			pods: []*corev1.Pod{
				buildMockPod(
					"longhorn-manager-1",
					common.LonghornSystemNamespaceName,
					longhornLabels,
					corev1.PodRunning,
					[]corev1.PodCondition{readyCondition},
				),
				buildMockPod(
					"longhorn-manager-2",
					common.LonghornSystemNamespaceName,
					longhornLabels,
					corev1.PodPending,
					[]corev1.PodCondition{notReadyCondition},
				),
				buildMockPod(
					"virt-controller-1",
					common.HarvesterSystemNamespaceName,
					virtControllerLabels,
					corev1.PodRunning,
					[]corev1.PodCondition{readyCondition},
				),
				buildMockPod(
					"virt-controller-2",
					common.HarvesterSystemNamespaceName,
					virtControllerLabels,
					corev1.PodFailed,
					[]corev1.PodCondition{notReadyCondition},
				),
			},
			expectedReady:       true,
			expectedMsgContains: "",
		},
		{
			name: "RKE ready condition missing",
			rkeControlPlane: buildMockRKEControlPlane(
				util.LocalClusterName,
				util.FleetLocalNamespaceName,
				[]genericcondition.GenericCondition{},
			),
			pods:                nil,
			expectedReady:       false,
			expectedMsgContains: "rkeControlPlane is not ready",
		},
		{
			name: "Multiple RKE conditions with ready true",
			rkeControlPlane: buildMockRKEControlPlane(
				util.LocalClusterName,
				util.FleetLocalNamespaceName,
				[]genericcondition.GenericCondition{
					{Type: "Provisioned", Status: corev1.ConditionTrue},
					{Type: "Ready", Status: corev1.ConditionTrue},
					{Type: "Updated", Status: corev1.ConditionFalse},
				},
			),
			pods: []*corev1.Pod{
				buildMockPod(
					"longhorn-manager-1",
					common.LonghornSystemNamespaceName,
					longhornLabels,
					corev1.PodRunning,
					[]corev1.PodCondition{readyCondition},
				),
				buildMockPod(
					"virt-controller-1",
					common.HarvesterSystemNamespaceName,
					virtControllerLabels,
					corev1.PodRunning,
					[]corev1.PodCondition{readyCondition},
				),
			},
			expectedReady:       true,
			expectedMsgContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testClientset := corefake.NewSimpleClientset()

			for _, pod := range tt.pods {
				err := testClientset.Tracker().Add(pod)
				assert.Nil(t, err, "Mock pod should add into fake controller tracker")
			}

			podCache := fakeclients.PodCache(testClientset.CoreV1().Pods)
			rkeCache := fakeclients.RKEControlPlaneCache(func(namespace string) rkev1controller.RKEControlPlaneClient {
				rkeMap := make(map[string]*rkev1.RKEControlPlane)
				if tt.rkeControlPlane != nil {
					rkeMap[tt.rkeControlPlane.Namespace+"/"+tt.rkeControlPlane.Name] = tt.rkeControlPlane
				}
				return &fakeclients.MockRKEControlPlaneClient{
					RKEControlPlanes: rkeMap,
				}
			})

			handler := &ReadyzHandler{
				podCache: podCache,
				rkeCache: rkeCache,
			}
			ready, msg := handler.clusterReady()
			assert.Equal(t, tt.expectedReady, ready, "Ready status should match expected")
			if tt.expectedMsgContains != "" {
				assert.Contains(t, msg, tt.expectedMsgContains, "Message should contain expected substring")
			} else {
				assert.Empty(t, msg, "Message should be empty when cluster is ready")
			}
		})
	}
}

func TestAuthentication(t *testing.T) {
	tests := []struct {
		name          string
		setupToken    string
		providedToken string
		injectHash    string
		shouldError   bool
	}{
		{
			name:          "Valid token with hasher verification",
			setupToken:    "correct-token",
			providedToken: "correct-token",
			shouldError:   false,
		},
		{
			name:          "Invalid token",
			setupToken:    "correct-token",
			providedToken: "wrong-token",
			shouldError:   true,
		},
		{
			name:          "Invalid hash format",
			injectHash:    "invalid-hash",
			providedToken: "any-token",
			shouldError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			hash := tt.injectHash
			if hash == "" {
				hash, err = hashers.Sha256Hasher{}.CreateHash(tt.setupToken)
				require.NoError(t, err)
			}

			originalHash := tokenHash
			tokenHash = hash
			defer func() { tokenHash = originalHash }()

			err = (&ReadyzHandler{}).validateToken(tt.providedToken)

			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func buildMockPod(name, namespace string, labels map[string]string, phase corev1.PodPhase, conditions []corev1.PodCondition) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Status: corev1.PodStatus{
			Phase:      phase,
			Conditions: conditions,
		},
	}
}

func buildMockRKEControlPlane(name string, namespace string, conditions []genericcondition.GenericCondition) *rkev1.RKEControlPlane {
	return &rkev1.RKEControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: rkev1.RKEControlPlaneStatus{
			Conditions: conditions,
		},
	}
}
