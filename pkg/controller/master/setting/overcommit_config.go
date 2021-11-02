package setting

import (
	"encoding/json"
	"fmt"
	"strconv"

	longhorn "github.com/longhorn/longhorn-manager/types"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

func (h *Handler) syncOvercommitConfig(setting *harvesterv1.Setting) error {
	overcommit := &settings.Overcommit{}
	if err := json.Unmarshal([]byte(setting.Value), overcommit); err != nil {
		return fmt.Errorf("Invalid JSON `%s`: %s", setting.Value, err.Error())
	}

	// Longhorn storage overcommit
	storage, err := h.longhornSettingCache.Get(util.LonghornSystemNamespaceName, string(longhorn.SettingNameStorageOverProvisioningPercentage))
	if err != nil {
		return err
	}
	storageCpy := storage.DeepCopy()
	percentage := strconv.Itoa(overcommit.Storage)
	if storageCpy.Value != percentage {
		storageCpy.Value = percentage
		if _, err := h.longhornSettings.Update(storageCpy); err != nil {
			return err
		}
	}

	return nil
}
