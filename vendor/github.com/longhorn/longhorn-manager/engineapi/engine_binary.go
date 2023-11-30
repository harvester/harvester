package engineapi

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/longhorn/longhorn-manager/datastore"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func GetEngineBinaryClient(ds *datastore.DataStore, volumeName, nodeID string) (client *EngineBinary, err error) {
	var e *longhorn.Engine

	defer func() {
		err = errors.Wrapf(err, "cannot get client for volume %v", volumeName)
	}()
	es, err := ds.ListVolumeEngines(volumeName)
	if err != nil {
		return nil, err
	}
	if len(es) == 0 {
		return nil, fmt.Errorf("cannot find engine")
	}
	if len(es) != 1 {
		return nil, fmt.Errorf("more than one engine exists")
	}
	for _, e = range es {
		break
	}
	if e.Spec.BackendStoreDriver == longhorn.BackendStoreDriverTypeV2 {
		return nil, nil
	}
	if e.Status.CurrentState != longhorn.InstanceStateRunning {
		return nil, fmt.Errorf("engine is not running")
	}
	if isReady, err := ds.CheckEngineImageReadiness(e.Status.CurrentImage, nodeID); !isReady {
		if err != nil {
			return nil, fmt.Errorf("cannot get engine client with image %v: %v", e.Status.CurrentImage, err)
		}
		return nil, fmt.Errorf("cannot get engine client with image %v because it isn't deployed on this node", e.Status.CurrentImage)
	}

	engineCollection := &EngineCollection{}
	return engineCollection.NewEngineClient(&EngineClientRequest{
		VolumeName:   e.Spec.VolumeName,
		EngineImage:  e.Status.CurrentImage,
		IP:           e.Status.IP,
		Port:         e.Status.Port,
		InstanceName: e.Name,
	})
}
