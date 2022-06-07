package engineapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	VolumeHeadName = "volume-head"
)

// SnapshotCreate calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) SnapshotCreate(engine *longhorn.Engine, name string, labels map[string]string) (string, error) {
	args := []string{"snapshot", "create"}
	for k, v := range labels {
		args = append(args, "--label", k+"="+v)
	}
	args = append(args, name)

	output, err := e.ExecuteEngineBinary(args...)
	if err != nil {
		return "", errors.Wrapf(err, "error creating snapshot '%s'", name)
	}
	return strings.TrimSpace(output), nil
}

// SnapshotList calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) SnapshotList(*longhorn.Engine) (map[string]*longhorn.SnapshotInfo, error) {
	output, err := e.ExecuteEngineBinary("snapshot", "info")
	if err != nil {
		return nil, errors.Wrapf(err, "error listing snapshot")
	}
	data := map[string]*longhorn.SnapshotInfo{}
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return nil, errors.Wrapf(err, "error parsing snapshot list")
	}
	return data, nil
}

// SnapshotGet calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) SnapshotGet(engine *longhorn.Engine, name string) (*longhorn.SnapshotInfo, error) {
	data, err := e.SnapshotList(nil)
	if err != nil {
		return nil, err
	}
	return data[name], nil
}

// SnapshotDelete calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) SnapshotDelete(engine *longhorn.Engine, name string) error {
	if name == VolumeHeadName {
		return fmt.Errorf("invalid operation: cannot remove %v", VolumeHeadName)
	}
	if _, err := e.ExecuteEngineBinary("snapshot", "rm", name); err != nil {
		return errors.Wrapf(err, "error deleting snapshot '%s'", name)
	}
	return nil
}

// SnapshotRevert calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) SnapshotRevert(engine *longhorn.Engine, name string) error {
	if name == VolumeHeadName {
		return fmt.Errorf("invalid operation: cannot revert to %v", VolumeHeadName)
	}
	if _, err := e.ExecuteEngineBinary("snapshot", "revert", name); err != nil {
		return errors.Wrapf(err, "error reverting to snapshot '%s'", name)
	}
	return nil
}

// SnapshotPurge calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) SnapshotPurge(*longhorn.Engine) error {
	if _, err := e.ExecuteEngineBinaryWithoutTimeout([]string{}, "snapshot", "purge", "--skip-if-in-progress"); err != nil {
		return errors.Wrapf(err, "error starting snapshot purge")
	}
	logrus.Debugf("Volume %v snapshot purge started", e.Name())
	return nil
}

// SnapshotPurgeStatus calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) SnapshotPurgeStatus(*longhorn.Engine) (map[string]*longhorn.PurgeStatus, error) {
	output, err := e.ExecuteEngineBinary("snapshot", "purge-status")
	if err != nil {
		return nil, errors.Wrapf(err, "error getting snapshot purge status")
	}

	data := map[string]*longhorn.PurgeStatus{}
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return nil, errors.Wrapf(err, "error parsing snapshot purge status")
	}

	return data, nil
}

// SnapshotClone calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) SnapshotClone(engine *longhorn.Engine, snapshotName, fromControllerAddress string) error {
	args := []string{"snapshot", "clone", "--snapshot-name", snapshotName, "--from-controller-address", fromControllerAddress}
	if _, err := e.ExecuteEngineBinaryWithoutTimeout([]string{}, args...); err != nil {
		return errors.Wrapf(err, "error starting snapshot clone")
	}
	logrus.Debugf("Cloned snapshot %v from volume %v to volume %v", snapshotName, fromControllerAddress, e.cURL)
	return nil
}

// SnapshotCloneStatus calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) SnapshotCloneStatus(*longhorn.Engine) (map[string]*longhorn.SnapshotCloneStatus, error) {
	args := []string{"snapshot", "clone-status"}
	output, err := e.ExecuteEngineBinary(args...)
	if err != nil {
		return nil, err
	}
	snapshotCloneStatusMap := make(map[string]*longhorn.SnapshotCloneStatus)
	if err := json.Unmarshal([]byte(output), &snapshotCloneStatusMap); err != nil {
		return nil, err
	}
	return snapshotCloneStatusMap, nil
}
