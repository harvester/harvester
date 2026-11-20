package util

import (
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	longhorntypes "github.com/longhorn/longhorn-manager/types"
	storagev1 "k8s.io/api/storage/v1"
)

const (
	ParamDataEngine = "dataEngine"
)

// IsLonghornV1StorageClass returns true if sc is a Longhorn v1 StorageClass.
// Longhorn treats a missing dataEngine parameter as v1.
func IsLonghornV1StorageClass(sc *storagev1.StorageClass) bool {
	engine := sc.Parameters[ParamDataEngine]
	return sc.Provisioner == longhorntypes.LonghornDriverName &&
		(engine == "" || engine == string(longhorn.DataEngineTypeV1))
}

func GetLonghornDataEngineType(sc *storagev1.StorageClass) longhorn.DataEngineType {
	v, ok := sc.Parameters[ParamDataEngine]
	if ok {
		return longhorn.DataEngineType(v)
	}
	return longhorn.DataEngineTypeAll
}
