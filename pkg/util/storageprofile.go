package util

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

// ToCloneStrategy converts a string to a CDICloneStrategy type.
// It returns an error if the string does not match any known clone strategy.
func ToCloneStrategy(v string) (*cdiv1.CDICloneStrategy, error) {
	var cloneStrategy cdiv1.CDICloneStrategy
	switch v {
	case string(cdiv1.CloneStrategyHostAssisted):
		cloneStrategy = cdiv1.CloneStrategyHostAssisted
	case string(cdiv1.CloneStrategySnapshot):
		cloneStrategy = cdiv1.CloneStrategySnapshot
	case string(cdiv1.CloneStrategyCsiClone):
		cloneStrategy = cdiv1.CloneStrategyCsiClone
	default:
		return nil, fmt.Errorf("invalid clone strategy %s", v)
	}
	return &cloneStrategy, nil
}

// ToVolumeMode converts a string to a PersistentVolumeMode type.
// It returns an error if the string does not match any known volume mode.
func ToVolumeMode(v string) (*corev1.PersistentVolumeMode, error) {
	var volumeMode corev1.PersistentVolumeMode
	switch v {
	case string(corev1.PersistentVolumeBlock):
		volumeMode = corev1.PersistentVolumeBlock
	case string(corev1.PersistentVolumeFilesystem):
		volumeMode = corev1.PersistentVolumeFilesystem
	default:
		return nil, fmt.Errorf("invalid volume mode %s", v)
	}
	return &volumeMode, nil
}

// ToAccessMode converts a string to a PersistentVolumeAccessMode type.
// It returns an error if the string does not match any known access mode.
func ToAccessMode(v string) (*corev1.PersistentVolumeAccessMode, error) {
	var accessMode corev1.PersistentVolumeAccessMode
	switch v {
	case string(corev1.ReadWriteOnce):
		accessMode = corev1.ReadWriteOnce
	case string(corev1.ReadOnlyMany):
		accessMode = corev1.ReadOnlyMany
	case string(corev1.ReadWriteMany):
		accessMode = corev1.ReadWriteMany
	case string(corev1.ReadWriteOncePod):
		accessMode = corev1.ReadWriteOncePod
	default:
		return nil, fmt.Errorf("invalid access mode %s", v)
	}
	return &accessMode, nil
}

// ParseVolumeModeAccessModes parses a JSON string into a slice of ClaimPropertySet.
// The JSON string should be in the format:
//
//	{
//	  "volumeMode1": ["accessMode1", "accessMode2"],
//	  "volumeMode2": ["accessMode1", "accessMode2"]
//	}
//
// It returns an error if the JSON is invalid or if any of the volume modes or access modes are invalid.
func ParseVolumeModeAccessModes(jsonStr string) ([]cdiv1.ClaimPropertySet, error) {
	tmp := map[string][]string{}
	if err := json.Unmarshal([]byte(jsonStr), &tmp); err != nil {
		return nil, err
	}

	result := make([]cdiv1.ClaimPropertySet, 0, len(tmp))
	for volumeModeStr, accessModesStr := range tmp {
		vm, err := ToVolumeMode(volumeModeStr)
		if err != nil {
			return nil, err
		}
		var ams []corev1.PersistentVolumeAccessMode
		for _, amStr := range accessModesStr {
			am, err := ToAccessMode(amStr)
			if err != nil {
				return nil, err
			}
			ams = append(ams, *am)
		}
		result = append(result, cdiv1.ClaimPropertySet{
			AccessModes: ams,
			VolumeMode:  vm,
		})
	}
	return result, nil
}
