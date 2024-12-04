package common

import (
	"encoding/json"
	"errors"
	"fmt"

	longhorntypes "github.com/longhorn/longhorn-manager/types"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/rancher/wrangler/v3/pkg/slice"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

type VMIMutator interface {
	PatchImageSCParams(vmi *harvesterv1.VirtualMachineImage) ([]string, error)
}

type vmiMutator struct {
	scCache ctlstoragev1.StorageClassCache
}

func GetVMIMutator(scCache ctlstoragev1.StorageClassCache) VMIMutator {
	return &vmiMutator{
		scCache: scCache,
	}
}

func mergeStorageClassParams(vmi *harvesterv1.VirtualMachineImage, sc *storagev1.StorageClass) map[string]string {
	params := util.GetImageDefaultStorageClassParameters()
	var mergeParams map[string]string
	if sc != nil {
		mergeParams = sc.Parameters
	} else if vmi.Spec.StorageClassParameters != nil {
		mergeParams = vmi.Spec.StorageClassParameters
	}
	var allowPatchParams = []string{
		longhorntypes.OptionNodeSelector, longhorntypes.OptionDiskSelector,
		longhorntypes.OptionNumberOfReplicas, longhorntypes.OptionStaleReplicaTimeout,
		util.LonghornDataLocality,
		util.LonghornOptionEncrypted,
		util.CSIProvisionerSecretNameKey, util.CSIProvisionerSecretNamespaceKey,
		util.CSINodeStageSecretNameKey, util.CSINodeStageSecretNamespaceKey,
		util.CSINodePublishSecretNameKey, util.CSINodePublishSecretNamespaceKey,
	}

	for k, v := range mergeParams {
		if slice.ContainsString(allowPatchParams, k) {
			params[k] = v
		}
	}
	return params
}

func (m *vmiMutator) getSC(scName string) (*storagev1.StorageClass, error) {
	if scName != "" {
		storageClass, err := m.scCache.Get(scName)
		if err != nil {
			return nil, err
		}
		if storageClass.Provisioner != longhorntypes.LonghornDriverName {
			return nil, fmt.Errorf("the provisioner of storageClass must be %s, not %s", longhorntypes.LonghornDriverName, storageClass.Provisioner)
		}
		if storageClass.Parameters[util.LonghornOptionBackingImageName] != "" {
			return nil, errors.New("can not use a backing image storageClass as the base storageClass template")
		}
		return storageClass, nil
	}

	storageClasses, err := m.scCache.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, storageClass := range storageClasses {
		if storageClass.Annotations[util.AnnotationIsDefaultStorageClassName] == "true" &&
			storageClass.Provisioner == longhorntypes.LonghornDriverName {
			return storageClass, nil
		}
	}

	return nil, nil
}

func (m *vmiMutator) PatchImageSCParams(vmi *harvesterv1.VirtualMachineImage) ([]string, error) {
	var patchOps types.PatchOps

	scName := vmi.Annotations[util.AnnotationStorageClassName]
	sc, err := m.getSC(scName)
	if err != nil {
		return patchOps, err
	}

	parameters := mergeStorageClassParams(vmi, sc)
	valueBytes, err := json.Marshal(parameters)
	if err != nil {
		return patchOps, err
	}

	verb := "add"
	if vmi.Spec.StorageClassParameters != nil {
		verb = "replace"
	}

	patchOps = append(patchOps, fmt.Sprintf(`{"op": "%s", "path": "/spec/storageClassParameters", "value": %s}`, verb, string(valueBytes)))
	return patchOps, nil
}
