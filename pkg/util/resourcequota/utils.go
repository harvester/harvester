package resourcequota

import (
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"
)

func HasMigratingVM(rq *corev1.ResourceQuota) bool {
	if rq.Annotations == nil {
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

// delete the may existing VM Miration, return true if it exists
func DeleteMigratingVM(rq *corev1.ResourceQuota, vmName, vmUID string) bool {
	if rq.Annotations == nil {
		return false
	}
	len1 := len(rq.Annotations)
	delete(rq.Annotations, util.GenerateAnnotationKeyMigratingVMName(vmName))
	delete(rq.Annotations, util.GenerateAnnotationKeyMigratingVMUID(vmUID))
	len2 := len(rq.Annotations)
	return len1 != len2
}

func ContainsMigratingVM(rq *corev1.ResourceQuota, vmName, vmUID string) bool {
	if rq.Annotations == nil {
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
	if rq.Annotations == nil {
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
