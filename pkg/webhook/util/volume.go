package util

import (
	"fmt"
	"slices"

	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	corev1 "k8s.io/api/core/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	kvirtfeatures "kubevirt.io/kubevirt/pkg/virt-config/featuregate"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
	indexeresutil "github.com/harvester/harvester/pkg/util/indexeres"
	werror "github.com/harvester/harvester/pkg/webhook/error"
)

func checkOnlineExpand(
	pvc *corev1.PersistentVolumeClaim,
	scCache ctlstoragev1.StorageClassCache,
	settingCache ctlharvesterv1.SettingCache,
) (bool, error) {
	provisioner, err := getPVCProvisioner(pvc, scCache)
	if err != nil {
		return false, fmt.Errorf("error determining provisioner for PVC %s/%s: %w", pvc.Namespace, pvc.Name, err)
	}

	return util.GetCSIOnlineExpandValidation(provisioner, settingCache)
}

func getPVCProvisioner(pvc *corev1.PersistentVolumeClaim, scCache ctlstoragev1.StorageClassCache) (string, error) {
	provisioner := util.GetProvisionedPVCProvisioner(pvc, scCache)
	if provisioner == "" {
		return "", fmt.Errorf("failed to determine provisioner for PVC %s/%s", pvc.Namespace, pvc.Name)
	}
	return provisioner, nil
}

func isOnlineExpandNeeded(pvc *corev1.PersistentVolumeClaim, vmCache ctlkv1.VirtualMachineCache) (bool, error) {
	indexKey := ref.Construct(pvc.Namespace, pvc.Name)
	vms, err := vmCache.GetByIndex(indexeresutil.VMByPVCIndex, indexKey)
	if err != nil {
		return false, werror.NewInternalError(fmt.Sprintf("failed to get VMs by index: %s, PVC: %s/%s, err: %s", indexeresutil.VMByPVCIndex, pvc.Namespace, pvc.Name, err))
	}

	for _, vm := range vms {
		if vm.Status.PrintableStatus != kubevirtv1.VirtualMachineStatusStopped {
			return true, nil
		}
	}

	return false, nil
}

func isHotpluggedFilesystemPVC(pvc *corev1.PersistentVolumeClaim, vmCache ctlkv1.VirtualMachineCache) (bool, error) {
	// Check if the PVC is in Filesystem mode
	if pvc.Spec.VolumeMode == nil || *pvc.Spec.VolumeMode != corev1.PersistentVolumeFilesystem {
		return false, nil
	}

	// Check if the PVC is hotplugged to any VM
	vms, err := vmCache.GetByIndex(indexeresutil.VMByHotplugPVCIndex, ref.Construct(pvc.Namespace, pvc.Name))
	if err != nil {
		return false, werror.NewInternalError(err.Error())
	}

	return len(vms) > 0, nil
}

func isKubevirtExpandEnabled(kubevirt *kubevirtv1.KubeVirt) bool {
	featureGates := kubevirt.Spec.Configuration.DeveloperConfiguration.FeatureGates
	return slices.Contains(featureGates, kvirtfeatures.ExpandDisksGate)
}

func CheckExpand(pvc *corev1.PersistentVolumeClaim,
	vmCache ctlkv1.VirtualMachineCache,
	kubevirtCache ctlkv1.KubeVirtCache,
	scCache ctlstoragev1.StorageClassCache,
	settingCache ctlharvesterv1.SettingCache) error {

	// Check if online expand is needed first
	onlineExpand, err := isOnlineExpandNeeded(pvc, vmCache)
	if err != nil {
		return err
	}
	if !onlineExpand {
		return nil
	}

	kubevirt, err := kubevirtCache.Get(util.HarvesterSystemNamespaceName, util.KubeVirtObjectName)
	if err != nil {
		return err
	}
	if !isKubevirtExpandEnabled(kubevirt) {
		return werror.NewInvalidError(util.PVCExpandErrorPrefix+": kubevirt ExpandDisks not included in featureGate", "")
	}

	hotpluggedFSPVC, err := isHotpluggedFilesystemPVC(pvc, vmCache)
	if err != nil {
		return err
	}
	if hotpluggedFSPVC {
		return werror.NewInvalidError(
			fmt.Sprintf(
				util.PVCExpandErrorPrefix+": Expansion of hotplugged PVC '%s/%s' in filesystem mode is not supported",
				pvc.Namespace,
				pvc.Name,
			),
			"",
		)
	}

	expandable, err := checkOnlineExpand(pvc, scCache, settingCache)
	if err != nil {
		return err
	}
	if !expandable {
		return werror.NewInvalidError(
			fmt.Sprintf(
				util.PVCExpandErrorPrefix+": pvc %s/%s is not online expandable with its provider",
				pvc.Namespace,
				pvc.Name,
			),
			"",
		)
	}

	return nil
}
