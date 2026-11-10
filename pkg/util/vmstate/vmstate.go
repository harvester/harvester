package vmstate

import (
	"errors"
	"fmt"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kubevirtv1 "kubevirt.io/api/core/v1"
	backendstorage "kubevirt.io/kubevirt/pkg/storage/backend-storage"
	"kubevirt.io/kubevirt/pkg/tpm"

	"github.com/harvester/harvester/pkg/util"
)

// ErrVMStateStorageClassMissing indicates a missing VM-state StorageClass.
var ErrVMStateStorageClassMissing = fmt.Errorf(
	"storage class %q is missing; required for persistent vTPM or EFI. "+
		"Add a Longhorn disk or create this storage class",
	util.StorageClassVmstatePersistence,
)

func vmiSpecOf(obj interface{}) (ns, name string, spec *kubevirtv1.VirtualMachineInstanceSpec, err error) {
	switch o := obj.(type) {
	case *kubevirtv1.VirtualMachine:
		if o.Spec.Template == nil {
			return o.Namespace, o.Name, nil, nil
		}
		return o.Namespace, o.Name, &o.Spec.Template.Spec, nil
	case *kubevirtv1.VirtualMachineInstance:
		return o.Namespace, o.Name, &o.Spec, nil
	default:
		return "", "", nil, fmt.Errorf("unsupported type %T: expected *VirtualMachine or *VirtualMachineInstance", obj)
	}
}

func needsVMState(spec *kubevirtv1.VirtualMachineInstanceSpec) bool {
	return spec != nil && (tpm.HasPersistentDevice(spec) || backendstorage.HasPersistentEFI(spec))
}

// BackendPVCExists returns true when the backend-storage PVC for the given name already exists
// in the namespace. KubeVirt reuses an existing PVC without needing the StorageClass.
func BackendPVCExists(pvcCache ctlcorev1.PersistentVolumeClaimCache, namespace, name string) bool {
	pvc, err := pvcCache.Get(namespace, backendstorage.PVCPrefix+"-"+name)
	return err == nil && pvc.DeletionTimestamp == nil
}

// CheckVMStateStorageClass returns ErrVMStateStorageClassMissing when the vmstate-persistence SC
// is needed but absent, a wrapped error on lookup failure, or nil otherwise.
// If pvcCache is non-nil and the backend PVC already exists, the check is skipped.
func CheckVMStateStorageClass(obj interface{}, scCache ctlstoragev1.StorageClassCache, pvcCache ctlcorev1.PersistentVolumeClaimCache) error {
	ns, name, spec, err := vmiSpecOf(obj)
	if err != nil {
		return err
	}
	if !needsVMState(spec) {
		return nil
	}
	if pvcCache != nil && BackendPVCExists(pvcCache, ns, name) {
		return nil
	}
	if _, err = scCache.Get(util.StorageClassVmstatePersistence); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrVMStateStorageClassMissing
		}
		return fmt.Errorf("looking up storage class %q: %w", util.StorageClassVmstatePersistence, err)
	}
	return nil
}

// IsVMStateStorageClassMissing reports whether err is ErrVMStateStorageClassMissing.
func IsVMStateStorageClassMissing(err error) bool {
	return errors.Is(err, ErrVMStateStorageClassMissing)
}
