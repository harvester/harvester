package vm

import (
	"encoding/json"
	"fmt"
	"strings"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/util/retry"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdicommon "kubevirt.io/containerized-data-importer/pkg/controller/common"
)

// createTemplate creates a VirtualMachineTemplate and VirtualMachineTemplateVersion
// that are derived from the given VirtualMachine.
func (h *vmActionHandler) createTemplate(namespace, name string, input CreateTemplateInput) (err error) {
	var (
		keyPairIDs []string
		vm         *kubevirtv1.VirtualMachine
		vmt        *harvesterv1.VirtualMachineTemplate
		vmtv       *harvesterv1.VirtualMachineTemplateVersion
	)
	defer func() {
		var cleanupErr error
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"namespace": namespace,
				"name":      name,
			}).Errorf("failed to create VM Template and associated resources: %v", err)

			if cleanupErr = h.cleanupVMTemplateVersion(vmtv); cleanupErr != nil {
				err = fmt.Errorf("failed to clean up VM template version while dealing with error %w: %s", err, cleanupErr.Error())
			}

			if cleanupErr = h.cleanupVMTemplate(vmt); cleanupErr != nil {
				err = fmt.Errorf("failed to clean up VM template while dealing with error %w: %s", err, cleanupErr.Error())
			}
		}
	}()

	vm, err = h.vmCache.Get(namespace, name)
	if err != nil {
		return err
	}

	keyPairIDs, err = getSSHKeysFromVMITemplateSpec(vm.Spec.Template)
	if err != nil {
		return err
	}

	vmt, err = h.createVMTemplate(namespace, input)
	if err != nil {
		return err
	}

	vmtv, err = h.createVMTemplateVersion(namespace, vm, vmt, keyPairIDs)
	if err != nil {
		return err
	}

	if input.WithData {
		vmtv, err = h.createTemplateWithData(vm, vmtv)
		if err != nil {
			return err
		}
	}

	return h.createSecrets(vmtv, vm)
}

func (h *vmActionHandler) createTemplateWithData(vm *kubevirtv1.VirtualMachine, vmtvIn *harvesterv1.VirtualMachineTemplateVersion) (vmtv *harvesterv1.VirtualMachineTemplateVersion, err error) {
	var (
		pvcMap             map[string]corev1.PersistentVolumeClaim
		pvcStorageClassMap map[string]string
		vmImageMap         map[string]harvesterv1.VirtualMachineImage
	)

	pvcMap, pvcStorageClassMap, err = h.getPVCStorageClassMap(vm)
	if err != nil {
		return vmtv, err
	}
	vmImageMap, err = h.createVMImages(vmtv, vm, pvcStorageClassMap)
	if err != nil {
		return vmtv, err
	}
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		vmtv, err = h.vmTemplateVersionClient.Get(vmtv.Namespace, vmtv.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		return h.sanitizeVolumes(vmtv, pvcMap, vmImageMap)
	})
	if err != nil {
		return vmtv, err
	}
	return vmtv, nil
}

func (h *vmActionHandler) getPVCStorageClassMap(vm *kubevirtv1.VirtualMachine) (map[string]corev1.PersistentVolumeClaim, map[string]string, error) {
	pvcMap := map[string]corev1.PersistentVolumeClaim{}
	pvcStorageClassMap := map[string]string{}
	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			continue // Skip if the volume does not reference a PVC
		}

		pvc, err := h.pvcCache.Get(vm.Namespace, volume.PersistentVolumeClaim.ClaimName)
		if err != nil {
			return pvcMap, pvcStorageClassMap, err
		}
		pvcMap[pvc.Name] = *pvc

		scName, err := h.getStorageClassNameFromPVCOrBackingImage(pvc)
		if err != nil {
			return pvcMap, pvcStorageClassMap, err
		}
		pvcStorageClassMap[pvc.Name] = scName
	}
	return pvcMap, pvcStorageClassMap, nil
}

// If the PVC has an annoation, which associates it with a BackingImage, then
// this function will return the StorageClass name of the BackingImage. Otherwise it
// will return the StorageClass name of the PVC itself.
func (h *vmActionHandler) getStorageClassNameFromPVCOrBackingImage(pvc *corev1.PersistentVolumeClaim) (scName string, err error) {
	if imageId, ok := pvc.Annotations[util.AnnotationImageID]; ok {
		return h.getStorageClassNameFromImageID(imageId)
	}
	return *pvc.Spec.StorageClassName, nil
}

// Given an ImageID, find the corresponding VMImage and figure out it's
// StorageClass name
func (h *vmActionHandler) getStorageClassNameFromImageID(imageId string) (scName string, err error) {
	imageIdSplit := strings.Split(imageId, "/")
	if len(imageIdSplit) == 2 {
		vmImage, err := h.vmImageCache.Get(imageIdSplit[0], imageIdSplit[1])
		if err != nil {
			return "", err
		}

		if scName, ok := vmImage.Annotations[util.AnnotationStorageClassName]; ok {
			return scName, nil
		} else {
			return "", fmt.Errorf("VMImage %s/%s does not have an annotation %s", vmImage.Namespace, vmImage.Name, util.AnnotationStorageClassName)
		}
	}
	return "", fmt.Errorf("malformed image ID: %s", imageId)
}

func (h *vmActionHandler) sanitizeVolumes(templateVersion *harvesterv1.VirtualMachineTemplateVersion, pvcMap map[string]corev1.PersistentVolumeClaim, vmImageMap map[string]harvesterv1.VirtualMachineImage) error {
	templateVersionCopy := templateVersion.DeepCopy()
	vmSource := &templateVersionCopy.Spec.VM
	volumeClaimTemplates := []corev1.PersistentVolumeClaim{}
	for index, volume := range vmSource.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			continue
		}

		volumeClaimTemplate := generateVolumeClaimTemplate(index, templateVersion.Name, templateVersion.Namespace, volume.PersistentVolumeClaim.ClaimName, pvcMap, vmImageMap)
		volumeClaimTemplates = append(volumeClaimTemplates, volumeClaimTemplate)
		vmSource.Spec.Template.Spec.Volumes[index].PersistentVolumeClaim.ClaimName = volumeClaimTemplate.Name
	}

	volumeCliamTemplatesJSON, err := json.Marshal(volumeClaimTemplates)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"namespace":       templateVersion.Namespace,
			"templateVersion": templateVersion.Name,
			"volumeClaim":     volumeClaimTemplates,
		}).Error("Failed to json.Marshal volume claim templates")
		return err
	}

	if vmSource.ObjectMeta.Annotations == nil {
		vmSource.ObjectMeta.Annotations = map[string]string{}
	}
	vmSource.ObjectMeta.Annotations[util.AnnotationVolumeClaimTemplates] = string(volumeCliamTemplatesJSON)
	if _, err := h.vmTemplateVersionClient.Update(templateVersionCopy); err != nil {
		return err
	}
	return nil
}

func generateVolumeClaimTemplate(index int, tvName, tvNamespace, claimName string, pvcMap map[string]corev1.PersistentVolumeClaim, vmImageMap map[string]harvesterv1.VirtualMachineImage) (pvc corev1.PersistentVolumeClaim) {
	// generate new volume template
	// - longhorn v1, we need to use the new storageclass Name (for backingImage)
	// - others, use the pvc StorageClassName
	vmImageName := getTemplateVersionVMImageName(tvName, index)
	vmImage := vmImageMap[vmImageName]

	claim := pvcMap[claimName]
	targetStorageClassName := util.GenerateStorageClassName(string(vmImage.UID))
	if _, ok := pvc.Annotations[cdicommon.AnnCreatedForDataVolume]; ok {
		targetStorageClassName = *claim.Spec.StorageClassName
	}
	pvcName := getTemplateVersionPvcName(tvName, index)

	pvc = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvcName,
			Annotations: map[string]string{
				util.AnnotationImageID: fmt.Sprintf("%s/%s", tvNamespace, vmImageName),
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      claim.Spec.AccessModes,
			Resources:        claim.Spec.Resources,
			VolumeMode:       claim.Spec.VolumeMode,
			StorageClassName: &targetStorageClassName,
		},
	}
	return pvc
}

func (h *vmActionHandler) createSecrets(templateVersion *harvesterv1.VirtualMachineTemplateVersion, vm *kubevirtv1.VirtualMachine) error {
	for index, credential := range vm.Spec.Template.Spec.AccessCredentials {
		if sshPublicKey := credential.SSHPublicKey; sshPublicKey != nil && sshPublicKey.Source.Secret != nil {
			toCreateSecretName := getTemplateVersionSSHPublicKeySecretName(templateVersion.Name, index)
			if err := h.copySecret(sshPublicKey.Source.Secret.SecretName, toCreateSecretName, templateVersion); err != nil {
				return err
			}
		}
		if userPassword := credential.UserPassword; userPassword != nil && userPassword.Source.Secret != nil {
			toCreateSecretName := getTemplateVersionUserPasswordSecretName(templateVersion.Name, index)
			if err := h.copySecret(userPassword.Source.Secret.SecretName, toCreateSecretName, templateVersion); err != nil {
				return err
			}
		}
	}
	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.CloudInitNoCloud == nil {
			continue
		}
		if volume.CloudInitNoCloud.UserDataSecretRef != nil {
			toCreateSecretName := getTemplateVersionUserDataSecretName(templateVersion.Name, volume.Name)
			if err := h.copySecret(volume.CloudInitNoCloud.UserDataSecretRef.Name, toCreateSecretName, templateVersion); err != nil {
				return err
			}
		}
		if volume.CloudInitNoCloud.NetworkDataSecretRef != nil {
			toCreateSecretName := getTemplateVersionNetworkDataSecretName(templateVersion.Name, volume.Name)
			if err := h.copySecret(volume.CloudInitNoCloud.NetworkDataSecretRef.Name, toCreateSecretName, templateVersion); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *vmActionHandler) copySecret(sourceName, targetName string, templateVersion *harvesterv1.VirtualMachineTemplateVersion) error {
	secret, err := h.secretCache.Get(templateVersion.Namespace, sourceName)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"namespace":       templateVersion.Namespace,
			"templateVersion": templateVersion.Name,
			"secret":          sourceName,
		}).Error("Failed to get secret")
		return err
	}
	toCreate := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetName,
			Namespace: secret.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: templateVersion.APIVersion,
					Kind:       templateVersion.Kind,
					Name:       templateVersion.Name,
					UID:        templateVersion.UID,
				},
			},
		},
		Data: secret.Data,
	}
	_, err = h.secretClient.Create(toCreate)
	return err

}

func (h *vmActionHandler) createVMImages(templateVersion *harvesterv1.VirtualMachineTemplateVersion, vm *kubevirtv1.VirtualMachine, pvcStorageClassMap map[string]string) (map[string]harvesterv1.VirtualMachineImage, error) {
	var err error
	vmImageMap := map[string]harvesterv1.VirtualMachineImage{}
	for index, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			continue
		}
		targetSCName := pvcStorageClassMap[volume.PersistentVolumeClaim.ClaimName]
		// 3 cases here:
		// Root volume with backingImage, check SC name starts with "longhorn-templateversion-", use backingImage backend
		// volume with non longhorn provisioner, use CDI backend
		// volume with longhorn provisioner, use backingImage backend
		// we use CDI as default, so we need to check other two cases
		vmImageBackend := harvesterv1.VMIBackendCDI
		if strings.HasPrefix(targetSCName, "longhorn-templateversion-") {
			vmImageBackend = harvesterv1.VMIBackendBackingImage
		} else {
			// checking SC
			targetSC, err := h.storageClassCache.Get(targetSCName)
			if err != nil {
				return nil, fmt.Errorf("failed to get storage class %s, error: %v", targetSCName, err)
			}
			if targetSC.Provisioner == util.CSIProvisionerLonghorn {
				if dataEngineVers, find := targetSC.Parameters["dataEngine"]; find {
					if dataEngineVers == string(longhorn.DataEngineTypeV1) {
						vmImageBackend = harvesterv1.VMIBackendBackingImage
					}
				} else {
					vmImageBackend = harvesterv1.VMIBackendBackingImage
				}
			}
		}
		if vmImageBackend == "" {
			return nil, fmt.Errorf("failed to configure vm image backend for volume %s", volume.Name)
		}
		vmImageName := getTemplateVersionVMImageName(templateVersion.Name, index)
		vmImage := &harvesterv1.VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmImageName,
				Namespace: vm.Namespace,
				Annotations: map[string]string{
					util.AnnotationStorageClassName: targetSCName,
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: templateVersion.APIVersion,
						Kind:       templateVersion.Kind,
						Name:       templateVersion.Name,
						UID:        templateVersion.UID,
					},
				},
			},
			Spec: harvesterv1.VirtualMachineImageSpec{
				Backend:                vmImageBackend,
				DisplayName:            vmImageName,
				SourceType:             harvesterv1.VirtualMachineImageSourceTypeExportVolume,
				PVCName:                volume.PersistentVolumeClaim.ClaimName,
				PVCNamespace:           vm.Namespace,
				TargetStorageClassName: targetSCName,
			},
		}

		vmImage, err = h.vmImages.Create(vmImage)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"namespace":       templateVersion.Namespace,
				"templateVersion": templateVersion.Name,
				"pvc":             volume.PersistentVolumeClaim.ClaimName,
			}).Error("Failed to create VM image")
			return nil, err
		}
		vmImageMap[vmImage.Name] = *vmImage
	}
	return vmImageMap, nil
}

func (h *vmActionHandler) createVMTemplate(namespace string, input CreateTemplateInput) (vmt *harvesterv1.VirtualMachineTemplate, err error) {
	vmt, err = h.vmTemplateClient.Create(
		&harvesterv1.VirtualMachineTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      input.Name,
				Namespace: namespace,
			},
			Spec: harvesterv1.VirtualMachineTemplateSpec{
				Description: input.Description,
			},
		})
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"namespace": namespace,
			"name":      input.Name,
		}).Errorf("failed to create VM Template: %v", err)
		return nil, err
	}
	return vmt, nil
}

func (h *vmActionHandler) createVMTemplateVersion(namespace string, vm *kubevirtv1.VirtualMachine, vmt *harvesterv1.VirtualMachineTemplate, keyPairIDs []string) (vmtv *harvesterv1.VirtualMachineTemplateVersion, err error) {
	name := fmt.Sprintf("%s-%s", vmt.Name, rand.String(6))
	vmtID := fmt.Sprintf("%s/%s", vmt.Namespace, vmt.Name)
	vmSourceSpec := h.sanitizeVirtualMachineForTemplateVersion(name, vm)
	vmtv, err = h.vmTemplateVersionClient.Create(
		&harvesterv1.VirtualMachineTemplateVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: harvesterv1.VirtualMachineTemplateVersionSpec{
				TemplateID:  vmtID,
				Description: fmt.Sprintf("Template derived from virtual machine [%s]", vmtID),
				VM:          vmSourceSpec,
				KeyPairIDs:  keyPairIDs,
			},
		})
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"namespace": namespace,
			"name":      name,
		}).Errorf("failed to reate VM Template Version: %v", err)
		return nil, err
	}
	return vmtv, nil
}

// cleanup the VirtualMachineTemplate in case we hold a non-nil reference
func (h *vmActionHandler) cleanupVMTemplate(vmt *harvesterv1.VirtualMachineTemplate) (err error) {
	if vmt != nil {
		if err = h.vmTemplateClient.Delete(vmt.Namespace, vmt.Name, &metav1.DeleteOptions{}); err != nil {
			logrus.WithFields(logrus.Fields{
				"namespace": vmt.Namespace,
				"name":      vmt.Name,
			}).Errorf("failed to clean up template version: %v", err)
			return err
		}
	}
	return nil
}

// cleanup the VirtualMachineTemplateVersion in case we hold a non-nil reference
func (h *vmActionHandler) cleanupVMTemplateVersion(vmtv *harvesterv1.VirtualMachineTemplateVersion) (err error) {
	if vmtv != nil {
		if err = h.vmTemplateVersionClient.Delete(vmtv.Namespace, vmtv.Name, &metav1.DeleteOptions{}); err != nil {
			logrus.WithFields(logrus.Fields{
				"namespace": vmtv.Namespace,
				"name":      vmtv.Name,
			}).Errorf("failed to clean up template version: %v", err)
			return err
		}
	}
	return nil
}
