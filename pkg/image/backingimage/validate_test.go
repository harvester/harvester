package backingimage

import (
	"testing"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_backingImageAnnotationImmutability(t *testing.T) {
	var testCases = []struct {
		name    string
		oldVMI  *harvesterv1.VirtualMachineImage
		newVMI  *harvesterv1.VirtualMachineImage
		wantErr bool
	}{
		// Add test cases here
		{
			name:    "no change in annotation",
			oldVMI:  &harvesterv1.VirtualMachineImage{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{util.AnnotationHarvesterVMImageStorageClassNameOverride: "old"}}},
			newVMI:  &harvesterv1.VirtualMachineImage{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{util.AnnotationHarvesterVMImageStorageClassNameOverride: "old"}}},
			wantErr: false,
		},
		{
			name:    "no annotation",
			oldVMI:  &harvesterv1.VirtualMachineImage{ObjectMeta: metav1.ObjectMeta{}},
			newVMI:  &harvesterv1.VirtualMachineImage{ObjectMeta: metav1.ObjectMeta{}},
			wantErr: false,
		},
		{
			name:    "annotation changed",
			oldVMI:  &harvesterv1.VirtualMachineImage{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{util.AnnotationHarvesterVMImageStorageClassNameOverride: "old"}}},
			newVMI:  &harvesterv1.VirtualMachineImage{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{util.AnnotationHarvesterVMImageStorageClassNameOverride: "new"}}},
			wantErr: true,
		},
		{
			name:    "annotation removed",
			oldVMI:  &harvesterv1.VirtualMachineImage{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{util.AnnotationHarvesterVMImageStorageClassNameOverride: "old"}}},
			newVMI:  &harvesterv1.VirtualMachineImage{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}}},
			wantErr: true,
		},
		{
			name:    "annotation added",
			oldVMI:  &harvesterv1.VirtualMachineImage{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}}},
			newVMI:  &harvesterv1.VirtualMachineImage{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{util.AnnotationHarvesterVMImageStorageClassNameOverride: "new"}}},
			wantErr: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := (&Validator{}).ensureBackingImageAnnotationIsImmutable(tc.oldVMI, tc.newVMI)
			if (err != nil) != tc.wantErr {
				t.Errorf("expected error: %v, got: %v", tc.wantErr, err)
			}
		})
	}
}
