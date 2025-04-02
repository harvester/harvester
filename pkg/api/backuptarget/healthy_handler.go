package backuptarget

import (
	"context"
	"fmt"

	// Although we don't use following drivers directly, we need to import them to register drivers.
	// NFS Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/nfs/nfs.go#L47-L51
	// S3 Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/s3/s3.go#L33-L37
	_ "github.com/longhorn/backupstore/nfs" //nolint
	_ "github.com/longhorn/backupstore/s3"  //nolint
	"github.com/rancher/apiserver/pkg/apierror"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
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

func (h *HealthyHandler) Do(_ *harvesterServer.Ctx) (harvesterServer.ResponseBody, error) {
	backupTargetSetting, err := h.settingCache.Get(settings.BackupTargetSettingName)
	if err != nil {
		return nil, apierror.NewAPIError(validation.ServerError, fmt.Errorf("can't get %s setting, error: %w", settings.BackupTargetSettingName, err).Error())
	}
	if backupTargetSetting.Value == "" {
		return nil, apierror.NewAPIError(validation.InvalidBodyContent, fmt.Errorf("%s setting is not set", settings.BackupTargetSettingName).Error())
	}

	target, err := settings.DecodeBackupTarget(backupTargetSetting.Value)
	if err != nil {
		return nil, apierror.NewAPIError(validation.ServerError, fmt.Errorf("can't decode %s setting %s, error: %w", settings.BackupTargetSettingName, backupTargetSetting.Value, err).Error())
	}
	if target.IsDefaultBackupTarget() {
		return nil, apierror.NewAPIError(validation.InvalidBodyContent, fmt.Errorf("can't check the backup target healthy, %s setting is not set", settings.BackupTargetSettingName).Error())
	}

	_, err = util.GetBackupStoreDriver(h.secretCache, target)
	if err != nil {
		return nil, apierror.NewAPIError(validation.InvalidBodyContent, fmt.Errorf("can't connect to backup target %+v, error: %w", target, err).Error())
	}

	return nil, nil
}
