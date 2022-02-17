package volume

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/data/convert"
	corev1 "k8s.io/api/core/v1"
)

const (
	actionExport       = "export"
	actionCancelExpand = "cancelExpand"
)

func Formatter(request *types.APIRequest, resource *types.RawResource) {
	resource.Actions = make(map[string]string, 1)
	if request.AccessControl.CanUpdate(request, resource.APIObject, resource.Schema) != nil {
		return
	}

	pvc := &corev1.PersistentVolumeClaim{}
	if err := convert.ToObj(resource.APIObject.Data(), pvc); err != nil {
		return
	}

	if canExport(pvc) {
		resource.AddAction(request, actionExport)
	}

	if canCancelExpand(pvc) {
		resource.AddAction(request, actionCancelExpand)
	}
}

func canExport(pvc *corev1.PersistentVolumeClaim) bool {
	return !IsResizing(pvc)
}

func canCancelExpand(pvc *corev1.PersistentVolumeClaim) bool {
	return IsResizing(pvc)
}

func IsResizing(pvc *corev1.PersistentVolumeClaim) bool {
	if pvc == nil {
		return false
	}

	for _, condition := range pvc.Status.Conditions {
		if condition.Type == corev1.PersistentVolumeClaimResizing && condition.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}
