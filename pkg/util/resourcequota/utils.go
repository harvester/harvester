package resourcequota

import (
	"encoding/json"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"
)

func HasMigratingVM(rq *corev1.ResourceQuota) bool {
	if rq.Annotations == nil {
		return false
	}

	for k := range rq.Annotations {
		if strings.HasPrefix(k, util.AnnotationMigratingPrefix) {
			return true
		}
	}
	return false
}

func UpdateMigratingVM(rq *corev1.ResourceQuota, vmName string, rl corev1.ResourceList) error {
	rlb, err := json.Marshal(rl)
	if err != nil {
		return err
	}

	if rq.Annotations == nil {
		rq.Annotations = make(map[string]string)
	}
	rq.Annotations[util.AnnotationMigratingPrefix+vmName] = string(rlb)
	return nil
}

func RemoveMigratingVM(rq *corev1.ResourceQuota, vmName string) {
	if rq.Annotations == nil {
		return
	}
	delete(rq.Annotations, util.AnnotationMigratingPrefix+vmName)
}

func ContainsMigratingVM(rq *corev1.ResourceQuota, vmName string) bool {
	if rq.Annotations == nil {
		return false
	}
	if _, ok := rq.Annotations[util.AnnotationMigratingPrefix+vmName]; ok {
		return true
	}
	return false
}

func GetResourceListFromMigratingVM(rq *corev1.ResourceQuota, vmName string) (corev1.ResourceList, error) {
	if rq.Annotations != nil {
		if v, ok := rq.Annotations[util.AnnotationMigratingPrefix+vmName]; ok {
			var rl corev1.ResourceList
			if err := json.Unmarshal([]byte(v), &rl); err != nil {
				return nil, err
			}
			return rl, nil
		}
	}
	return nil, nil
}

func GetResourceListFromMigratingVMs(rq *corev1.ResourceQuota) (map[string]corev1.ResourceList, error) {
	vms := make(map[string]corev1.ResourceList)
	if rq.Annotations == nil {
		return vms, nil
	}

	for k := range rq.Annotations {
		if strings.HasPrefix(k, util.AnnotationMigratingPrefix) {

			var rl corev1.ResourceList
			if err := json.Unmarshal([]byte(rq.Annotations[k]), &rl); err != nil {
				return nil, err
			}
			vms[strings.TrimPrefix(k, util.AnnotationMigratingPrefix)] = rl
		}
	}
	return vms, nil
}
