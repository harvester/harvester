package util

import (
	"errors"
	"fmt"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	backendstorage "kubevirt.io/kubevirt/pkg/storage/backend-storage"
)

// ErrVMStateStorageClassMissing indicates a missing VM-state StorageClass.
var ErrVMStateStorageClassMissing = fmt.Errorf(
	"storage class %q is missing; required for persistent vTPM or EFI. "+
		"Add a Longhorn disk or create this storage class",
	StorageClassVmstatePersistence,
)

// CheckVMStateStorageClass returns ErrVMStateStorageClassMissing when the vmstate-persistence SC
// is needed but absent, a wrapped error on lookup failure, or nil otherwise.
func CheckVMStateStorageClass(obj interface{}, scCache ctlstoragev1.StorageClassCache) error {
	if !backendstorage.IsBackendStorageNeeded(obj) {
		return nil
	}
	_, err := scCache.Get(StorageClassVmstatePersistence)
	if err == nil {
		return nil
	}
	if apierrors.IsNotFound(err) {
		return ErrVMStateStorageClassMissing
	}
	return fmt.Errorf("failed to get storage class %s: %w", StorageClassVmstatePersistence, err)
}

// IsVMStateStorageClassMissing reports whether err is ErrVMStateStorageClassMissing.
func IsVMStateStorageClassMissing(err error) bool {
	return errors.Is(err, ErrVMStateStorageClassMissing)
}

const (
	ParamDataEngine = "dataEngine"
)

func GetLonghornDataEngineType(sc *storagev1.StorageClass) longhorn.DataEngineType {
	v, ok := sc.Parameters[ParamDataEngine]
	if ok {
		return longhorn.DataEngineType(v)
	}
	return longhorn.DataEngineTypeAll
}
