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

func (e *Engine) SnapshotCreate(name string, labels map[string]string) (string, error) {
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

func (e *Engine) SnapshotList() (map[string]*longhorn.Snapshot, error) {
	output, err := e.ExecuteEngineBinary("snapshot", "info")
	if err != nil {
		return nil, errors.Wrapf(err, "error listing snapshot")
	}
	data := map[string]*longhorn.Snapshot{}
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return nil, errors.Wrapf(err, "error parsing snapshot list")
	}
	return data, nil
}

func (e *Engine) SnapshotGet(name string) (*longhorn.Snapshot, error) {
	data, err := e.SnapshotList()
	if err != nil {
		return nil, err
	}
	return data[name], nil
}

func (e *Engine) SnapshotDelete(name string) error {
	if name == VolumeHeadName {
		return fmt.Errorf("invalid operation: cannot remove %v", VolumeHeadName)
	}
	if _, err := e.ExecuteEngineBinary("snapshot", "rm", name); err != nil {
		return errors.Wrapf(err, "error deleting snapshot '%s'", name)
	}
	return nil
}

func (e *Engine) SnapshotRevert(name string) error {
	if name == VolumeHeadName {
		return fmt.Errorf("invalid operation: cannot revert to %v", VolumeHeadName)
	}
	if _, err := e.ExecuteEngineBinary("snapshot", "revert", name); err != nil {
		return errors.Wrapf(err, "error reverting to snapshot '%s'", name)
	}
	return nil
}

func (e *Engine) SnapshotPurge() error {
	if _, err := e.ExecuteEngineBinaryWithoutTimeout([]string{}, "snapshot", "purge", "--skip-if-in-progress"); err != nil {
		return errors.Wrapf(err, "error starting snapshot purge")
	}
	logrus.Debugf("Volume %v snapshot purge started", e.Name())
	return nil
}

func (e *Engine) SnapshotPurgeStatus() (map[string]*longhorn.PurgeStatus, error) {
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

func (e *Engine) SnapshotClone(snapshotName, fromControllerAddress string) error {
	args := []string{"snapshot", "clone", "--snapshot-name", snapshotName, "--from-controller-address", fromControllerAddress}
	if _, err := e.ExecuteEngineBinaryWithoutTimeout([]string{}, args...); err != nil {
		return errors.Wrapf(err, "error starting snapshot clone")
	}
	logrus.Debugf("Cloned snapshot %v from volume %v to volume %v", snapshotName, fromControllerAddress, e.cURL)
	return nil
}

func (e *Engine) SnapshotCloneStatus() (map[string]*longhorn.SnapshotCloneStatus, error) {
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
