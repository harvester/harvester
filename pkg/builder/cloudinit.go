package builder

import (
	corev1 "k8s.io/api/core/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

const (
	CloudInitTypeNoCloud     = "noCloud"
	CloudInitTypeConfigDrive = "configDrive"
	CloudInitDiskName        = "cloudinitdisk"
)

type CloudInitSource struct {
	CloudInitType         string
	UserDataSecretName    string
	UserDataBase64        string
	UserData              string
	NetworkDataSecretName string
	NetworkDataBase64     string
	NetworkData           string
}

func (v *VMBuilder) CloudInit(diskName string, cloudInitSource CloudInitSource) *VMBuilder {
	var volume kubevirtv1.Volume
	switch cloudInitSource.CloudInitType {
	case CloudInitTypeNoCloud:
		volume = kubevirtv1.Volume{
			Name: diskName,
			VolumeSource: kubevirtv1.VolumeSource{
				CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{
					UserData:          cloudInitSource.UserData,
					UserDataBase64:    cloudInitSource.UserDataBase64,
					NetworkData:       cloudInitSource.NetworkData,
					NetworkDataBase64: cloudInitSource.NetworkDataBase64,
				},
			},
		}
		if cloudInitSource.UserDataSecretName != "" {
			volume.VolumeSource.CloudInitNoCloud.UserDataSecretRef = &corev1.LocalObjectReference{
				Name: cloudInitSource.UserDataSecretName,
			}
		}
		if cloudInitSource.NetworkDataSecretName != "" {
			volume.VolumeSource.CloudInitNoCloud.NetworkDataSecretRef = &corev1.LocalObjectReference{
				Name: cloudInitSource.NetworkDataSecretName,
			}
		}
	case CloudInitTypeConfigDrive:
		volume = kubevirtv1.Volume{
			Name: diskName,
			VolumeSource: kubevirtv1.VolumeSource{
				CloudInitConfigDrive: &kubevirtv1.CloudInitConfigDriveSource{
					UserData:          cloudInitSource.UserData,
					UserDataBase64:    cloudInitSource.UserDataBase64,
					NetworkData:       cloudInitSource.NetworkData,
					NetworkDataBase64: cloudInitSource.NetworkDataBase64,
				},
			},
		}
		if cloudInitSource.UserDataSecretName != "" {
			volume.VolumeSource.CloudInitConfigDrive.UserDataSecretRef = &corev1.LocalObjectReference{
				Name: cloudInitSource.UserDataSecretName,
			}
		}
		if cloudInitSource.NetworkDataSecretName != "" {
			volume.VolumeSource.CloudInitConfigDrive.NetworkDataSecretRef = &corev1.LocalObjectReference{
				Name: cloudInitSource.NetworkDataSecretName,
			}
		}
	}
	v.Volume(diskName, volume)
	return v
}

func (v *VMBuilder) CloudInitDisk(diskName, diskBus string, isCDRom bool, bootOrder uint, cloudInitSource CloudInitSource) *VMBuilder {
	return v.Disk(diskName, diskBus, isCDRom, bootOrder).CloudInit(diskName, cloudInitSource)
}

func (v *VMBuilder) SSHKey(sshKeyName string) *VMBuilder {
	v.SSHNames = append(v.SSHNames, sshKeyName)
	return v
}
