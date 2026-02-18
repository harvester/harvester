package resourcequota

import (
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"
)

const (
	// even kubevirt can't 100% precisely calculate the exact memory a vmi POD will consume
	// we add additional 128Mi when compensate RQ, to ensure the vmi migration target pod can be created
	additionalCompensationMemeory = 128 << 20
)

func HasMigratingVM(rq *corev1.ResourceQuota) bool {
	if rq.Annotations == nil {
		return false
	}

	for k := range rq.Annotations {
		if strings.HasPrefix(k, util.AnnotationMigratingUIDPrefix) {
			return true
		}
	}
	return false
}

func HasMigratingCompensation(rq *corev1.ResourceQuota) bool {
	return rq.Annotations[util.AnnotationMigratingCompensation] != ""
}

func AddMigratingCompensation(rq *corev1.ResourceQuota, rl corev1.ResourceList) error {
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

// delete the may existing Miration compensation, return true if it exists
func DeleteMigratingCompensation(rq *corev1.ResourceQuota) bool {
	if rq.Annotations == nil {
		return false
	}
	len1 := len(rq.Annotations)
	delete(rq.Annotations, util.AnnotationMigratingCompensation)
	len2 := len(rq.Annotations)
	return len1 != len2
}

func AddMigratingVM(rq *corev1.ResourceQuota, vmName, vmUID string, rl corev1.ResourceList) error {
	rlb, err := json.Marshal(rl)
	if err != nil {
		return err
	}

	if rq.Annotations == nil {
		rq.Annotations = make(map[string]string)
	}

	rq.Annotations[util.GenerateAnnotationKeyMigratingVMUID(vmUID)] = string(rlb) // add UID based key, value
	return nil
}

// delete the may existing VM Miration, return true if it exists
func DeleteMigratingVM(rq *corev1.ResourceQuota, vmName, vmUID string) bool {
	if rq.Annotations == nil {
		return false
	}
	len1 := len(rq.Annotations)
	delete(rq.Annotations, util.GenerateAnnotationKeyMigratingVMUID(vmUID))
	len2 := len(rq.Annotations)
	return len1 != len2
}

func ContainsMigratingVM(rq *corev1.ResourceQuota, vmUID string) bool {
	if rq.Annotations == nil {
		return false
	}
	if _, ok := rq.Annotations[util.GenerateAnnotationKeyMigratingVMUID(vmUID)]; ok {
		return true
	}
	return false
}

func getResourceListOfMigratingVMsFromRQ(rq *corev1.ResourceQuota) (map[string]corev1.ResourceList, error) {
	vms := make(map[string]corev1.ResourceList)
	if rq.Annotations == nil {
		return vms, nil
	}

	for k := range rq.Annotations {
		if !strings.HasPrefix(k, util.AnnotationMigratingUIDPrefix) {
			continue
		}

		var rl corev1.ResourceList
		if err := json.Unmarshal([]byte(rq.Annotations[k]), &rl); err != nil {
			return nil, fmt.Errorf("failed to unmarshal vm %v quantity %v %w", k, rq.Annotations[k], err)
		}
		vms[strings.TrimPrefix(k, util.AnnotationMigratingUIDPrefix)] = rl

	}
	return vms, nil
}

func getResourceListOfMigratingCompensationFromRQ(rq *corev1.ResourceQuota) (corev1.ResourceList, error) {
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
