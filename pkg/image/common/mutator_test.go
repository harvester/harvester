package common

import (
	"encoding/json"
	"testing"

	longhorntypes "github.com/longhorn/longhorn-manager/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestPatchImageSCParams(t *testing.T) {
	defaultSC := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "longhorn-v2",
			Annotations: map[string]string{
				util.AnnotationIsDefaultStorageClassName: "true",
			},
		},
		Provisioner: longhorntypes.LonghornDriverName,
		Parameters: map[string]string{
			longhorntypes.OptionNumberOfReplicas: "1",
			longhorntypes.OptionDiskSelector:     "nvme",
			"dataEngine":                         "v2",
		},
	}
	targetSC := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "harvester-longhorn",
		},
		Provisioner: longhorntypes.LonghornDriverName,
		Parameters: map[string]string{
			longhorntypes.OptionNumberOfReplicas: "2",
		},
	}

	// Parameters expected in spec.storageClassParameters for each source SC.
	defaultSCParams := util.GetImageDefaultStorageClassParameters()
	defaultSCParams[longhorntypes.OptionNumberOfReplicas] = "1"
	defaultSCParams[longhorntypes.OptionDiskSelector] = "nvme"
	targetSCParams := util.GetImageDefaultStorageClassParameters()
	targetSCParams[longhorntypes.OptionNumberOfReplicas] = "2"

	testCases := []struct {
		name           string
		annotations    map[string]string
		targetSC       string
		expectedParams map[string]string
	}{
		{
			name: "annotation set",
			annotations: map[string]string{
				util.AnnotationStorageClassName: targetSC.Name,
			},
			expectedParams: targetSCParams,
		},
		{
			name:           "only spec.targetStorageClassName set",
			targetSC:       targetSC.Name,
			expectedParams: targetSCParams,
		},
		{
			name:           "neither set uses the default storageClass",
			expectedParams: defaultSCParams,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(defaultSC.DeepCopy(), targetSC.DeepCopy())
			mutator := GetVMIMutator(fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses))

			vmi := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "image",
					Namespace:   "default",
					Annotations: tc.annotations,
				},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend:                harvesterv1.VMIBackendBackingImage,
					TargetStorageClassName: tc.targetSC,
				},
			}

			patchOps, err := mutator.PatchImageSCParams(vmi)
			require.NoError(t, err)
			require.Len(t, patchOps, 1)

			var op struct {
				Op    string            `json:"op"`
				Path  string            `json:"path"`
				Value map[string]string `json:"value"`
			}
			require.NoError(t, json.Unmarshal([]byte(patchOps[0]), &op))
			assert.Equal(t, "add", op.Op)
			assert.Equal(t, "/spec/storageClassParameters", op.Path)
			assert.Equal(t, tc.expectedParams, op.Value)
		})
	}
}
