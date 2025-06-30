package persistentvolumeclaim

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/util"
)

func TestPatchDataSource(t *testing.T) {
	var (
		blockMode = corev1.PersistentVolumeBlock
		fsMode    = corev1.PersistentVolumeFilesystem
	)
	var testCases = []struct {
		desc        string
		pvc         *corev1.PersistentVolumeClaim
		expected    string
		expectedErr error
	}{
		{
			desc: "skip if block volume mode",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pvc",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeMode: &blockMode,
				},
			},
			expected:    "",
			expectedErr: nil,
		},
		{
			desc: "skip if no annotations",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-pvc",
					Annotations: map[string]string{},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeMode: &fsMode,
				},
			},
			expected:    "",
			expectedErr: nil,
		},
		{
			desc: "skip if data source ref exists",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pvc",
					Annotations: map[string]string{
						util.AnnotationVolForVM: "true",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeMode: &fsMode,
					DataSourceRef: &corev1.TypedObjectReference{
						Name: "new-snapshot-test",
						Kind: "VolumeSnapshot",
					},
				},
			},
			expected:    "",
			expectedErr: nil,
		},
		{
			desc: "skip if data source exists",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pvc",
					Annotations: map[string]string{
						util.AnnotationVolForVM: "true",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeMode: &fsMode,
					DataSource: &corev1.TypedLocalObjectReference{
						Name: "new-snapshot-test",
						Kind: "VolumeSnapshot",
					},
				},
			},
			expected:    "",
			expectedErr: nil,
		},
		{
			desc: "generate patch if no data source",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pvc",
					Annotations: map[string]string{
						util.AnnotationVolForVM: "true",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeMode: &fsMode,
				},
			},
			expected:    `{"op": "replace", "path": "/spec/dataSourceRef", "value": {"apiGroup":"cdi.kubevirt.io","kind":"VolumeImportSource","name":"filesystem-blank-source"}}`,
			expectedErr: nil,
		},
	}

	m := pvcMutator{}
	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			actual, err := m.patchDataSource(testCase.pvc)
			if err != nil {
				if err != testCase.expectedErr {
					t.Fatal("unexpected error: ", err)
				}
			}

			if actual != testCase.expected {
				t.Fatalf("expected %s, got %s", testCase.expected, actual)
			}
		})
	}
}
