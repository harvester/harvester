package resourcequota

import (
	"encoding/json"
	"fmt"
	"strings"

	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	corev1 "k8s.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"
)

const (
	// even kubevirt can't 100% precisely calculate the exact memory a vmi POD will consume
	// we add an additional 128Mi when compensating RQ, to ensure the vmi migration target pod can be created
	additionalCompensationMemory = 128 << 20

	stringTrue  = "true"
	stringFalse = "false"
)

func HasMigratingVM(rq *corev1.ResourceQuota) bool {
	if rq == nil || rq.Annotations == nil {
		return false
	}

	for k := range rq.Annotations {
		if strings.HasPrefix(k, util.AnnotationMigratingUIDPrefix) {
			return true
		}
		if strings.HasPrefix(k, util.AnnotationMigratingNamePrefix) {
			return true
		}
	}
	return false
}

func AddMigratingVM(rq *corev1.ResourceQuota, vmName, vmUID string, rl corev1.ResourceList) error {
	if rq == nil {
		return fmt.Errorf("failed to call AddMigratingVM as input rq is nil")
	}
	rlb, err := json.Marshal(rl)
	if err != nil {
		return err
	}

	if rq.Annotations == nil {
		rq.Annotations = make(map[string]string)
	}

	// name key may exceed 63 chars, but it does not affect the delete function
	delete(rq.Annotations, util.GenerateAnnotationKeyMigratingVMName(vmName))     // remove the may existing old key
	rq.Annotations[util.GenerateAnnotationKeyMigratingVMUID(vmUID)] = string(rlb) // add UID based key, value
	return nil
}

// delete the maybe existing VM Migration, return true if it exists
func DeleteMigratingVM(rq *corev1.ResourceQuota, vmName, vmUID string) bool {
	if rq == nil || rq.Annotations == nil {
		return false
	}
	len1 := len(rq.Annotations)
	delete(rq.Annotations, util.GenerateAnnotationKeyMigratingVMName(vmName))
	delete(rq.Annotations, util.GenerateAnnotationKeyMigratingVMUID(vmUID))
	len2 := len(rq.Annotations)
	return len1 != len2
}

func ContainsMigratingVM(rq *corev1.ResourceQuota, vmName, vmUID string) bool {
	if rq == nil || rq.Annotations == nil {
		return false
	}
	// check both possible keys
	if _, ok := rq.Annotations[util.GenerateAnnotationKeyMigratingVMName(vmName)]; ok {
		return true
	}
	if _, ok := rq.Annotations[util.GenerateAnnotationKeyMigratingVMUID(vmUID)]; ok {
		return true
	}
	return false
}

// check both possible keys
func getResourceListFromMigratingVMs(rq *corev1.ResourceQuota) (map[string]corev1.ResourceList, error) {
	vms := make(map[string]corev1.ResourceList)
	if rq == nil || rq.Annotations == nil {
		return vms, nil
	}

	for k := range rq.Annotations {
		if strings.HasPrefix(k, util.AnnotationMigratingUIDPrefix) {

			var rl corev1.ResourceList
			if err := json.Unmarshal([]byte(rq.Annotations[k]), &rl); err != nil {
				return nil, fmt.Errorf("failed to unmarshal vm %v quantity %v %w", k, rq.Annotations[k], err)
			}
			vms[strings.TrimPrefix(k, util.AnnotationMigratingUIDPrefix)] = rl
		} else if strings.HasPrefix(k, util.AnnotationMigratingNamePrefix) {

			var rl corev1.ResourceList
			if err := json.Unmarshal([]byte(rq.Annotations[k]), &rl); err != nil {
				return nil, fmt.Errorf("failed to unmarshal vm %v quantity %v %w", k, rq.Annotations[k], err)
			}
			vms[strings.TrimPrefix(k, util.AnnotationMigratingNamePrefix)] = rl
		}
	}
	return vms, nil
}

func HasMigratingCompensation(rq *corev1.ResourceQuota) bool {
	if rq == nil || rq.Annotations == nil {
		return false
	}
	return rq.Annotations[util.AnnotationMigratingCompensation] != ""
}

func AddMigratingCompensation(rq *corev1.ResourceQuota, rl corev1.ResourceList) error {
	if rq == nil {
		return fmt.Errorf("failed to call AddMigratingCompensation as input rq is nil")
	}
	rlb, err := json.Marshal(rl)
	if err != nil {
		return err
	}

	if rq.Annotations == nil {
		rq.Annotations = make(map[string]string)
	}

	rq.Annotations[util.AnnotationMigratingCompensation] = string(rlb)
	return nil
}

// delete the maybe existing Migration compensation, return true if it exists
func DeleteMigratingCompensation(rq *corev1.ResourceQuota) bool {
	if rq == nil || rq.Annotations == nil {
		return false
	}
	initialLen := len(rq.Annotations)
	delete(rq.Annotations, util.AnnotationMigratingCompensation)
	return initialLen != len(rq.Annotations)
}

func getResourceListOfMigratingCompensation(rq *corev1.ResourceQuota) (corev1.ResourceList, error) {
	if rq == nil || rq.Annotations == nil {
		return nil, nil
	}
	compensation := rq.Annotations[util.AnnotationMigratingCompensation]
	if compensation == "" {
		return nil, nil
	}
	var rl corev1.ResourceList
	if err := json.Unmarshal([]byte(compensation), &rl); err != nil {
		return nil, fmt.Errorf("failed to unmarshal compensation quantity %v %w", compensation, err)
	}

	return rl, nil
}

func SetAnnotationMigratingScalingResyncNeeded(rq *corev1.ResourceQuota) {
	if rq == nil {
		return
	}
	// set true
	if rq.Annotations == nil {
		rq.Annotations = make(map[string]string)
	}
	rq.Annotations[util.AnnotationMigratingScalingResyncNeeded] = stringTrue
}

func ClearAnnotationMigratingScalingResyncNeeded(rq *corev1.ResourceQuota) {
	if rq == nil || rq.Annotations == nil {
		return
	}

	// clear the flag if it exists and is "true"
	val, ok := rq.Annotations[util.AnnotationMigratingScalingResyncNeeded]
	if !ok || val != stringTrue {
		return
	}

	rq.Annotations[util.AnnotationMigratingScalingResyncNeeded] = stringFalse
}

func IsMigratingScalingResyncNeeded(rq *corev1.ResourceQuota) bool {
	if rq == nil || rq.Annotations == nil {
		return false
	}
	return rq.Annotations[util.AnnotationMigratingScalingResyncNeeded] == stringTrue
}

func IsResourceQuotaAutoScalingDisabled(rq *corev1.ResourceQuota) bool {
	if rq == nil || rq.Annotations == nil {
		return false
	}
	return rq.Annotations[util.AnnotationSkipResourceQuotaAutoScaling] == stringTrue
}

// check if a resourcequota is managed by the namespace annotation util.CattleAnnotationResourceQuota
func IsResourceQuotaManagedByNamespaceAnnotation(rq *corev1.ResourceQuota, rqStr string) (bool, error) {
	if rqStr == "" {
		return false, nil
	}
	if IsEmptyResourceQuota(rq) {
		return false, nil
	}
	var rqBase v3.NamespaceResourceQuota
	if err := json.Unmarshal([]byte(rqStr), &rqBase); err != nil {
		return false, err
	}
	rCPULimit, rMemoryLimit, err := GetCPUMemoryLimitsFromRancherNamespaceResourceQuota(&rqBase)
	if err != nil {
		return false, err
	}
	return !rCPULimit.IsZero() || !rMemoryLimit.IsZero(), nil
}

// check if a resourcequota is managed by the namespace annotation util.CattleAnnotationResourceQuota and has MemoryLimits
func IsResourceQuotaManagedByNamespaceAnnotationWithMemoryLimits(rq *corev1.ResourceQuota, rqStr string) (bool, error) {
	if rqStr == "" {
		return false, nil
	}
	if isMemoryLimitEmpty(rq) {
		return false, nil
	}
	var rqBase v3.NamespaceResourceQuota
	if err := json.Unmarshal([]byte(rqStr), &rqBase); err != nil {
		return false, err
	}
	rMemoryLimit, err := GetMemoryLimitsFromRancherNamespaceResourceQuota(&rqBase)
	if err != nil {
		return false, err
	}
	return !rMemoryLimit.IsZero(), nil
}
