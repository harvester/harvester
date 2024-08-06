package util

import (
	"fmt"

	longhornv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	longhorntypes "github.com/longhorn/longhorn-manager/types"
	"k8s.io/apimachinery/pkg/labels"

	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/util"
)

func GetLHEngine(engineCache ctllonghornv1.EngineCache, volumeName string) (*longhornv1beta2.Engine, error) {
	engines, err := engineCache.List(
		util.LonghornSystemNamespaceName,
		labels.SelectorFromSet(labels.Set{
			longhorntypes.LonghornLabelVolume: volumeName,
		}))
	if err != nil {
		return nil, fmt.Errorf("failed to list Longhorn engines with volume %s, err: %w", volumeName, err)
	}
	if len(engines) != 1 {
		return nil, fmt.Errorf("expected 1 Longhorn engine volume %s, found %d", volumeName, len(engines))
	}
	return engines[0], nil
}
