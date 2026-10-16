package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestEnsureTargetSC(t *testing.T) {
	defaultSC := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "harvester-longhorn",
			Annotations: map[string]string{util.AnnotationIsDefaultStorageClassName: "true"},
		},
	}
	nonDefaultSC := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "harvester-longhorn",
			Annotations: map[string]string{util.AnnotationIsDefaultStorageClassName: "false"},
		},
	}
	// a VMI created before the target StorageClass was enforced
	legacyVMI := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{Name: "hvst-upgrade-p2mk7", Namespace: "harvester-system"},
	}

	testCases := []struct {
		name        string
		sc          *storagev1.StorageClass
		vmi         *harvesterv1.VirtualMachineImage
		update      bool
		expectErr   bool
		expectPatch bool
	}{
		{
			name:      "create without default storage class is rejected",
			sc:        nonDefaultSC,
			vmi:       legacyVMI,
			expectErr: true,
		},
		{
			name:        "create with default storage class is backfilled",
			sc:          defaultSC,
			vmi:         legacyVMI,
			expectPatch: true,
		},
		{
			name:   "update of legacy VMI without default storage class is allowed",
			sc:     nonDefaultSC,
			vmi:    legacyVMI,
			update: true,
		},
		{
			name:        "update of legacy VMI with default storage class is backfilled",
			sc:          defaultSC,
			vmi:         legacyVMI,
			update:      true,
			expectPatch: true,
		},
		{
			name: "update of VMI with target storage class needs no default",
			sc:   nonDefaultSC,
			vmi: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "image",
					Namespace:   "default",
					Annotations: map[string]string{util.AnnotationStorageClassName: "longhorn-static"},
				},
				Spec: harvesterv1.VirtualMachineImageSpec{TargetStorageClassName: "longhorn-static"},
			},
			update: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tc.sc)
			m := GetVMIMutator(fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses))

			var patchOps []string
			var err error
			if tc.update {
				patchOps, err = m.EnsureTargetSCOnUpdate(tc.vmi)
			} else {
				patchOps, err = m.EnsureTargetSC(tc.vmi)
			}

			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expectPatch, len(patchOps) > 0)
		})
	}
}
