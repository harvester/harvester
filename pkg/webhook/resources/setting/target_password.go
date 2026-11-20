package setting

import (
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func checkReadyAnnotation(request *types.Request, old, next *harvesterv1.Setting) error {
	if old.Annotations[backuputil.ReadyAnnotation] == next.Annotations[backuputil.ReadyAnnotation] {
		return nil
	}
	if request != nil && request.IsFromController() {
		return nil
	}
	return werror.NewInvalidError("repository readiness is managed by the controller", "metadata.annotations")
}

// checkThirdPartyInput validates user submissions. Authenticated controller
// masking is handled separately by validateUpdateBackupTarget.
func (v *settingValidator) checkThirdPartyInput(target *settings.BackupTarget) error {
	if target.ThirdPartyBackupEnabled {
		return v.checkRepoPassword(target)
	}
	if target.RepoPassword != "" {
		return werror.NewInvalidError(
			"thirdPartyRepositoryPassword requires third-party backups to be enabled", settings.KeywordValue)
	}
	return nil
}

// checkRepoPassword handles user input for an enabled S3 target only.
func (v *settingValidator) checkRepoPassword(target *settings.BackupTarget) error {
	if target.Type != settings.S3BackupType {
		return werror.NewInvalidError("third-party backups require an S3 backup target", settings.KeywordValue)
	}
	if err := backuputil.CheckPasswordInput(target.RepoPassword); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}
	secret, err := v.secretCache.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("Can't read backup target Secret: %v", err))
	}
	if err := backuputil.CheckPasswordChange(secret.Data, backuputil.RepositoryID(target), target.RepoPassword); err != nil {
		return werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}
	return nil
}

// isCredentialMasking matches a controller update that clears saved credential inputs.
func isCredentialMasking(request *types.Request, old *harvesterv1.Setting, target *settings.BackupTarget) bool {
	if request == nil || !request.IsFromController() || target == nil {
		return false
	}
	previous, err := decodeBackupTarget(old)
	if err != nil || previous == nil {
		return false
	}
	previous.AccessKeyID, previous.SecretAccessKey, previous.RepoPassword = "", "", ""
	return reflect.DeepEqual(previous, target)
}
