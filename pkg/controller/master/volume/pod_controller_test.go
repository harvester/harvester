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

func TestHandler_AttachVolumesOnChange(t *testing.T) {
	type input struct {
		key     string
		pod     *corev1.Pod
		pvcs    []*corev1.PersistentVolumeClaim
		volumes []*lhv1beta1.Volume
	}
	type output struct {
		pod *corev1.Pod
		err error
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
				key: "default/test-deleted-pod",
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						DeletionTimestamp: &metav1.Time{},
					},
				},
			},
			expected: output{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						DeletionTimestamp: &metav1.Time{},
					},
				},
				err: nil,
			},
		},
		{
			name: "skip running pod",
			given: input{
				key: "default/test-running-pod",
				pod: &corev1.Pod{
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			expected: output{
				pod: &corev1.Pod{
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
				err: nil,
			},
		},
		{
			name: "skip succeeded pod",
			given: input{
				key: "default/test-succeeded-pod",
				pod: &corev1.Pod{
					Status: corev1.PodStatus{
						Phase: corev1.PodSucceeded,
					},
				},
			},
			expected: output{
				pod: &corev1.Pod{
					Status: corev1.PodStatus{
						Phase: corev1.PodSucceeded,
					},
				},
				err: nil,
			},
		},
		{
			name: "skip failed pod",
			given: input{
				key: "default/test-failed-pod",
				pod: &corev1.Pod{
					Status: corev1.PodStatus{
						Phase: corev1.PodFailed,
					},
				},
			},
			expected: output{
				pod: &corev1.Pod{
					Status: corev1.PodStatus{
						Phase: corev1.PodFailed,
					},
				},
				err: nil,
			},
		},
		{
			name: "skip unknown pod",
			given: input{
				key: "default/test-unknown-pod",
				pod: &corev1.Pod{
					Status: corev1.PodStatus{
						Phase: corev1.PodUnknown,
					},
				},
			},
			expected: output{
				pod: &corev1.Pod{
					Status: corev1.PodStatus{
						Phase: corev1.PodUnknown,
					},
				},
				err: nil,
			},
		},
		{
			name: "missing pvc",
			given: input{
				key: "default/test-missing-pvc-pod",
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "test-missing-pvc",
									},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				},
			},
			expected: output{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "test-missing-pvc",
									},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				},
				err: fmt.Errorf("can't get pvc"),
			},
		},
		{
			name: "non longhorn pvc",
			given: input{
				key: "default/test-non-longhorn-pvc-pod",
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "test-non-longhorn-pvc",
									},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				},
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "test-non-longhorn-pvc",
							Annotations: map[string]string{
								"volume.kubernetes.io/storage-provisioner": "not-longhorn-driver",
							},
						},
					},
				},
			},
			expected: output{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "test-non-longhorn-pvc",
									},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				},
				err: nil,
			},
		},
		{
			name: "missing volume",
			given: input{
				key: "default/test-missing-volume-pod",
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "test-missing-volume-pvc",
									},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				},
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "test-missing-volume-pvc",
							Annotations: map[string]string{
								"volume.kubernetes.io/storage-provisioner": "driver.longhorn.io",
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							VolumeName: "test-missing-volume",
						},
					},
				},
			},
			expected: output{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "test-missing-volume-pvc",
									},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				},
				err: fmt.Errorf("can't find volume"),
			},
		},
		{
			name: "attaching volume",
			given: input{
				key: "default/test-attaching-volume-pod",
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "test-attaching-volume-pvc",
									},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				},
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "test-attaching-volume-pvc",
							Annotations: map[string]string{
								"volume.kubernetes.io/storage-provisioner": "driver.longhorn.io",
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							VolumeName: "test-attaching-volume",
						},
					},
				},
				volumes: []*lhv1beta1.Volume{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "longhorn-system",
							Name:      "test-attaching-volume",
						},
						Status: lhv1beta1.VolumeStatus{
							State: lhv1beta1.VolumeStateAttaching,
						},
					},
				},
			},
			expected: output{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "test-attaching-volume-pvc",
									},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				},
				err: nil,
			},
		},
		{
			name: "attached volume",
			given: input{
				key: "default/test-attached-volume-pod",
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "test-attached-volume-pvc",
									},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				},
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "test-attached-volume-pvc",
							Annotations: map[string]string{
								"volume.kubernetes.io/storage-provisioner": "driver.longhorn.io",
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							VolumeName: "test-attached-volume",
						},
					},
				},
				volumes: []*lhv1beta1.Volume{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "longhorn-system",
							Name:      "test-attached-volume",
						},
						Status: lhv1beta1.VolumeStatus{
							State: lhv1beta1.VolumeStateAttached,
						},
					},
				},
			},
			expected: output{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "test-attached-volume-pvc",
									},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				},
				err: nil,
			},
		},
		{
			name: "detached volume",
			given: input{
				key: "default/test-detached-volume-pod",
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "test-detached-volume-pvc",
									},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				},
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "test-detached-volume-pvc",
							Annotations: map[string]string{
								"volume.kubernetes.io/storage-provisioner": "driver.longhorn.io",
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							VolumeName: "test-detached-volume",
						},
					},
				},
				volumes: []*lhv1beta1.Volume{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "longhorn-system",
							Name:      "test-detached-volume",
						},
						Spec: lhv1beta1.VolumeSpec{
							NodeID: "",
						},
						Status: lhv1beta1.VolumeStatus{
							State:   lhv1beta1.VolumeStateDetached,
							OwnerID: "node-1",
						},
					},
				},
			},
			expected: output{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "test-detached-volume-pvc",
									},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				},
				err: nil,
			},
		},
	}

	for _, tc := range testCases {
		var objs, lhObjs []runtime.Object
		for _, p := range tc.given.pvcs {
			if p != nil {
				objs = append(objs, p)
			}
		}
		for _, v := range tc.given.volumes {
			if v != nil {
				lhObjs = append(lhObjs, v)
			}
		}
		k8sclientset := k8sfake.NewSimpleClientset(objs...)
		clientset := fake.NewSimpleClientset(lhObjs...)

		var ctrl = &PodController{
			pvcCache:    fakeclients.PersistentVolumeClaimCache(k8sclientset.CoreV1().PersistentVolumeClaims),
			volumes:     fakeclients.LonghornVolumeClient(clientset.LonghornV1beta1().Volumes),
			volumeCache: fakeclients.LonghornVolumeCache(clientset.LonghornV1beta1().Volumes),
		}
		pod, err := ctrl.AttachVolumesOnChange(tc.given.key, tc.given.pod)
		assert.Equal(t, tc.expected.pod, pod, "case %q", tc.name)
		if tc.expected.err != nil {
			assert.NotNil(t, err, "case %q", tc.name)
		} else {
			assert.Nil(t, err, "case %q", tc.name)
		}
	}
}
