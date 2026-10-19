package vm

import (
	"testing"

	longhornv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdicommon "kubevirt.io/containerized-data-importer/pkg/controller/common"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestCanDoBackupLonghornV2Volumes(t *testing.T) {
	const (
		namespace   = "default"
		vmName      = "test-vm"
		pvcName     = "test-pvc"
		pvName      = "pvc-test"
		longhornSC  = "longhorn-sc"
		otherSC     = "other-sc"
		otherDriver = "example.com/csi"
	)

	newObjects := func(extra ...runtime.Object) []runtime.Object {
		return append([]runtime.Object{
			newBackupTargetSetting(`{"type":"s3","endpoint":"http://s3:9000","bucketName":"backup","bucketRegion":"us-east-1"}`),
			newStorageClass(longhornSC, util.CSIProvisionerLonghorn),
			newStorageClass(otherSC, otherDriver),
		}, extra...)
	}

	tests := []struct {
		name    string
		objects []runtime.Object
		want    bool
	}{
		{
			name: "offers backup for Longhorn v1 volume",
			objects: newObjects(
				newPVC(namespace, pvcName, pvName, longhornSC, nil),
				newLonghornVolume(pvName, longhornv1beta2.DataEngineTypeV1),
			),
			want: true,
		},
		{
			name: "does not offer backup for Longhorn v2 volume",
			objects: newObjects(
				newPVC(namespace, pvcName, pvName, longhornSC, nil),
				newLonghornVolume(pvName, longhornv1beta2.DataEngineTypeV2),
			),
			want: false,
		},
		{
			name: "offers backup for non-Longhorn volume",
			objects: newObjects(
				newPVC(namespace, pvcName, pvName, otherSC, nil),
			),
			want: true,
		},
		{
			name: "offers backup for Longhorn PVC that is not bound yet",
			objects: newObjects(
				newPVC(namespace, pvcName, "", longhornSC, nil),
			),
			want: true,
		},
		{
			name: "does not offer backup if the Longhorn volume cannot be found",
			objects: newObjects(
				newPVC(namespace, pvcName, pvName, longhornSC, nil),
			),
			want: false,
		},
		{
			name: "does not offer backup for volume created for a CDI DataVolume",
			objects: newObjects(
				newPVC(namespace, pvcName, pvName, longhornSC, map[string]string{
					cdicommon.AnnCreatedForDataVolume: "dv-uid",
				}),
				newLonghornVolume(pvName, longhornv1beta2.DataEngineTypeV1),
			),
			want: false,
		},
		{
			name: "does not offer backup without backup target",
			objects: []runtime.Object{
				newBackupTargetSetting(`{}`),
				newStorageClass(longhornSC, util.CSIProvisionerLonghorn),
				newPVC(namespace, pvcName, pvName, longhornSC, nil),
				newLonghornVolume(pvName, longhornv1beta2.DataEngineTypeV1),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tt.objects...)
			vf := &vmformatter{
				settingCache: fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
				pvcCache:     fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
				scCache:      fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
				volumeCache:  fakeclients.LonghornVolumeCache(clientset.LonghornV1beta2().Volumes),
			}

			got := vf.canDoBackup(newVM(namespace, vmName, pvcName), nil)
			assert.Equal(t, tt.want, got)
		})
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

func newPVC(namespace, name, volumeName, storageClassName string, annotations map[string]string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			Annotations: annotations,
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

func newBackupTargetSetting(value string) *v1beta1.Setting {
	return &v1beta1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: settings.BackupTargetSettingName,
		},
		Value: value,
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
