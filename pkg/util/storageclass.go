package util

import (
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	storagev1 "k8s.io/api/storage/v1"
)

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
