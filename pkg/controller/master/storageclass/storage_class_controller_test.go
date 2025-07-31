package storageclass

import (
	"testing"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	storagesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

func TestCompareClaimPropertySets(t *testing.T) {
	block := corev1.PersistentVolumeBlock
	fs := corev1.PersistentVolumeFilesystem

	tests := []struct {
		name     string
		a, b     []cdiv1.ClaimPropertySet
		expected bool
	}{
		{
			name: "identical sets, same order",
			a: []cdiv1.ClaimPropertySet{
				{VolumeMode: &block, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadOnlyMany}},
				{VolumeMode: &fs, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}},
			},
			b: []cdiv1.ClaimPropertySet{
				{VolumeMode: &block, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadOnlyMany}},
				{VolumeMode: &fs, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}},
			},
			expected: true,
		},
		{
			name: "identical sets, different order",
			a: []cdiv1.ClaimPropertySet{
				{VolumeMode: &fs, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}},
				{VolumeMode: &block, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany, corev1.ReadWriteOnce}},
			},
			b: []cdiv1.ClaimPropertySet{
				{VolumeMode: &block, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadOnlyMany}},
				{VolumeMode: &fs, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}},
			},
			expected: true,
		},
		{
			name: "different access modes",
			a: []cdiv1.ClaimPropertySet{
				{VolumeMode: &block, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}},
			},
			b: []cdiv1.ClaimPropertySet{
				{VolumeMode: &block, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany}},
			},
			expected: false,
		},
		{
			name: "different volume modes",
			a: []cdiv1.ClaimPropertySet{
				{VolumeMode: &block, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}},
			},
			b: []cdiv1.ClaimPropertySet{
				{VolumeMode: &fs, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}},
			},
			expected: false,
		},
		{
			name:     "empty sets",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name: "one empty, one not",
			a:    nil,
			b: []cdiv1.ClaimPropertySet{
				{VolumeMode: &block, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}},
			},
			expected: false,
		},
		{
			name: "access modes different order",
			a: []cdiv1.ClaimPropertySet{
				{VolumeMode: &block, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany, corev1.ReadWriteOnce}},
			},
			b: []cdiv1.ClaimPropertySet{
				{VolumeMode: &block, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadOnlyMany}},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := equalsClaimPropertySets(tt.a, tt.b)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func Test_syncStorageProfile(t *testing.T) {
	block := corev1.PersistentVolumeBlock
	fs := corev1.PersistentVolumeFilesystem

	tests := []struct {
		name        string
		sc          *storagev1.StorageClass
		sp          *cdiv1.StorageProfile
		vsc         *storagesnapshotv1.VolumeSnapshotClass
		expectedSP  *cdiv1.StorageProfile
		expectedErr bool
	}{
		{
			name: "update clone strategy",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc",
					Annotations: map[string]string{
						util.AnnotationStorageProfileCloneStrategy: "snapshot",
					},
				},
			},
			sp: &cdiv1.StorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "sc"},
			},
			expectedSP: &cdiv1.StorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "sc"},
				Spec: cdiv1.StorageProfileSpec{
					CloneStrategy: ptr.To(cdiv1.CloneStrategySnapshot),
				},
			},
		},
		{
			name: "remove clone strategy",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc",
				},
			},
			sp: &cdiv1.StorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "sc"},
				Spec: cdiv1.StorageProfileSpec{
					CloneStrategy: ptr.To(cdiv1.CloneStrategySnapshot),
				},
			},
			expectedSP: &cdiv1.StorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "sc"},
			},
		},
		{
			name: "update snapshot class",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc",
					Annotations: map[string]string{
						util.AnnotationStorageProfileSnapshotClass: "snapclass",
					},
				},
			},
			sp: &cdiv1.StorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "sc"},
			},
			vsc: &storagesnapshotv1.VolumeSnapshotClass{
				ObjectMeta: metav1.ObjectMeta{Name: "snapclass"},
			},
			expectedSP: &cdiv1.StorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "sc"},
				Spec: cdiv1.StorageProfileSpec{
					SnapshotClass: ptr.To("snapclass"),
				},
			},
		},
		{
			name: "skip update if snapshot class not found",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc",
					Annotations: map[string]string{
						util.AnnotationStorageProfileSnapshotClass: "snapclass",
					},
				},
			},
			sp: &cdiv1.StorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "sc"},
			},
			expectedErr: true,
		},
		{
			name: "remove snapshot class",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc",
				},
			},
			sp: &cdiv1.StorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "sc"},
				Spec: cdiv1.StorageProfileSpec{
					SnapshotClass: ptr.To("snapclass"),
				},
			},
			expectedSP: &cdiv1.StorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "sc"},
			},
		},
		{
			name: "update claim property sets",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc",
					Annotations: map[string]string{
						util.AnnotationStorageProfileVolumeModeAccessModes: `{"Block":["ReadWriteOnce"]}`,
					},
				},
			},
			sp: &cdiv1.StorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "sc"},
				Spec: cdiv1.StorageProfileSpec{
					ClaimPropertySets: []cdiv1.ClaimPropertySet{
						{VolumeMode: &fs, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}},
					},
				},
			},
			expectedSP: &cdiv1.StorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "sc"},
				Spec: cdiv1.StorageProfileSpec{
					ClaimPropertySets: []cdiv1.ClaimPropertySet{
						{VolumeMode: &block, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}},
					},
				},
			},
		},
		{
			name: "remove claim property sets",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc",
				},
			},
			sp: &cdiv1.StorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "sc"},
				Spec: cdiv1.StorageProfileSpec{
					ClaimPropertySets: []cdiv1.ClaimPropertySet{
						{VolumeMode: &block, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}},
						{VolumeMode: &fs, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}},
					},
				},
			},
			expectedSP: &cdiv1.StorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "sc"},
			},
		},
		{
			name: "invalid claim property sets json",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc",
					Annotations: map[string]string{
						util.AnnotationStorageProfileVolumeModeAccessModes: `not-a-json`,
					},
				},
			},
			sp: &cdiv1.StorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "sc"},
			},
			expectedErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			typedObjects := []runtime.Object{}
			if tc.sp != nil {
				typedObjects = append(typedObjects, tc.sp)
			}
			if tc.vsc != nil {
				typedObjects = append(typedObjects, tc.vsc)
			}
			clientset := fake.NewSimpleClientset(typedObjects...)
			storageProfileCache := fakeclients.StorageProfileCache(clientset.CdiV1beta1().StorageProfiles)
			handler := &storageClassHandler{
				storageProfileCache:      storageProfileCache,
				storageProfileClient:     fakeclients.StorageProfileClient(clientset.CdiV1beta1().StorageProfiles),
				volumeSnapshotClassCache: fakeclients.VolumeSnapshotClassCache(clientset.SnapshotV1().VolumeSnapshotClasses),
			}

			err := handler.syncStorageProfile(tc.sc)
			if tc.expectedErr {
				assert.Error(t, err, "expected error but got none")
				return
			}
			assert.NoError(t, err)

			actualSP, err := storageProfileCache.Get(tc.sc.Name)
			if tc.expectedSP == nil {
				assert.True(t, apierrors.IsNotFound(err), "expected storage profile not exist")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedSP, actualSP)
			}
		})
	}
}

func Test_syncCDISettings(t *testing.T) {
	tests := []struct {
		name           string
		sc             *storagev1.StorageClass
		cdi            *cdiv1.CDI
		expectCDI      *cdiv1.CDI
		cdiConfigExist bool
	}{
		{
			name: "CDI not found should skip",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{Name: "sc"},
			},
			cdiConfigExist: false,
		},
		{
			name: "invalid filesystem overhead annotation should skip update",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc",
					Annotations: map[string]string{
						util.AnnotationCDIFSOverhead: "1.2345",
					},
				},
			},
			cdi: &cdiv1.CDI{
				ObjectMeta: metav1.ObjectMeta{Name: util.CDIObjectName},
			},
			expectCDI: &cdiv1.CDI{
				ObjectMeta: metav1.ObjectMeta{Name: util.CDIObjectName},
			},
			cdiConfigExist: true,
		},
		{
			name: "valid filesystem overhead triggers update",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc",
					Annotations: map[string]string{
						util.AnnotationCDIFSOverhead: "0.85",
					},
				},
			},
			cdi: &cdiv1.CDI{
				ObjectMeta: metav1.ObjectMeta{Name: util.CDIObjectName},
			},
			expectCDI: &cdiv1.CDI{
				ObjectMeta: metav1.ObjectMeta{Name: util.CDIObjectName},
				Spec: cdiv1.CDISpec{
					Config: &cdiv1.CDIConfigSpec{
						FilesystemOverhead: &cdiv1.FilesystemOverhead{
							StorageClass: map[string]cdiv1.Percent{
								"sc": "0.85",
							},
						},
					},
				},
			},
			cdiConfigExist: true,
		},
		{
			name: "valid filesystem overhead when annotation missing (cdi storage class map is nil)",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc",
					Annotations: map[string]string{
						util.AnnotationCDIFSOverhead: "0.85",
					},
				},
			},
			cdi: &cdiv1.CDI{
				ObjectMeta: metav1.ObjectMeta{Name: util.CDIObjectName},
				Spec: cdiv1.CDISpec{
					Config: &cdiv1.CDIConfigSpec{
						FilesystemOverhead: &cdiv1.FilesystemOverhead{},
					},
				},
			},
			expectCDI: &cdiv1.CDI{
				ObjectMeta: metav1.ObjectMeta{Name: util.CDIObjectName},
				Spec: cdiv1.CDISpec{
					Config: &cdiv1.CDIConfigSpec{
						FilesystemOverhead: &cdiv1.FilesystemOverhead{
							StorageClass: map[string]cdiv1.Percent{
								"sc": "0.85",
							},
						},
					},
				},
			},
			cdiConfigExist: true,
		},
		{
			name: "remove filesystem overhead when annotation missing",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc",
				},
			},
			cdi: &cdiv1.CDI{
				ObjectMeta: metav1.ObjectMeta{Name: util.CDIObjectName},
				Spec: cdiv1.CDISpec{
					Config: &cdiv1.CDIConfigSpec{
						FilesystemOverhead: &cdiv1.FilesystemOverhead{
							StorageClass: map[string]cdiv1.Percent{
								"sc": "0.85",
							},
						},
					},
				},
			},
			expectCDI: &cdiv1.CDI{
				ObjectMeta: metav1.ObjectMeta{Name: util.CDIObjectName},
				Spec: cdiv1.CDISpec{
					Config: &cdiv1.CDIConfigSpec{
						FilesystemOverhead: &cdiv1.FilesystemOverhead{
							StorageClass: map[string]cdiv1.Percent{},
						},
					},
				},
			},
			cdiConfigExist: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			typedObjects := []runtime.Object{}
			if tc.cdi != nil {
				typedObjects = append(typedObjects, tc.cdi)
			}
			clientset := fake.NewSimpleClientset(typedObjects...)
			handler := &storageClassHandler{
				cdiCache:  fakeclients.CDICache(clientset.CdiV1beta1().CDIs),
				cdiClient: fakeclients.CDIClient(clientset.CdiV1beta1().CDIs),
			}

			err := handler.syncCDISettings(tc.sc)
			assert.NoError(t, err)

			updated, err := handler.cdiCache.Get(util.CDIObjectName)
			if !tc.cdiConfigExist {
				assert.True(t, apierrors.IsNotFound(err), "expected CDI not exist")
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expectCDI, updated)
		})
	}
}

func Test_syncCDI(t *testing.T) {
	// test case: Longhorn v1 should skip
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "lhv1",
		},
		Provisioner: util.CSIProvisionerLonghorn,
		Parameters:  map[string]string{"dataEngine": "v1"},
	}
	handler := &storageClassHandler{}
	err := handler.syncCDI(sc)
	assert.NoError(t, err)

	// test case: should update CDI settings and storage profile
	sc = &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sc",
			Annotations: map[string]string{
				util.AnnotationCDIFSOverhead:                       "0.85",
				util.AnnotationStorageProfileCloneStrategy:         "snapshot",
				util.AnnotationStorageProfileVolumeModeAccessModes: `{"Filesystem":["ReadWriteMany"]}`,
				util.AnnotationStorageProfileSnapshotClass:         "snapclass",
			},
		},
	}
	cdi := &cdiv1.CDI{ObjectMeta: metav1.ObjectMeta{Name: util.CDIObjectName}}
	sp := &cdiv1.StorageProfile{ObjectMeta: metav1.ObjectMeta{Name: "sc"}}
	vsc := &storagesnapshotv1.VolumeSnapshotClass{ObjectMeta: metav1.ObjectMeta{Name: "snapclass"}}

	typedObjects := []runtime.Object{cdi, sp, vsc}
	clientset := fake.NewSimpleClientset(typedObjects...)
	handler = &storageClassHandler{
		cdiCache:                 fakeclients.CDICache(clientset.CdiV1beta1().CDIs),
		cdiClient:                fakeclients.CDIClient(clientset.CdiV1beta1().CDIs),
		storageProfileCache:      fakeclients.StorageProfileCache(clientset.CdiV1beta1().StorageProfiles),
		storageProfileClient:     fakeclients.StorageProfileClient(clientset.CdiV1beta1().StorageProfiles),
		volumeSnapshotClassCache: fakeclients.VolumeSnapshotClassCache(clientset.SnapshotV1().VolumeSnapshotClasses),
	}
	err = handler.syncCDI(sc)
	assert.NoError(t, err)
	updatedCDI, err := handler.cdiCache.Get(util.CDIObjectName)
	assert.NoError(t, err)
	expectedCDI := &cdiv1.CDI{
		ObjectMeta: metav1.ObjectMeta{Name: util.CDIObjectName},
		Spec: cdiv1.CDISpec{
			Config: &cdiv1.CDIConfigSpec{
				FilesystemOverhead: &cdiv1.FilesystemOverhead{
					StorageClass: map[string]cdiv1.Percent{
						"sc": "0.85",
					},
				},
			},
		},
	}
	assert.Equal(t, expectedCDI, updatedCDI)
	updatedSP, err := handler.storageProfileCache.Get("sc")
	assert.NoError(t, err)
	expectedSP := &cdiv1.StorageProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "sc"},
		Spec: cdiv1.StorageProfileSpec{
			CloneStrategy: ptr.To(cdiv1.CloneStrategySnapshot),
			SnapshotClass: ptr.To("snapclass"),
			ClaimPropertySets: []cdiv1.ClaimPropertySet{
				{VolumeMode: ptr.To(corev1.PersistentVolumeFilesystem), AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}},
			},
		},
	}
	assert.Equal(t, expectedSP, updatedSP)
}
