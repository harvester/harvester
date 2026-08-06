package imagevolumesizecheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestCheckPVC(t *testing.T) {
	image := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-image",
			Namespace: "default",
		},
		Status: harvesterv1.VirtualMachineImageStatus{
			VirtualSize: 5 * 1024 * 1024 * 1024, // 5Gi
		},
	}

	tests := []struct {
		name            string
		pvc             *corev1.PersistentVolumeClaim
		expectViolation bool
		expectMinSizeGi int64
		expectError     bool
	}{
		{
			name: "no image annotation is not a violation",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "pvc-1", Namespace: "default"},
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
					},
				},
			},
			expectViolation: false,
		},
		{
			name: "undersized volume is a violation",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvc-2",
					Namespace: "default",
					Annotations: map[string]string{
						util.AnnotationImageID: "default/test-image",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
					},
				},
			},
			expectViolation: true,
			expectMinSizeGi: 5,
		},
		{
			name: "sufficiently sized volume is not a violation",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvc-3",
					Namespace: "default",
					Annotations: map[string]string{
						util.AnnotationImageID: "default/test-image",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("5Gi")},
					},
				},
			},
			expectViolation: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cs := fake.NewSimpleClientset(image)
			c := &Checker{
				imageCache: fakeclients.VirtualMachineImageCache(cs.HarvesterhciV1beta1().VirtualMachineImages),
			}

			violation, minSize, err := c.checkPVCMinSize(tc.pvc)

			if tc.expectError {
				assert.NotNil(t, err, tc.name)
				return
			}

			assert.Nil(t, err, tc.name)
			assert.Equal(t, tc.expectViolation, violation, tc.name)

			if tc.expectViolation {
				assert.Equal(t, tc.expectMinSizeGi*1024*1024*1024, minSize.Value(), tc.name)
			}
		})
	}
}
