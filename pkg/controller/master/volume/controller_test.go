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

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestHandler_DetachVolumesOnChange(t *testing.T) {
	type input struct {
		key    string
		volume *lhv1beta1.Volume
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
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}

		var ctrl = &Controller{
			pvcCache:    fakeclients.PersistentVolumeClaimCache(k8sclientset.CoreV1().PersistentVolumeClaims),
			volumeCache: fakeclients.LonghornVolumeCache(clientset.LonghornV1beta1().Volumes),
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
