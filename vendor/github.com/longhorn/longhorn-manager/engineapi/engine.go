package engineapi

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	imutil "github.com/longhorn/longhorn-instance-manager/pkg/util"

	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

// Should be the same values as in https://github.com/longhorn/longhorn-engine/blob/master/pkg/types/types.go
const (
	ProcessStateInProgress = "in_progress"
	ProcessStateComplete   = "complete"
	ProcessStateError      = "error"

	ErrNotImplement = "not implemented"
)

type EngineCollection struct{}

type EngineBinary struct {
	volumeName   string
	image        string
	ip           string
	port         int
	cURL         string
	instanceName string
}

func (c *EngineCollection) NewEngineClient(request *EngineClientRequest) (*EngineBinary, error) {
	if request.EngineImage == "" {
		return nil, fmt.Errorf("invalid empty engine image from request")
	}

	if request.IP != "" && request.Port == 0 {
		return nil, fmt.Errorf("invalid empty port from request with valid IP")
	}

	return &EngineBinary{
		volumeName:   request.VolumeName,
		image:        request.EngineImage,
		ip:           request.IP,
		port:         request.Port,
		cURL:         imutil.GetURL(request.IP, request.Port),
		instanceName: request.InstanceName,
	}, nil
}

func (e *EngineBinary) Name() string {
	return e.volumeName
}

func (e *EngineBinary) LonghornEngineBinary() string {
	return filepath.Join(types.GetEngineBinaryDirectoryOnHostForImage(e.image), "longhorn")
}

func (e *EngineBinary) ExecuteEngineBinary(args ...string) (string, error) {
	args, err := e.addFlags(args)
	if err != nil {
		return "", err
	}
	return util.Execute([]string{}, e.LonghornEngineBinary(), args...)
}

func (e *EngineBinary) ExecuteEngineBinaryWithTimeout(timeout time.Duration, args ...string) (string, error) {
	args, err := e.addFlags(args)
	if err != nil {
		return "", err
	}
	return util.ExecuteWithTimeout(timeout, []string{}, e.LonghornEngineBinary(), args...)
}

func (e *EngineBinary) ExecuteEngineBinaryWithoutTimeout(envs []string, args ...string) (string, error) {
	args, err := e.addFlags(args)
	if err != nil {
		return "", err
	}
	return util.ExecuteWithoutTimeout(envs, e.LonghornEngineBinary(), args...)
}

func parseReplica(s string) (*Replica, error) {
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return nil, errors.Errorf("cannot parse line `%s`", s)
	}
	url := fields[0]
	mode := longhorn.ReplicaMode(fields[1])
	if mode != longhorn.ReplicaModeRW && mode != longhorn.ReplicaModeWO {
		mode = longhorn.ReplicaModeERR
	}
	return &Replica{
		URL:  url,
		Mode: mode,
	}, nil
}

// ReplicaList calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) ReplicaList(*longhorn.Engine) (map[string]*Replica, error) {
	output, err := e.ExecuteEngineBinary("ls")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list replicas from controller '%s'", e.volumeName)
	}
	replicas := make(map[string]*Replica)
	lines := strings.Split(output, "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "ADDRESS") {
			continue
		}
		if l == "" {
			continue
		}
		replica, err := parseReplica(l)
		if err != nil {
			return nil, errors.Wrapf(err, "error parsing replica status from `%s`, output %v", l, output)
		}
		replicas[replica.URL] = replica
	}
	return replicas, nil
}

// ReplicaAdd calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) ReplicaAdd(engine *longhorn.Engine, replicaName, url string, isRestoreVolume, fastSync bool, replicaFileSyncHTTPClientTimeout int64) error {
	if err := ValidateReplicaURL(url); err != nil {
		return err
	}
	cmd := []string{"add", url}
	if isRestoreVolume {
		cmd = append(cmd, "--restore")
	}

	version, err := e.VersionGet(engine, true)
	if err != nil {
		return err
	}

	if version.ClientVersion.CLIAPIVersion >= 6 {
		cmd = append(cmd,
			"--size", strconv.FormatInt(engine.Spec.VolumeSize, 10),
			"--current-size", strconv.FormatInt(engine.Status.CurrentSize, 10))
	}

	if version.ClientVersion.CLIAPIVersion >= 7 {
		cmd = append(cmd, "--file-sync-http-client-timeout", strconv.FormatInt(replicaFileSyncHTTPClientTimeout, 10))

		if fastSync {
			cmd = append(cmd, "--fast-sync")
		}
	}

	if version.ClientVersion.CLIAPIVersion >= 9 {
		cmd = append(cmd, "--replica-instance-name", replicaName)
	}

	if _, err := e.ExecuteEngineBinaryWithoutTimeout([]string{}, cmd...); err != nil {
		return errors.Wrapf(err, "failed to add replica address='%s' to controller '%s'", url, e.volumeName)
	}
	return nil
}

// ReplicaRemove calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) ReplicaRemove(engine *longhorn.Engine, url string) error {
	if err := ValidateReplicaURL(url); err != nil {
		return err
	}
	if _, err := e.ExecuteEngineBinary("rm", url); err != nil {
		return errors.Wrapf(err, "failed to rm replica address='%s' from controller '%s'", url, e.volumeName)
	}
	return nil
}

// VolumeGet calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) VolumeGet(*longhorn.Engine) (*Volume, error) {
	output, err := e.ExecuteEngineBinary("info")
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get volume info")
	}

	info := &Volume{}
	if err := json.Unmarshal([]byte(output), info); err != nil {
		return nil, errors.Wrapf(err, "cannot decode volume info: %v", output)
	}
	return info, nil
}

// VersionGet calls engine binary to get client version and request gRPC proxy
// for server version.
func (e *EngineBinary) VersionGet(engine *longhorn.Engine, clientOnly bool) (*EngineVersion, error) {
	cmdline := []string{"version"}
	if clientOnly {
		cmdline = append(cmdline, "--client-only")
	} else {
		cmdline = append([]string{"--url", e.cURL}, cmdline...)
	}
	output, err := util.Execute([]string{}, e.LonghornEngineBinary(), cmdline...)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get volume version")
	}

	version := &EngineVersion{}
	if err := json.Unmarshal([]byte(output), version); err != nil {
		return nil, errors.Wrapf(err, "cannot decode volume version: %v", output)
	}
	return version, nil
}

// VolumeExpand calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) VolumeExpand(engine *longhorn.Engine) error {
	size := engine.Spec.VolumeSize
	if _, err := e.ExecuteEngineBinary("expand", "--size", strconv.FormatInt(size, 10)); err != nil {
		return errors.Wrapf(err, "cannot get expand volume engine to size %v", size)
	}

	return nil
}

// ReplicaRebuildStatus calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) ReplicaRebuildStatus(*longhorn.Engine) (map[string]*longhorn.RebuildStatus, error) {
	output, err := e.ExecuteEngineBinary("replica-rebuild-status")
	if err != nil {
		return nil, errors.Wrapf(err, "error getting replica rebuild status")
	}

	data := map[string]*longhorn.RebuildStatus{}
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return nil, errors.Wrapf(err, "error parsing replica rebuild status")
	}

	return data, nil
}

// VolumeFrontendStart calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) VolumeFrontendStart(engine *longhorn.Engine) error {
	frontendName, err := GetEngineInstanceFrontend(engine.Spec.BackendStoreDriver, engine.Spec.Frontend)
	if err != nil {
		return err
	}
	if frontendName == "" {
		return fmt.Errorf("cannot start empty frontend")
	}

	if _, err := e.ExecuteEngineBinary("frontend", "start", frontendName); err != nil {
		return errors.Wrapf(err, "failed to start frontend %v", frontendName)
	}

	return nil
}

// VolumeFrontendShutdown calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) VolumeFrontendShutdown(*longhorn.Engine) error {
	if _, err := e.ExecuteEngineBinary("frontend", "shutdown"); err != nil {
		return errors.Wrapf(err, "error shutting down the frontend")
	}

	return nil
}

// VolumeUnmapMarkSnapChainRemovedSet calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) VolumeUnmapMarkSnapChainRemovedSet(engine *longhorn.Engine) error {
	cmdline := []string{"unmap-mark-snap-chain-removed"}
	if engine.Spec.UnmapMarkSnapChainRemovedEnabled {
		cmdline = append(cmdline, "--enable")
	} else {
		cmdline = append(cmdline, "--disable")
	}
	if _, err := e.ExecuteEngineBinary(cmdline...); err != nil {
		return errors.Wrapf(err, "error setting volume flag UnmapMarkSnapChainRemoved to %v", engine.Spec.UnmapMarkSnapChainRemovedEnabled)
	}

	return nil
}

// ReplicaRebuildVerify calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) ReplicaRebuildVerify(engine *longhorn.Engine, replicaName, url string) error {
	if err := ValidateReplicaURL(url); err != nil {
		return err
	}

	version, err := e.VersionGet(engine, true)
	if err != nil {
		return err
	}

	cmd := []string{"verify-rebuild-replica", url}
	if version.ClientVersion.CLIAPIVersion >= 9 {
		cmd = append(cmd, "--replica-instance-name", replicaName)
	}

	if _, err := e.ExecuteEngineBinaryWithoutTimeout([]string{}, cmd...); err != nil {
		return errors.Wrapf(err, "failed to verify rebuilding for the replica from address %s", url)
	}
	return nil
}

// Close engine proxy client connection.
// Do not panic this method because this is could be called by the fallback client.
func (e *EngineBinary) Close() {
}

// ReplicaModeUpdate calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) ReplicaModeUpdate(engine *longhorn.Engine, url, mode string) error {
	_, err := e.ExecuteEngineBinary("update", "--mode", mode, url)
	if err != nil {
		return errors.Wrapf(err, "failed to update replica %v mode to %v", url, mode)
	}
	return nil
}

func (e *EngineBinary) MetricsGet(*longhorn.Engine) (*Metrics, error) {
	return nil, fmt.Errorf(ErrNotImplement)
}

// addFlags always adds required flags to args. In addition, if the engine version is high enough, it adds additional
// engine identity validation flags.
func (e *EngineBinary) addFlags(args []string) ([]string, error) {
	version, err := e.VersionGet(nil, true)
	if err != nil {
		return args, errors.Wrap(err, "failed to get engine CLI version while adding identity flags")
	}

	argsToAdd := []string{"--url", e.cURL}
	if version.ClientVersion.CLIAPIVersion >= 9 {
		argsToAdd = append(argsToAdd, "--volume-name", e.volumeName, "--engine-instance-name", e.instanceName)
	}
	return append(argsToAdd, args...), nil
}
