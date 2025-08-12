package backuptarget

import (
	"context"
	"fmt"
	"net/http"

	// Although we don't use following drivers directly, we need to import them to register drivers.
	// NFS Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/nfs/nfs.go#L47-L51
	// S3 Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/s3/s3.go#L33-L37
	_ "github.com/longhorn/backupstore/nfs" //nolint
	_ "github.com/longhorn/backupstore/s3"  //nolint
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
)

type HealthyHandler struct {
	context      context.Context
	settingCache v1beta1.SettingCache
	secretCache  ctlcorev1.SecretCache
}

func NewHealthyHandler(scaled *config.Scaled) *HealthyHandler {
	return &HealthyHandler{
		context:      scaled.Ctx,
		settingCache: scaled.HarvesterFactory.Harvesterhci().V1beta1().Setting().Cache(),
		secretCache:  scaled.CoreFactory.Core().V1().Secret().Cache(),
	}
}

func (h *HealthyHandler) ServeHTTP(rw http.ResponseWriter, _ *http.Request) {
	backupTargetSetting, err := h.settingCache.Get(settings.BackupTargetSettingName)
	if err != nil {
		util.ResponseError(rw, http.StatusInternalServerError, fmt.Errorf("can't get %s setting, error: %w", settings.BackupTargetSettingName, err))
		return
	}
	if backupTargetSetting.Value == "" {
		util.ResponseError(rw, http.StatusBadRequest, fmt.Errorf("%s setting is not set", settings.BackupTargetSettingName))
		return
	}

	target, err := settings.DecodeBackupTarget(backupTargetSetting.Value)
	if err != nil {
		util.ResponseError(rw, http.StatusInternalServerError, fmt.Errorf("can't decode %s setting %s, error: %w", settings.BackupTargetSettingName, backupTargetSetting.Value, err))
		return
	}
	if target.IsDefaultBackupTarget() {
		util.ResponseError(rw, http.StatusBadRequest, fmt.Errorf("can't check the backup target healthy, %s setting is not set", settings.BackupTargetSettingName))
		return
	}

	_, err = backuputil.GetBackupStoreDriver(h.secretCache, target)
	if err != nil {
		util.ResponseError(rw, http.StatusServiceUnavailable, fmt.Errorf("can't connect to backup target %+v, error: %w", target, err))
		return
	}
	util.ResponseOK(rw)
}
