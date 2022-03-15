package builder

import (
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"
)

const (
	StorageClassNamePrefix = "longhorn"

	DiskTypeDisk  = "disk"
	DiskTypeCDRom = "cd-rom"

	DiskBusVirtio = "virtio"
	DiskBusScsi   = "scsi"
	DiskBusSata   = "sata"

	PersistentVolumeModeBlock      = "Block"
	PersistentVolumeModeFilesystem = "Filesystem"

	PersistentVolumeAccessModeReadWriteOnce = "ReadWriteOnce"
	PersistentVolumeAccessModeReadOnlyMany  = "ReadOnlyMany"
	PersistentVolumeAccessModeReadWriteMany = "ReadWriteMany"

	DefaultDiskSize        = "10Gi"
	DefaultImagePullPolicy = "IfNotPresent"
)

type PersistentVolumeClaimOption struct {
	ImageID          string
	VolumeMode       corev1.PersistentVolumeMode
	AccessMode       corev1.PersistentVolumeAccessMode
	StorageClassName *string
	Annotations      map[string]string
}

func UintPtr(in int) *uint {
	var out *uint
	u := uint(in)
	if in > 0 {
		out = &u
	}
	return out
}

func BuildImageStorageClassName(namespace, name string) string {
	if namespace != "" {
		return StorageClassNamePrefix + "-" + namespace + "-" + name
	}
	return StorageClassNamePrefix + "-" + name
}

func (v *VMBuilder) Disk(diskName, diskBus string, isCDRom bool, bootOrder int) *VMBuilder {
	var (
		exist bool
		index int
		disks = v.VirtualMachine.Spec.Template.Spec.Domain.Devices.Disks
	)
	for i, disk := range disks {
		if disk.Name == diskName {
			exist = true
			index = i
			break
		}
	}
	diskDevice := kubevirtv1.DiskDevice{
		Disk: &kubevirtv1.DiskTarget{
			Bus: diskBus,
		},
	}
	if isCDRom {
		diskDevice = kubevirtv1.DiskDevice{
			CDRom: &kubevirtv1.CDRomTarget{
				Bus: diskBus,
			},
		}
	}
	disk := kubevirtv1.Disk{
		Name:       diskName,
		BootOrder:  UintPtr(bootOrder),
		DiskDevice: diskDevice,
	}
	if exist {
		disks[index] = disk
	} else {
		disks = append(disks, disk)
	}
	v.VirtualMachine.Spec.Template.Spec.Domain.Devices.Disks = disks
	return v
}

func (v *VMBuilder) Volume(diskName string, volume kubevirtv1.Volume) *VMBuilder {
	var (
		exist   bool
		index   int
		volumes = v.VirtualMachine.Spec.Template.Spec.Volumes
	)
	for i, e := range volumes {
		if e.Name == diskName {
			exist = true
			index = i
			break
		}
	}

	if exist {
		volumes[index] = volume
	} else {
		volumes = append(volumes, volume)
	}
	v.VirtualMachine.Spec.Template.Spec.Volumes = volumes
	return v
}

func (v *VMBuilder) ExistingPVCVolume(diskName, pvcName string, hotpluggable bool) *VMBuilder {
	return v.Volume(diskName, kubevirtv1.Volume{
		Name: diskName,
		VolumeSource: kubevirtv1.VolumeSource{
			PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
				Hotpluggable: hotpluggable,
			},
		},
	})
}

func (v *VMBuilder) ExistingVolumeDisk(diskName, diskBus string, isCDRom, hotpluggable bool, bootOrder int, pvcName string) *VMBuilder {
	return v.Disk(diskName, diskBus, isCDRom, bootOrder).ExistingPVCVolume(diskName, pvcName, hotpluggable)
}

func (v *VMBuilder) ContainerDiskVolume(diskName, imageName, ImagePullPolicy string) *VMBuilder {
	return v.Volume(diskName, kubevirtv1.Volume{
		Name: diskName,
		VolumeSource: kubevirtv1.VolumeSource{
			ContainerDisk: &kubevirtv1.ContainerDiskSource{
				Image:           imageName,
				ImagePullPolicy: corev1.PullPolicy(ImagePullPolicy),
			},
		},
	})
}

func (v *VMBuilder) ContainerDisk(diskName, diskBus string, isCDRom bool, bootOrder int, imageName, ImagePullPolicy string) *VMBuilder {
	return v.Disk(diskName, diskBus, isCDRom, bootOrder).ContainerDiskVolume(diskName, imageName, ImagePullPolicy)
}

func (v *VMBuilder) PVCVolume(diskName, diskSize, pvcName string, hotpluggable bool, opt *PersistentVolumeClaimOption) *VMBuilder {
	if opt == nil {
		defaultStorageClass := "longhorn"
		opt = &PersistentVolumeClaimOption{
			VolumeMode:       corev1.PersistentVolumeBlock,
			AccessMode:       corev1.ReadWriteMany,
			StorageClassName: &defaultStorageClass,
		}
	}

	if pvcName == "" {
		pvcName = fmt.Sprintf("%s-%s-%s", v.VirtualMachine.Name, diskName, rand.String(5))
	}

	var pvcs []*corev1.PersistentVolumeClaim
	volumeClaimTemplates, ok := v.VirtualMachine.Annotations[util.AnnotationVolumeClaimTemplates]
	if ok && volumeClaimTemplates != "" {
		if err := json.Unmarshal([]byte(volumeClaimTemplates), &pvcs); err != nil {
			logrus.Warnf("failed to unmarshal the volumeClaimTemplates annotation: %v", err)
		}
	}
	if opt.Annotations == nil {
		opt.Annotations = map[string]string{}
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pvcName,
			Annotations: opt.Annotations,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				opt.AccessMode,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(diskSize),
				},
			},
			VolumeMode:       &opt.VolumeMode,
			StorageClassName: opt.StorageClassName,
		},
	}
	if opt.ImageID != "" {
		pvc.Annotations[AnnotationKeyImageID] = opt.ImageID
	}

	pvcs = append(pvcs, pvc)

	toUpdateVolumeClaimTemplates, err := json.Marshal(pvcs)
	if err != nil {
		logrus.Warnf("failed to marshal the volumeClaimTemplates annotation: %v", err)
	} else {
		v.VirtualMachine.Annotations[util.AnnotationVolumeClaimTemplates] = string(toUpdateVolumeClaimTemplates)
	}

	return v.ExistingPVCVolume(diskName, pvcName, hotpluggable)
}

func (v *VMBuilder) PVCDisk(diskName, diskBus string, isCDRom, hotpluggable bool, bootOrder int, diskSize, pvcName string, opt *PersistentVolumeClaimOption) *VMBuilder {
	return v.Disk(diskName, diskBus, isCDRom, bootOrder).PVCVolume(diskName, diskSize, pvcName, hotpluggable, opt)
}
