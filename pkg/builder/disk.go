package builder

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	kubevirtv1 "kubevirt.io/client-go/api/v1"
	cdiv1 "kubevirt.io/containerized-data-importer/pkg/apis/core/v1beta1"
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

type DataVolumeOption struct {
	ImageID          string
	DownloadURL      string
	VolumeMode       corev1.PersistentVolumeMode
	AccessMode       corev1.PersistentVolumeAccessMode
	StorageClassName *string
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

func (v *VMBuilder) ExistingDataVolume(diskName, dataVolumeName string) *VMBuilder {
	return v.Volume(diskName, kubevirtv1.Volume{
		Name: diskName,
		VolumeSource: kubevirtv1.VolumeSource{
			DataVolume: &kubevirtv1.DataVolumeSource{
				Name: dataVolumeName,
			},
		},
	})
}

func (v *VMBuilder) ExistingVolumeDisk(diskName, diskBus string, isCDRom bool, bootOrder int, dataVolumeName string) *VMBuilder {
	return v.Disk(diskName, diskBus, isCDRom, bootOrder).ExistingDataVolume(diskName, dataVolumeName)
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

func (v *VMBuilder) DataVolume(diskName, diskSize, dataVolumeName string, opt *DataVolumeOption) *VMBuilder {
	if opt == nil {
		opt = &DataVolumeOption{
			VolumeMode: corev1.PersistentVolumeBlock,
			AccessMode: corev1.ReadWriteMany,
		}
	}
	if dataVolumeName == "" {
		dataVolumeName = fmt.Sprintf("%s-%s-%s", v.VirtualMachine.Name, diskName, rand.String(5))
	}
	// DataVolumeTemplates
	dataVolumeTemplates := v.VirtualMachine.Spec.DataVolumeTemplates
	dataVolumeSpecSource := cdiv1.DataVolumeSource{
		Blank: &cdiv1.DataVolumeBlankImage{},
	}

	if opt.DownloadURL != "" {
		dataVolumeSpecSource = cdiv1.DataVolumeSource{
			HTTP: &cdiv1.DataVolumeSourceHTTP{
				URL: opt.DownloadURL,
			},
		}
	}
	dataVolumeTemplate := kubevirtv1.DataVolumeTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name: dataVolumeName,
		},
		Spec: cdiv1.DataVolumeSpec{
			Source: &dataVolumeSpecSource,
			PVC: &corev1.PersistentVolumeClaimSpec{
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
		},
	}
	if opt.ImageID != "" {
		dataVolumeTemplate.Annotations = map[string]string{
			AnnotationKeyImageID: opt.ImageID,
		}
	}
	dataVolumeTemplates = append(dataVolumeTemplates, dataVolumeTemplate)
	v.VirtualMachine.Spec.DataVolumeTemplates = dataVolumeTemplates

	return v.ExistingDataVolume(diskName, dataVolumeName)
}

func (v *VMBuilder) DataVolumeDisk(diskName, diskBus string, isCDRom bool, bootOrder int, diskSize, dataVolumeName string, opt *DataVolumeOption) *VMBuilder {
	return v.Disk(diskName, diskBus, isCDRom, bootOrder).DataVolume(diskName, diskSize, dataVolumeName, opt)
}
