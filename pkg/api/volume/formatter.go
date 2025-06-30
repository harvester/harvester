package volume

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/v3/pkg/data/convert"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"
)

const (
	actionExport       = "export"
	actionCancelExpand = "cancelExpand"
	actionClone        = "clone"
	actionSnapshot     = "snapshot"
)

type volFormatter struct {
	scCache ctlstoragev1.StorageClassCache
}

func (f *volFormatter) Formatter(request *types.APIRequest, resource *types.RawResource) {
	resource.Actions = make(map[string]string, 1)
	if request.AccessControl.CanUpdate(request, resource.APIObject, resource.Schema) != nil {
		return
	}

	pvc := &corev1.PersistentVolumeClaim{}
	if err := convert.ToObj(resource.APIObject.Data(), pvc); err != nil {
		return
	}

	if IsResizing(pvc) {
		resource.AddAction(request, actionCancelExpand)
		return
	}

	if pvc.Status.Phase != corev1.ClaimBound {
		return
	}

	// after we introduce the CDI path, now, the whole volumes could support export
	resource.AddAction(request, actionExport)

	resource.AddAction(request, actionClone)

	provisioner := util.GetProvisionedPVCProvisioner(pvc, f.scCache)
	if find := util.GetCSIProvisionerSnapshotCapability(provisioner); find {
		resource.AddAction(request, actionSnapshot)
	}
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
