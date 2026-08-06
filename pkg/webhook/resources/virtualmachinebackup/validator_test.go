package virtualmachinebackup

import (
	"strings"
	"testing"

	longhornv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestCheckBackupVolumeSnapshotClassRejectsLonghornV2Volumes(t *testing.T) {
	const (
		namespace = "default"
		vmName    = "test-vm"
		pvcName   = "test-pvc"
		pvName    = "pvc-123"
	)

	longhornSC := "longhorn"
	otherSC := "other"

	tests := []struct {
		name        string
		objects     []runtime.Object
		wantErr     bool
		errContains string
	}{
		{
			name: "rejects Longhorn v2 volume",
			objects: []runtime.Object{
				newCSIDriverConfigSetting(),
				newPVC(namespace, pvcName, pvName, longhornSC),
				newStorageClass(longhornSC, util.CSIProvisionerLonghorn),
				newLonghornVolume(pvName, longhornv1beta2.DataEngineTypeV2),
			},
			wantErr:     true,
			errContains: "contains Longhorn v2 volume",
		},
		{
			name: "allows Longhorn v1 volume",
			objects: []runtime.Object{
				newCSIDriverConfigSetting(),
				newPVC(namespace, pvcName, pvName, longhornSC),
				newStorageClass(longhornSC, util.CSIProvisionerLonghorn),
				newLonghornVolume(pvName, longhornv1beta2.DataEngineTypeV1),
			},
		},
		{
			name: "ignores non-Longhorn volume",
			objects: []runtime.Object{
				newCSIDriverConfigSetting(),
				newPVC(namespace, pvcName, pvName, otherSC),
				newStorageClass(otherSC, "example.com/csi"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tt.objects...)
			validator := &virtualMachineBackupValidator{
				setting:     fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
				pvcCache:    fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
				volumeCache: fakeclients.LonghornVolumeCache(clientset.LonghornV1beta2().Volumes),
				scCache:     fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
				vmbr:        common.NewVMBackupReader(),
			}

			err := validator.checkBackupVolumeSnapshotClass(newVM(namespace, vmName, pvcName), newVMBackup(namespace, vmName))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error to contain %q, got %q", tt.errContains, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func newVMBackup(namespace, sourceName string) *v1beta1.VirtualMachineBackup {
	return &v1beta1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "test-backup",
		},
		Spec: v1beta1.VirtualMachineBackupSpec{
			Source: corev1.TypedLocalObjectReference{
				APIGroup: &v1beta1.SchemeGroupVersion.Group,
				Kind:     "VirtualMachine",
				Name:     sourceName,
			},
			Type: v1beta1.Backup,
		},
	}
}

func newVM(namespace, name, pvcName string) *kubevirtv1.VirtualMachine {
	return &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Volumes: []kubevirtv1.Volume{
						{
							Name: "disk-0",
							VolumeSource: kubevirtv1.VolumeSource{
								PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: pvcName,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func newPVC(namespace, name, volumeName, storageClassName string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClassName,
			VolumeName:       volumeName,
		},
	}
}

func newStorageClass(name, provisioner string) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner: provisioner,
	}
}

func newCSIDriverConfigSetting() *v1beta1.Setting {
	return &v1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.CSIDriverConfigSettingName,
		},
		Default: `{"driver.longhorn.io":{"volumeSnapshotClassName":"longhorn-snapshot","backupVolumeSnapshotClassName":"longhorn"},"example.com/csi":{"volumeSnapshotClassName":"example-snapshot","backupVolumeSnapshotClassName":"example-backup"}}`,
	}
}

func newLonghornVolume(name string, dataEngine longhornv1beta2.DataEngineType) *longhornv1beta2.Volume {
	return &longhornv1beta2.Volume{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: util.LonghornSystemNamespaceName,
			Name:      name,
		},
		Spec: longhornv1beta2.VolumeSpec{
			DataEngine: dataEngine,
		},
	}
}
