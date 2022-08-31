package volumesnapshot

import (
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/data/convert"

	"github.com/harvester/harvester/pkg/util"
)

const (
	actionRestore = "restore"
)

func Formatter(request *types.APIRequest, resource *types.RawResource) {
	resource.Actions = make(map[string]string, 1)
	if request.AccessControl.CanUpdate(request, resource.APIObject, resource.Schema) != nil {
		return
	}

	volumeSnapshot := &snapshotv1.VolumeSnapshot{}
	if err := convert.ToObj(resource.APIObject.Data(), volumeSnapshot); err != nil {
		return
	}

	if volumeSnapshot.Status != nil && *volumeSnapshot.Status.ReadyToUse && volumeSnapshot.Annotations[util.AnnotationStorageClassName] != "" {
		resource.AddAction(request, actionRestore)
	}
}
