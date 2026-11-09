package vmstate

import (
	"errors"
	"fmt"

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

// NeedsVMStateStorageClass reports whether obj requires the vmstate-persistence StorageClass.
// It checks only persistent vTPM/EFI set in the spec; CBT (set by the controller via status)
// is intentionally excluded to avoid false positives in admission webhooks.
func NeedsVMStateStorageClass(obj interface{}) bool {
	switch obj := obj.(type) {
	case *kubevirtv1.VirtualMachine:
		if obj.Spec.Template == nil {
			return false
		}
		return tpm.HasPersistentDevice(&obj.Spec.Template.Spec) || backendstorage.HasPersistentEFI(&obj.Spec.Template.Spec)
	case *kubevirtv1.VirtualMachineInstance:
		return tpm.HasPersistentDevice(&obj.Spec) || backendstorage.HasPersistentEFI(&obj.Spec)
	default:
		return false
	}
}

// CheckVMStateStorageClass returns ErrVMStateStorageClassMissing when the vmstate-persistence SC
// is needed but absent, a wrapped error on lookup failure, or nil otherwise.
func CheckVMStateStorageClass(obj interface{}, scCache ctlstoragev1.StorageClassCache) error {
	if !NeedsVMStateStorageClass(obj) {
		return nil
	}
	_, err := scCache.Get(util.StorageClassVmstatePersistence)
	if err == nil {
		return nil
	}
	if apierrors.IsNotFound(err) {
		return ErrVMStateStorageClassMissing
	}
	return fmt.Errorf("failed to get storage class %s: %w", util.StorageClassVmstatePersistence, err)
}

// IsVMStateStorageClassMissing reports whether err is ErrVMStateStorageClassMissing.
func IsVMStateStorageClassMissing(err error) bool {
	return errors.Is(err, ErrVMStateStorageClassMissing)
}
