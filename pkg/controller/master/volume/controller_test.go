package volume

import (
	"fmt"
	"testing"

	lhv1beta1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestHandler_DetachVolumesOnChange(t *testing.T) {
	type input struct {
		key    string
		volume *lhv1beta1.Volume
		vm     *kubevirtv1.VirtualMachine
		pvcs   []*corev1.PersistentVolumeClaim
	}
	type output struct {
		volume *lhv1beta1.Volume
		err    error
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name:     "ignor nil resource",
			given:    input{},
			expected: output{},
		},
		{
			name: "ignore deleted resource",
			given: input{
				key: "longhorn-system/test-with-deletion-timestamp",
				volume: &lhv1beta1.Volume{
					ObjectMeta: metav1.ObjectMeta{
						DeletionTimestamp: &metav1.Time{},
					},
				},
			},
			expected: output{
				volume: &lhv1beta1.Volume{
					ObjectMeta: metav1.ObjectMeta{
						DeletionTimestamp: &metav1.Time{},
					},
				},
				err: nil,
			},
		},
		{
			name: "skip detached volume",
			given: input{
				key: "longhorn-system/test-skip-detached-volume",
				volume: &lhv1beta1.Volume{
					Status: lhv1beta1.VolumeStatus{
						State: lhv1beta1.VolumeStateDetached,
						KubernetesStatus: lhv1beta1.KubernetesStatus{
							Namespace: "default",
							PVCName:   "test-missing-pvc",
						},
					},
				},
			},
			expected: output{
				volume: &lhv1beta1.Volume{
					Status: lhv1beta1.VolumeStatus{
						State: lhv1beta1.VolumeStateDetached,
						KubernetesStatus: lhv1beta1.KubernetesStatus{
							Namespace: "default",
							PVCName:   "test-missing-pvc",
						},
					},
				},
				err: nil,
			},
		},
		{
			name: "skip detaching volume",
			given: input{
				key: "longhorn-system/test-skip-detaching-volume",
				volume: &lhv1beta1.Volume{
					Status: lhv1beta1.VolumeStatus{
						State: lhv1beta1.VolumeStateDetaching,
						KubernetesStatus: lhv1beta1.KubernetesStatus{
							Namespace: "default",
							PVCName:   "test-missing-pvc",
						},
					},
				},
			},
			expected: output{
				volume: &lhv1beta1.Volume{
					Status: lhv1beta1.VolumeStatus{
						State: lhv1beta1.VolumeStateDetaching,
						KubernetesStatus: lhv1beta1.KubernetesStatus{
							Namespace: "default",
							PVCName:   "test-missing-pvc",
						},
					},
				},
				err: nil,
			},
		},
		{
			name: "missing pvc",
			given: input{
				key: "longhorn-system/test-missing-pvc-volume",
				volume: &lhv1beta1.Volume{
					Status: lhv1beta1.VolumeStatus{
						State: lhv1beta1.VolumeStateAttached,
						KubernetesStatus: lhv1beta1.KubernetesStatus{
							Namespace: "default",
							PVCName:   "test-missing-pvc",
						},
					},
				},
			},
			expected: output{
				volume: &lhv1beta1.Volume{
					Status: lhv1beta1.VolumeStatus{
						State: lhv1beta1.VolumeStateAttached,
						KubernetesStatus: lhv1beta1.KubernetesStatus{
							Namespace: "default",
							PVCName:   "test-missing-pvc",
						},
					},
				},
				err: fmt.Errorf("can't find pvc"),
			},
		},
		{
			name: "volume on a running pod",
			given: input{
				key: "longhorn-system/test-volume-on-a-running-pod",
				volume: &lhv1beta1.Volume{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "longhorn-system",
						Name:      "test-volume-on-a-running-pod",
					},
					Status: lhv1beta1.VolumeStatus{
						State: lhv1beta1.VolumeStateAttached,
						KubernetesStatus: lhv1beta1.KubernetesStatus{
							Namespace: "default",
							PVCName:   "test-pvc",
							WorkloadsStatus: []lhv1beta1.WorkloadStatus{
								{
									PodName:   "test-pod",
									PodStatus: "Running",
								},
							},
						},
					},
				},
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "test-pvc",
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							VolumeName: "test-volume-on-a-running-pod",
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimBound,
						},
					},
				},
			},
			expected: output{
				volume: &lhv1beta1.Volume{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "longhorn-system",
						Name:      "test-volume-on-a-running-pod",
					},
					Status: lhv1beta1.VolumeStatus{
						State: lhv1beta1.VolumeStateAttached,
						KubernetesStatus: lhv1beta1.KubernetesStatus{
							Namespace: "default",
							PVCName:   "test-pvc",
							WorkloadsStatus: []lhv1beta1.WorkloadStatus{
								{
									PodName:   "test-pod",
									PodStatus: "Running",
								},
							},
						},
					},
				},
				err: nil,
			},
		},
		{
			name: "volume on a restarting vm",
			given: input{
				key: "longhorn-system/test-volume-on-a-restarting-vm",
				volume: &lhv1beta1.Volume{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "longhorn-system",
						Name:      "test-volume-on-a-restarting-vm",
					},
					Status: lhv1beta1.VolumeStatus{
						State: lhv1beta1.VolumeStateAttached,
						KubernetesStatus: lhv1beta1.KubernetesStatus{
							Namespace: "default",
							PVCName:   "test-pvc",
							WorkloadsStatus: []lhv1beta1.WorkloadStatus{
								{
									PodName:      "test-pod",
									PodStatus:    "Succeeded",
									WorkloadName: "test-vm",
									WorkloadType: "VirtualMachineInstance",
								},
							},
						},
					},
				},
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-vm",
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						RunStrategy: &[]kubevirtv1.VirtualMachineRunStrategy{kubevirtv1.RunStrategyRerunOnFailure}[0],
					},
					Status: kubevirtv1.VirtualMachineStatus{
						PrintableStatus: kubevirtv1.VirtualMachineStatusStopped,
					},
				},
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "test-pvc",
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							VolumeName: "test-volume-on-a-restarting-vm",
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimBound,
						},
					},
				},
			},
			expected: output{
				volume: &lhv1beta1.Volume{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "longhorn-system",
						Name:      "test-volume-on-a-restarting-vm",
					},
					Status: lhv1beta1.VolumeStatus{
						State: lhv1beta1.VolumeStateAttached,
						KubernetesStatus: lhv1beta1.KubernetesStatus{
							Namespace: "default",
							PVCName:   "test-pvc",
							WorkloadsStatus: []lhv1beta1.WorkloadStatus{
								{
									PodName:      "test-pod",
									PodStatus:    "Succeeded",
									WorkloadName: "test-vm",
									WorkloadType: "VirtualMachineInstance",
								},
							},
						},
					},
				},
				err: nil,
			},
		},
	}

	for _, tc := range testCases {
		var pvcobjs []runtime.Object
		for _, v := range tc.given.pvcs {
			if tc.given.pvcs != nil {
				pvcobjs = append(pvcobjs, v)
			}
		}
		k8sclientset := k8sfake.NewSimpleClientset(pvcobjs...)

		clientset := fake.NewSimpleClientset()
		if tc.given.volume != nil {
			var err = clientset.Tracker().Add(tc.given.volume)
			assert.Nil(t, err, "mock volume should add into fake controller tracker")
		}
		if tc.given.vm != nil {
			var err = clientset.Tracker().Add(tc.given.vm)
			assert.Nil(t, err, "mock vm should add into fake controller tracker")
		}
		var ctrl = &Controller{
			pvcCache:    fakeclients.PersistentVolumeClaimCache(k8sclientset.CoreV1().PersistentVolumeClaims),
			volumeCache: fakeclients.LonghornVolumeCache(clientset.LonghornV1beta1().Volumes),
			vmCache:     fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
		}
		volume, err := ctrl.DetachVolumesOnChange(tc.given.key, tc.given.volume)
		assert.Equal(t, tc.expected.volume, volume, "case %q", tc.name)
		if tc.expected.err != nil {
			assert.NotNil(t, err, "case %q", tc.name)
		} else {
			assert.Nil(t, err, "case %q", tc.name)
		}
	}
}
