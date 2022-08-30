package engineapi

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/longhorn/backupstore"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"
)

const (
	backupStateInProgress = "in_progress"
	backupStateComplete   = "complete"
	backupStateError      = "error"
)

type BackupTargetBinaryClient interface {
	BackupGet(destURL string, credential map[string]string) (*Backup, error)
	BackupVolumeGet(destURL string, credential map[string]string) (volume *BackupVolume, err error)
	BackupNameList(destURL, volumeName string, credential map[string]string) (names []string, err error)
	BackupVolumeNameList(destURL string, credential map[string]string) (names []string, err error)
	BackupDelete(destURL string, credential map[string]string) (err error)
	BackupVolumeDelete(destURL, volumeName string, credential map[string]string) (err error)
	BackupConfigMetaGet(destURL string, credential map[string]string) (*ConfigMetadata, error)
}

type BackupTargetConfig struct {
	URL        string
	Credential map[string]string
}

// getBackupCredentialEnv returns the environment variables as KEY=VALUE in string slice
func getBackupCredentialEnv(backupTarget string, credential map[string]string) ([]string, error) {
	envs := []string{}
	backupType, err := util.CheckBackupType(backupTarget)
	if err != nil {
		return envs, err
	}

	if backupType != types.BackupStoreTypeS3 || credential == nil {
		return envs, nil
	}

	var missingKeys []string
	if credential[types.AWSAccessKey] == "" {
		missingKeys = append(missingKeys, types.AWSAccessKey)
	}
	if credential[types.AWSSecretKey] == "" {
		missingKeys = append(missingKeys, types.AWSSecretKey)
	}
	// If AWS IAM Role not present, then the AWS credentials must be exists
	if credential[types.AWSIAMRoleArn] == "" && len(missingKeys) > 0 {
		return nil, fmt.Errorf("could not backup to %s, missing %v in the secret", backupType, missingKeys)
	}
	if len(missingKeys) == 0 {
		envs = append(envs, fmt.Sprintf("%s=%s", types.AWSAccessKey, credential[types.AWSAccessKey]))
		envs = append(envs, fmt.Sprintf("%s=%s", types.AWSSecretKey, credential[types.AWSSecretKey]))
	}
	envs = append(envs, fmt.Sprintf("%s=%s", types.AWSEndPoint, credential[types.AWSEndPoint]))
	envs = append(envs, fmt.Sprintf("%s=%s", types.AWSCert, credential[types.AWSCert]))
	envs = append(envs, fmt.Sprintf("%s=%s", types.HTTPSProxy, credential[types.HTTPSProxy]))
	envs = append(envs, fmt.Sprintf("%s=%s", types.HTTPProxy, credential[types.HTTPProxy]))
	envs = append(envs, fmt.Sprintf("%s=%s", types.NOProxy, credential[types.NOProxy]))
	envs = append(envs, fmt.Sprintf("%s=%s", types.VirtualHostedStyle, credential[types.VirtualHostedStyle]))
	return envs, nil
}

// parseBackupVolumeNamesList parses a list of backup volume names into a sorted string slice
func parseBackupVolumeNamesList(output string) ([]string, error) {
	data := map[string]struct{}{}
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return nil, errors.Wrapf(err, "error parsing backup volume names: \n%s", output)
	}

	volumeNames := []string{}
	for volumeName := range data {
		volumeNames = append(volumeNames, volumeName)
	}
	sort.Strings(volumeNames)
	return volumeNames, nil
}

// parseBackupNamesList parses a list of backup names into a sorted string slice
func parseBackupNamesList(output, volumeName string) ([]string, error) {
	data := map[string]*BackupVolume{}
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return nil, errors.Wrapf(err, "error parsing backup names: \n%s", output)
	}

	volumeData, ok := data[volumeName]
	if !ok {
		return nil, fmt.Errorf("cannot find the volume name %s in the data", volumeName)
	}

	backupNames := []string{}
	if volumeData.Messages[string(backupstore.MessageTypeError)] != "" {
		return backupNames, errors.New(volumeData.Messages[string(backupstore.MessageTypeError)])
	}
	for backupName := range volumeData.Backups {
		backupNames = append(backupNames, backupName)
	}
	sort.Strings(backupNames)
	return backupNames, nil
}

// parseBackupVolumeConfig parses a backup volume config
func parseBackupVolumeConfig(output string) (*BackupVolume, error) {
	backupVolume := new(BackupVolume)
	if err := json.Unmarshal([]byte(output), backupVolume); err != nil {
		return nil, errors.Wrapf(err, "error parsing one backup volume config: \n%s", output)
	}
	return backupVolume, nil
}

// parseBackupConfig parses a backup config
func parseBackupConfig(output string) (*Backup, error) {
	backup := new(Backup)
	if err := json.Unmarshal([]byte(output), backup); err != nil {
		return nil, errors.Wrapf(err, "error parsing one backup config: \n%s", output)
	}
	return backup, nil
}

// parseConfigMetadata parses the config metadata
func parseConfigMetadata(output string) (*ConfigMetadata, error) {
	metadata := new(ConfigMetadata)
	if err := json.Unmarshal([]byte(output), metadata); err != nil {
		return nil, errors.Wrapf(err, "error parsing config metadata: \n%s", output)
	}
	return metadata, nil
}

// SnapshotBackup calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) SnapshotBackup(engine *longhorn.Engine,
	snapName, backupName, backupTarget,
	backingImageName, backingImageChecksum string,
	labels, credential map[string]string) (string, string, error) {
	if snapName == VolumeHeadName {
		return "", "", fmt.Errorf("invalid operation: cannot backup %v", VolumeHeadName)
	}
	// TODO: update when replacing this function
	snap, err := e.SnapshotGet(nil, snapName)
	if err != nil {
		return "", "", errors.Wrapf(err, "error getting snapshot '%s', volume '%s'", snapName, e.name)
	}
	if snap == nil {
		return "", "", errors.Errorf("could not find snapshot '%s' to backup, volume '%s'", snapName, e.name)
	}
	version, err := e.VersionGet(nil, true)
	if err != nil {
		return "", "", err
	}
	args := []string{"backup", "create", "--dest", backupTarget}
	if backingImageName != "" {
		args = append(args, "--backing-image-name", backingImageName)
		// TODO: Remove this if there is no backward compatibility
		if version.ClientVersion.CLIAPIVersion <= CLIVersionFour {
			args = append(args, "--backing-image-url", "deprecated-field")
		} else if backingImageChecksum != "" {
			args = append(args, "--backing-image-checksum", backingImageChecksum)
		}
	}
	if backupName != "" && version.ClientVersion.CLIAPIVersion > CLIVersionFour {
		args = append(args, "--backup-name", backupName)
	}
	for k, v := range labels {
		args = append(args, "--label", k+"="+v)
	}
	args = append(args, snapName)

	// get environment variables if backup for s3
	envs, err := getBackupCredentialEnv(backupTarget, credential)
	if err != nil {
		return "", "", err
	}
	output, err := e.ExecuteEngineBinaryWithoutTimeout(envs, args...)
	if err != nil {
		return "", "", err
	}
	backupCreateInfo := BackupCreateInfo{}
	if err := json.Unmarshal([]byte(output), &backupCreateInfo); err != nil {
		return "", "", err
	}

	logrus.Debugf("Backup %v created for volume %v snapshot %v", backupCreateInfo.BackupID, e.Name(), snapName)
	return backupCreateInfo.BackupID, backupCreateInfo.ReplicaAddress, nil
}

// SnapshotBackupStatus calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) SnapshotBackupStatus(engine *longhorn.Engine, backupName, replicaAddress string) (*longhorn.EngineBackupStatus, error) {
	args := []string{"backup", "status", backupName}
	if replicaAddress != "" {
		args = append(args, "--replica", replicaAddress)
	}

	output, err := e.ExecuteEngineBinary(args...)
	if err != nil {
		return nil, err
	}

	engineBackupStatus := &longhorn.EngineBackupStatus{}
	if err := json.Unmarshal([]byte(output), engineBackupStatus); err != nil {
		return nil, err
	}
	return engineBackupStatus, nil
}

// ConvertEngineBackupState converts longhorn engine backup state to Backup CR state
func ConvertEngineBackupState(state string) longhorn.BackupState {
	// https://github.com/longhorn/longhorn-engine/blob/9da3616/pkg/replica/backup.go#L20-L22
	switch state {
	case backupStateInProgress:
		return longhorn.BackupStateInProgress
	case backupStateComplete:
		return longhorn.BackupStateCompleted
	case backupStateError:
		return longhorn.BackupStateError
	default:
		return longhorn.BackupStateUnknown
	}
}

// BackupRestore calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) BackupRestore(engine *longhorn.Engine, backupTarget, backupName, backupVolumeName, lastRestored string, credential map[string]string) error {
	backup := backupstore.EncodeBackupURL(backupName, backupVolumeName, backupTarget)

	// get environment variables if backup for s3
	envs, err := getBackupCredentialEnv(backupTarget, credential)
	if err != nil {
		return err
	}

	args := []string{"backup", "restore", backup}
	// TODO: Remove this compatible code and update the function signature
	//  when the manager doesn't support the engine v1.0.0 or older version.
	if lastRestored != "" {
		args = append(args, "--incrementally", "--last-restored", lastRestored)
	}

	if output, err := e.ExecuteEngineBinaryWithoutTimeout(envs, args...); err != nil {
		var taskErr TaskError
		if jsonErr := json.Unmarshal([]byte(output), &taskErr); jsonErr != nil {
			logrus.Warnf("Cannot unmarshal the restore error, maybe it's not caused by the replica restore failure: %v", jsonErr)
			return err
		}
		return taskErr
	}

	logrus.Debugf("Backup %v restored for volume %v", backup, e.Name())
	return nil
}

// BackupRestoreStatus calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) BackupRestoreStatus(*longhorn.Engine) (map[string]*longhorn.RestoreStatus, error) {
	args := []string{"backup", "restore-status"}
	output, err := e.ExecuteEngineBinary(args...)
	if err != nil {
		return nil, err
	}
	replicaStatusMap := make(map[string]*longhorn.RestoreStatus)
	if err := json.Unmarshal([]byte(output), &replicaStatusMap); err != nil {
		return nil, err
	}
	return replicaStatusMap, nil
}
