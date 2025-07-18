package storageclass

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"kubevirt.io/kubevirt/pkg/apimachinery/patch"

	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
	"github.com/stretchr/testify/assert"
)

func Test_generatePatchOps(t *testing.T) {
	tests := []struct {
		name      string
		sc        *storagev1.StorageClass
		expectOps types.PatchOps
	}{
		{
			name: "longhorn v2 missing annotations",
			sc: &storagev1.StorageClass{
				ObjectMeta:  metav1.ObjectMeta{Name: "lhv2"},
				Provisioner: util.CSIProvisionerLonghorn,
				Parameters:  map[string]string{"dataEngine": string(longhornv1.DataEngineTypeV2)},
			},
			expectOps: types.PatchOps{
				emptyAnnotationsPatch,
				fmt.Sprintf(patchAnnotation,
					patch.EscapeJSONPointer(util.AnnotationStorageProfileCloneStrategy),
					strconv.Quote(string(cdiv1.CloneStrategyHostAssisted))),
				fmt.Sprintf(patchAnnotation,
					patch.EscapeJSONPointer(util.AnnotationStorageProfileSnapshotClass),
					strconv.Quote("longhorn-snapshot")),
			},
		},
		{
			name: "longhorn v2 with annotations",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "lhv2",
					Annotations: map[string]string{
						util.AnnotationStorageProfileCloneStrategy: string(cdiv1.CloneStrategyHostAssisted),
						util.AnnotationStorageProfileSnapshotClass: "longhorn-snapshot",
					},
				},
				Provisioner: util.CSIProvisionerLonghorn,
				Parameters:  map[string]string{"dataEngine": string(longhornv1.DataEngineTypeV2)},
			},
			expectOps: nil,
		},
		{
			name: "longhorn v1 should not patch",
			sc: &storagev1.StorageClass{
				ObjectMeta:  metav1.ObjectMeta{Name: "lhv1"},
				Provisioner: util.CSIProvisionerLonghorn,
				Parameters:  map[string]string{"dataEngine": string(longhornv1.DataEngineTypeV1)},
			},
			expectOps: nil,
		},
		{
			name: "lvm missing annotation",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "lvm",
					Annotations: map[string]string{},
				},
				Provisioner: util.CSIProvisionerLVM,
			},
			expectOps: types.PatchOps{
				fmt.Sprintf(patchAnnotation,
					patch.EscapeJSONPointer(util.AnnotationStorageProfileVolumeModeAccessModes),
					strconv.Quote(`{"Block":["ReadWriteOnce"]}`)),
				fmt.Sprintf(patchAnnotation,
					patch.EscapeJSONPointer(util.AnnotationStorageProfileCloneStrategy),
					strconv.Quote(string(cdiv1.CloneStrategySnapshot))),
				fmt.Sprintf(patchAnnotation,
					patch.EscapeJSONPointer(util.AnnotationStorageProfileSnapshotClass),
					strconv.Quote("lvm-snapshot")),
			},
		},
		{
			name: "lvm with annotation",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "lvm",
					Annotations: map[string]string{
						util.AnnotationStorageProfileVolumeModeAccessModes: `{"Block":["ReadWriteOnce"]}`,
					},
				},
				Provisioner: util.CSIProvisionerLVM,
			},
			expectOps: types.PatchOps{
				fmt.Sprintf(patchAnnotation,
					patch.EscapeJSONPointer(util.AnnotationStorageProfileCloneStrategy),
					strconv.Quote(string(cdiv1.CloneStrategySnapshot))),
				fmt.Sprintf(patchAnnotation,
					patch.EscapeJSONPointer(util.AnnotationStorageProfileSnapshotClass),
					strconv.Quote("lvm-snapshot")),
			},
		},
		{
			name: "lvm with all annotations",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "lvm",
					Annotations: map[string]string{
						util.AnnotationStorageProfileVolumeModeAccessModes: `{"Block":["ReadWriteOnce"]}`,
						util.AnnotationStorageProfileCloneStrategy:         string(cdiv1.CloneStrategySnapshot),
						util.AnnotationStorageProfileSnapshotClass:         "lvm-snapshot",
					},
				},
				Provisioner: util.CSIProvisionerLVM,
			},
			expectOps: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ops := generateCDIAnnoPatchOps(tc.sc)
			for _, op := range ops {
				var js any
				err := json.Unmarshal([]byte(op), &js)
				assert.NoError(t, err, "patch operation should be valid JSON: %s", op)
			}
			assert.Equal(t, tc.expectOps, ops)
		})
	}
}
