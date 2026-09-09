package common

import (
	"testing"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/stretchr/testify/assert"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetStorageClassName(t *testing.T) {
	type input struct {
		storageclasses []*storagev1.StorageClass
		vmi            *harvesterv1.VirtualMachineImage
	}
	type output struct {
		name string
	}
	testcases := []struct {
		desc string
		in   input
		ex   output
	}{
		{
			desc: "CDI Backend with foobar StorageClass",
			in: input{
				storageclasses: []*storagev1.StorageClass{},
				vmi: &harvesterv1.VirtualMachineImage{
					Spec: harvesterv1.VirtualMachineImageSpec{
						Backend:                harvesterv1.VMIBackendCDI,
						TargetStorageClassName: "foobar",
					},
				},
			},
			ex: output{name: "foobar"},
		},
		{
			desc: "StorageClass lh-$UID",
			in: input{
				storageclasses: []*storagev1.StorageClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "lh-abcdefghi-jklmno-pqrstu-123456",
						},
					},
				},
				vmi: &harvesterv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "image-foobar",
						UID:  "abcdefghi-jklmno-pqrstu-123456",
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						Backend: harvesterv1.VMIBackendBackingImage,
					},
				},
			},
			ex: output{name: "lh-abcdefghi-jklmno-pqrstu-123456"},
		},
		{
			desc: "StorageClass longhorn-$NAME",
			in: input{
				storageclasses: []*storagev1.StorageClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "longhorn-image-foobar",
						},
					},
				},
				vmi: &harvesterv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "image-foobar",
						UID:  "abcdefghi-jklmno-pqrstu-123456",
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						Backend: harvesterv1.VMIBackendBackingImage,
					},
				},
			},
			ex: output{name: "longhorn-image-foobar"},
		},
		{
			desc: "StorageClass not found",
			in: input{
				storageclasses: []*storagev1.StorageClass{},
				vmi: &harvesterv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "image-foobar",
						UID:  "abcdefghi-jklmno-pqrstu-123456",
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						Backend: harvesterv1.VMIBackendBackingImage,
					},
				},
			},
			ex: output{name: "lh-abcdefghi-jklmno-pqrstu-123456"},
		},
		{
			desc: "Restore StorageClass from backingImage URL parameter when none exists",
			in: input{
				storageclasses: []*storagev1.StorageClass{},
				vmi: &harvesterv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "image-foobar",
						UID:  "abcdefghi-jklmno-pqrstu-123456",
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						Backend:    harvesterv1.VMIBackendBackingImage,
						SourceType: harvesterv1.VirtualMachineImageSourceTypeRestore,
						URL:        "s3://mybucket@pcloud/?backingImage=vmi-06791a48-8d0d-4895-999b-28296f0e1c10",
					},
				},
			},
			ex: output{name: "lh-06791a48-8d0d-4895-999b-28296f0e1c10"},
		},
		{
			desc: "Restore StorageClass uses existing legacy name",
			in: input{
				storageclasses: []*storagev1.StorageClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "longhorn-image-foobar",
						},
					},
				},
				vmi: &harvesterv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "image-foobar",
						UID:  "abcdefghi-jklmno-pqrstu-123456",
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						Backend:    harvesterv1.VMIBackendBackingImage,
						SourceType: harvesterv1.VirtualMachineImageSourceTypeRestore,
						URL:        "s3://mybucket@pcloud/?backingImage=vmi-06791a48-8d0d-4895-999b-28296f0e1c10",
					},
				},
			},
			ex: output{name: "longhorn-image-foobar"},
		},
		{
			desc: "Restore StorageClass uses existing URL-derived name",
			in: input{
				storageclasses: []*storagev1.StorageClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "lh-06791a48-8d0d-4895-999b-28296f0e1c10",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "longhorn-image-foobar",
						},
					},
				},
				vmi: &harvesterv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "image-foobar",
						UID:  "abcdefghi-jklmno-pqrstu-123456",
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						Backend:    harvesterv1.VMIBackendBackingImage,
						SourceType: harvesterv1.VirtualMachineImageSourceTypeRestore,
						URL:        "s3://mybucket@pcloud/?backingImage=vmi-06791a48-8d0d-4895-999b-28296f0e1c10",
					},
				},
			},
			ex: output{name: "lh-06791a48-8d0d-4895-999b-28296f0e1c10"},
		},
		{
			desc: "Custom storage class name override",
			in: input{
				storageclasses: []*storagev1.StorageClass{},
				vmi: &harvesterv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "image-foobar",
						UID:  "abcdefghi-jklmno-pqrstu-123456",
						Annotations: map[string]string{
							"harvesterhci.io/vmImageStorageClassNameOverride": "my-custom-storage-class",
						},
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						Backend: harvesterv1.VMIBackendBackingImage,
					},
				},
			},
			ex: output{name: "my-custom-storage-class"},
		},
		{
			desc: "Skip custom storage class name override when old format sc is present",
			in: input{
				storageclasses: []*storagev1.StorageClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "longhorn-image-foobar",
						},
					},
				},
				vmi: &harvesterv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "image-foobar",
						UID:  "abcdefghi-jklmno-pqrstu-123456",
						Annotations: map[string]string{
							"harvesterhci.io/vmImageStorageClassNameOverride": "my-custom-storage-class",
						},
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						Backend: harvesterv1.VMIBackendBackingImage,
					},
				},
			},
			ex: output{name: "longhorn-image-foobar"},
		},
		{
			desc: "Skip custom storage class name override when new format sc is present",
			in: input{
				storageclasses: []*storagev1.StorageClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "lh-06791a48-8d0d-4895-999b-28296f0e1c10",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "longhorn-image-foobar",
						},
					},
				},
				vmi: &harvesterv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "image-foobar",
						UID:  "06791a48-8d0d-4895-999b-28296f0e1c10",
						Annotations: map[string]string{
							"harvesterhci.io/vmImageStorageClassNameOverride": "my-custom-storage-class",
						},
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						Backend: harvesterv1.VMIBackendBackingImage,
					},
				},
			},
			ex: output{name: "lh-06791a48-8d0d-4895-999b-28296f0e1c10"},
		},
		{
			// restore storageclass when override image name annotation was used during creation
			// in this scenario backingimage will be named after override annotation and stored
			// accordingly in backup target
			desc: "Restore StorageClass uses existing URL-derived name when using custom storage class name override",
			in: input{
				storageclasses: []*storagev1.StorageClass{},
				vmi: &harvesterv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "image-foobar",
						UID:  "abcdefghi-jklmno-pqrstu-123456",
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						Backend:    harvesterv1.VMIBackendBackingImage,
						SourceType: harvesterv1.VirtualMachineImageSourceTypeRestore,
						URL:        "s3://mybucket@pcloud/?backingImage=my-custom-storage-class",
					},
				},
			},
			ex: output{name: "my-custom-storage-class"},
		},
	}

	for _, tc := range testcases {
		var res output
		var clientset = fake.NewSimpleClientset()

		for _, sc := range tc.in.storageclasses {
			err := clientset.Tracker().Add(sc)
			assert.Nil(t, err, "failed creating mock resources")
		}

		var vmio = &vmiOperator{
			scCache: fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
		}

		res.name = vmio.GetStorageClassName(tc.in.vmi)

		assert.Equal(t, res.name, tc.ex.name, tc.desc)
	}
}
