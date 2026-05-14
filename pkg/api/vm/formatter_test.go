package vm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestCanDoBackup(t *testing.T) {
	longhornSC := "longhorn"
	lvmSC := "lvm"
	backupTarget := `{"type":"s3","endpoint":"https://s3.amazonaws.com","bucketName":"test","bucketRegion":"us-west-1"}`

	tests := []struct {
		name           string
		pvcNames       []string                        // PVC names to attach to VM
		pvcs           []*corev1.PersistentVolumeClaim // PVCs to create
		storageClasses []*storagev1.StorageClass       // Storage classes to create
		expected       bool
	}{
		{
			name:     "explicit Longhorn SC - allow backup",
			pvcNames: []string{"pvc1"},
			pvcs:     []*corev1.PersistentVolumeClaim{newTestPVC("pvc1", &longhornSC)},
			storageClasses: []*storagev1.StorageClass{
				newTestStorageClass(longhornSC, util.CSIProvisionerLonghorn, false),
			},
			expected: true,
		},
		{
			name:     "explicit LVM SC - block backup",
			pvcNames: []string{"pvc1"},
			pvcs:     []*corev1.PersistentVolumeClaim{newTestPVC("pvc1", &lvmSC)},
			storageClasses: []*storagev1.StorageClass{
				newTestStorageClass(lvmSC, util.CSIProvisionerLVM, false),
			},
			expected: false,
		},
		{
			name:     "default Longhorn SC - allow backup",
			pvcNames: []string{"pvc1"},
			pvcs:     []*corev1.PersistentVolumeClaim{newTestPVC("pvc1", nil)},
			storageClasses: []*storagev1.StorageClass{
				newTestStorageClass(longhornSC, util.CSIProvisionerLonghorn, true),
			},
			expected: true,
		},
		{
			name:     "default LVM SC - block backup",
			pvcNames: []string{"pvc1"},
			pvcs:     []*corev1.PersistentVolumeClaim{newTestPVC("pvc1", nil)},
			storageClasses: []*storagev1.StorageClass{
				newTestStorageClass(lvmSC, util.CSIProvisionerLVM, true),
			},
			expected: false,
		},
		{
			name:           "no SC and no default - block backup",
			pvcNames:       []string{"pvc1"},
			pvcs:           []*corev1.PersistentVolumeClaim{newTestPVC("pvc1", nil)},
			storageClasses: []*storagev1.StorageClass{},
			expected:       false,
		},
		{
			name:     "mixed volumes (Longhorn + LVM) - block backup",
			pvcNames: []string{"pvc1", "pvc2"},
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC("pvc1", &longhornSC),
				newTestPVC("pvc2", &lvmSC),
			},
			storageClasses: []*storagev1.StorageClass{
				newTestStorageClass(longhornSC, util.CSIProvisionerLonghorn, false),
				newTestStorageClass(lvmSC, util.CSIProvisionerLVM, false),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build objects for fake clientset
			objs := make([]runtime.Object, 0, 1+len(tt.pvcs)+len(tt.storageClasses))
			objs = append(objs, &harvesterv1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.BackupTargetSettingName},
				Value:      backupTarget,
			})
			for _, pvc := range tt.pvcs {
				objs = append(objs, pvc)
			}
			for _, sc := range tt.storageClasses {
				objs = append(objs, sc)
			}

			clientset := fake.NewSimpleClientset(objs...)
			vf := &vmformatter{
				pvcCache:     fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
				scCache:      fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
				settingCache: fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
			}

			vm := newTestVM(tt.pvcNames...)
			vmi := newTestVMI()
			result := vf.canDoBackup(vm, vmi)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCanDoSnapshot(t *testing.T) {
	const (
		cephSC          = "fast-ceph"
		cephProvisioner = "rbd.csi.ceph.com"
	)

	tests := []struct {
		name           string
		pvcNames       []string
		pvcs           []*corev1.PersistentVolumeClaim
		storageClasses []*storagev1.StorageClass
		expected       bool
	}{
		{
			name:     "storage class snapshot class enables snapshot for external CSI",
			pvcNames: []string{"pvc1"},
			pvcs:     []*corev1.PersistentVolumeClaim{newTestPVC("pvc1", ptr.To(cephSC))},
			storageClasses: []*storagev1.StorageClass{
				newTestStorageClassWithAnnotations(cephSC, cephProvisioner, false, map[string]string{
					util.AnnotationStorageProfileSnapshotClass: "fast-ceph-snapclass",
				}),
			},
			expected: true,
		},
		{
			name:     "external CSI without snapshot class stays disabled",
			pvcNames: []string{"pvc1"},
			pvcs:     []*corev1.PersistentVolumeClaim{newTestPVC("pvc1", ptr.To(cephSC))},
			storageClasses: []*storagev1.StorageClass{
				newTestStorageClass(cephSC, cephProvisioner, false),
			},
			expected: false,
		},
		{
			name:     "mixed volumes block snapshot when one PVC has no snapshot class",
			pvcNames: []string{"pvc1", "pvc2"},
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC("pvc1", ptr.To(cephSC)),
				newTestPVC("pvc2", ptr.To("standard-ceph")),
			},
			storageClasses: []*storagev1.StorageClass{
				newTestStorageClassWithAnnotations(cephSC, cephProvisioner, false, map[string]string{
					util.AnnotationStorageProfileSnapshotClass: "fast-ceph-snapclass",
				}),
				newTestStorageClass("standard-ceph", cephProvisioner, false),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := make([]runtime.Object, 0, len(tt.pvcs)+len(tt.storageClasses))
			for _, pvc := range tt.pvcs {
				objs = append(objs, pvc)
			}
			for _, sc := range tt.storageClasses {
				objs = append(objs, sc)
			}

			clientset := fake.NewSimpleClientset(objs...)
			vf := &vmformatter{
				pvcCache: fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
				scCache:  fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
			}

			vm := newTestVM(tt.pvcNames...)
			vmi := newTestVMI()
			result := vf.canDoSnapshot(vm, vmi)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test helpers
func newTestVM(pvcNames ...string) *kubevirtv1.VirtualMachine {
	volumes := make([]kubevirtv1.Volume, 0, len(pvcNames))
	for i, name := range pvcNames {
		volumes = append(volumes, kubevirtv1.Volume{
			Name: "disk-" + string(rune('0'+i)),
			VolumeSource: kubevirtv1.VolumeSource{
				PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: name,
					},
				},
			},
		})
	}
	return &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default"},
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{Volumes: volumes},
			},
		},
	}
}

func newTestVMI() *kubevirtv1.VirtualMachineInstance {
	return &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default"},
		Status:     kubevirtv1.VirtualMachineInstanceStatus{Phase: kubevirtv1.Running},
	}
}

func newTestPVC(name string, scName *string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       corev1.PersistentVolumeClaimSpec{StorageClassName: scName},
	}
}

func newTestStorageClass(name, provisioner string, isDefault bool) *storagev1.StorageClass {
	return newTestStorageClassWithAnnotations(name, provisioner, isDefault, nil)
}

func newTestStorageClassWithAnnotations(name, provisioner string, isDefault bool, annotations map[string]string) *storagev1.StorageClass {
	sc := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: name, Annotations: annotations},
		Provisioner: provisioner,
	}
	if isDefault {
		if sc.Annotations == nil {
			sc.Annotations = map[string]string{}
		}
		sc.Annotations[util.AnnotationIsDefaultStorageClassName] = "true"
	}
	return sc
}
