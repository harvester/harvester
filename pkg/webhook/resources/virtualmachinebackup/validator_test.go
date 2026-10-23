package virtualmachinebackup

import (
	"fmt"
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

func TestCheckBackupVolumeSnapshotClassLonghornV2(t *testing.T) {
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
		backupType  v1beta1.BackupType
		objects     []runtime.Object
		wantErr     bool
		errContains string
	}{
		{
			name:       "native backup rejects Longhorn v2 volume",
			backupType: v1beta1.Backup,
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
			name:       "allows snapshot of Longhorn v2 volume",
			backupType: v1beta1.Snapshot,
			objects: []runtime.Object{
				newCSIDriverConfigSetting(),
				newPVC(namespace, pvcName, pvName, longhornSC),
				newStorageClass(longhornSC, util.CSIProvisionerLonghorn),
				newLonghornVolume(pvName, longhornv1beta2.DataEngineTypeV2),
			},
		},
		{
			name:       "native backup allows Longhorn v1 volume",
			backupType: v1beta1.Backup,
			objects: []runtime.Object{
				newCSIDriverConfigSetting(),
				newPVC(namespace, pvcName, pvName, longhornSC),
				newStorageClass(longhornSC, util.CSIProvisionerLonghorn),
				newLonghornVolume(pvName, longhornv1beta2.DataEngineTypeV1),
			},
		},
		{
			name:       "allows snapshot of Longhorn v1 volume",
			backupType: v1beta1.Snapshot,
			objects: []runtime.Object{
				newCSIDriverConfigSetting(),
				newPVC(namespace, pvcName, pvName, longhornSC),
				newStorageClass(longhornSC, util.CSIProvisionerLonghorn),
				newLonghornVolume(pvName, longhornv1beta2.DataEngineTypeV1),
			},
		},
		{
			name:       "native backup ignores non-Longhorn volume",
			backupType: v1beta1.Backup,
			objects: []runtime.Object{
				newCSIDriverConfigSetting(),
				newPVC(namespace, pvcName, pvName, otherSC),
				newStorageClass(otherSC, "example.com/csi"),
			},
		},
		{
			name:       "data mover backup allows Longhorn v2 volume",
			backupType: v1beta1.Restic,
			objects: []runtime.Object{
				newCSIDriverConfigSetting(),
				newPVC(namespace, pvcName, pvName, longhornSC),
				newStorageClass(longhornSC, util.CSIProvisionerLonghorn),
				newLonghornVolume(pvName, longhornv1beta2.DataEngineTypeV2),
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

			err := validator.checkBackupVolumeSnapshotClass(
				newVM(namespace, vmName, pvcName),
				newVMBackup(namespace, vmName, tt.backupType),
			)
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

func TestCheckDataMoverVolumes(t *testing.T) {
	const (
		namespace  = "default"
		vmName     = "test-vm"
		longhornSC = "longhorn"
		lvmSC      = "lvm"
	)

	blockMode := corev1.PersistentVolumeBlock
	filesystemMode := corev1.PersistentVolumeFilesystem

	tests := []struct {
		name        string
		backupType  v1beta1.BackupType
		pvcNames    []string
		objects     []runtime.Object
		wantErr     bool
		errContains string
	}{
		{
			name:       "restic rejects filesystem volume",
			backupType: v1beta1.Restic,
			pvcNames:   []string{"lvm-pvc"},
			objects: []runtime.Object{
				newPVCWithMode(namespace, "lvm-pvc", "lvm-pv", lvmSC, &filesystemMode),
				newStorageClass(lvmSC, util.CSIProvisionerLVM),
			},
			wantErr:     true,
			errContains: "requires Block volume mode",
		},
		{
			name:       "kopia rejects default filesystem volume",
			backupType: v1beta1.Kopia,
			pvcNames:   []string{"lvm-pvc"},
			objects: []runtime.Object{
				newPVC(namespace, "lvm-pvc", "lvm-pv", lvmSC),
				newStorageClass(lvmSC, util.CSIProvisionerLVM),
			},
			wantErr:     true,
			errContains: "uses Filesystem",
		},
		{
			name:       "restic rejects Longhorn v1 volume",
			backupType: v1beta1.Restic,
			pvcNames:   []string{"longhorn-v1-pvc"},
			objects: []runtime.Object{
				newPVCWithMode(namespace, "longhorn-v1-pvc", "longhorn-v1-pv", longhornSC, &blockMode),
				newStorageClass(longhornSC, util.CSIProvisionerLonghorn),
				newLonghornVolume("longhorn-v1-pv", longhornv1beta2.DataEngineTypeV1),
			},
			wantErr:     true,
			errContains: "Longhorn v1 PVC default/longhorn-v1-pvc is not supported",
		},
		{
			name:       "kopia rejects Longhorn v1 among other volumes",
			backupType: v1beta1.Kopia,
			pvcNames:   []string{"longhorn-v2-pvc", "longhorn-v1-pvc"},
			objects: []runtime.Object{
				newPVCWithMode(namespace, "longhorn-v1-pvc", "longhorn-v1-pv", longhornSC, &blockMode),
				newPVCWithMode(namespace, "longhorn-v2-pvc", "longhorn-v2-pv", longhornSC, &blockMode),
				newStorageClass(longhornSC, util.CSIProvisionerLonghorn),
				newLonghornVolume("longhorn-v1-pv", longhornv1beta2.DataEngineTypeV1),
				newLonghornVolume("longhorn-v2-pv", longhornv1beta2.DataEngineTypeV2),
			},
			wantErr:     true,
			errContains: "Longhorn v1 PVC default/longhorn-v1-pvc is not supported",
		},
		{
			name:       "kopia allows Longhorn v2 and LVM volumes",
			backupType: v1beta1.Kopia,
			pvcNames:   []string{"longhorn-v2-pvc", "lvm-pvc"},
			objects: []runtime.Object{
				newPVCWithMode(namespace, "longhorn-v2-pvc", "longhorn-v2-pv", longhornSC, &blockMode),
				newPVCWithMode(namespace, "lvm-pvc", "lvm-pv", lvmSC, &blockMode),
				newStorageClass(longhornSC, util.CSIProvisionerLonghorn),
				newStorageClass(lvmSC, util.CSIProvisionerLVM),
				newLonghornVolume("longhorn-v2-pv", longhornv1beta2.DataEngineTypeV2),
			},
		},
		{
			name:       "native backup does not require block volumes",
			backupType: v1beta1.Backup,
			pvcNames:   []string{"lvm-pvc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tt.objects...)
			validator := &virtualMachineBackupValidator{
				pvcCache:    fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
				volumeCache: fakeclients.LonghornVolumeCache(clientset.LonghornV1beta2().Volumes),
				scCache:     fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
			}

			err := validator.checkDataMoverVolumes(newVM(namespace, vmName, tt.pvcNames...), tt.backupType)
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

func TestCheckBackupTarget(t *testing.T) {
	tests := []struct {
		name        string
		backupType  v1beta1.BackupType
		targetValue string
		wantErr     bool
		errContains string
	}{
		{
			name:        "restic rejects NFS target",
			backupType:  v1beta1.Restic,
			targetValue: `{"type":"nfs","endpoint":"server:/backups"}`,
			wantErr:     true,
			errContains: "restic backup requires an S3 backup target",
		},
		{
			name:        "kopia rejects NFS target",
			backupType:  v1beta1.Kopia,
			targetValue: `{"type":"nfs","endpoint":"server:/backups"}`,
			wantErr:     true,
			errContains: "kopia backup requires an S3 backup target",
		},
		{
			name:        "native backup allows NFS target",
			backupType:  v1beta1.Backup,
			targetValue: `{"type":"nfs","endpoint":"server:/backups"}`,
		},
		{
			name:        "restic allows S3 target",
			backupType:  v1beta1.Restic,
			targetValue: `{"type":"s3","endpoint":"https://s3.example.com"}`,
		},
		{
			name:        "kopia allows S3 target",
			backupType:  v1beta1.Kopia,
			targetValue: `{"type":"s3","endpoint":"https://s3.example.com"}`,
		},
		{
			name:        "data mover rejects missing target",
			backupType:  v1beta1.Restic,
			targetValue: `{}`,
			wantErr:     true,
			errContains: "backup target is not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setting := &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.BackupTargetSettingName},
				Value:      tt.targetValue,
			}
			clientset := fake.NewSimpleClientset(setting)
			validator := &virtualMachineBackupValidator{
				setting: fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
			}

			err := validator.checkBackupTarget(tt.backupType)
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

func newVMBackup(namespace, sourceName string, backupType v1beta1.BackupType) *v1beta1.VirtualMachineBackup {
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
			Type: backupType,
		},
	}
}

func newVM(namespace, name string, pvcNames ...string) *kubevirtv1.VirtualMachine {
	volumes := make([]kubevirtv1.Volume, 0, len(pvcNames))
	for i, pvcName := range pvcNames {
		volumes = append(volumes, kubevirtv1.Volume{
			Name: fmt.Sprintf("disk-%d", i),
			VolumeSource: kubevirtv1.VolumeSource{
				PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvcName,
					},
				},
			},
		})
	}

	return &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{Volumes: volumes},
			},
		},
	}
}

func newPVC(namespace, name, volumeName, storageClassName string) *corev1.PersistentVolumeClaim {
	return newPVCWithMode(namespace, name, volumeName, storageClassName, nil)
}

func newPVCWithMode(
	namespace, name, volumeName, storageClassName string,
	volumeMode *corev1.PersistentVolumeMode,
) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClassName,
			VolumeName:       volumeName,
			VolumeMode:       volumeMode,
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
