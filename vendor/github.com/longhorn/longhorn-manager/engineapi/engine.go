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

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"
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
	name  string
	image string
	ip    string
	port  int
	cURL  string
}

func (c *EngineCollection) NewEngineClient(request *EngineClientRequest) (*EngineBinary, error) {
	if request.EngineImage == "" {
		return nil, fmt.Errorf("invalid empty engine image from request")
	}

	if request.IP != "" && request.Port == 0 {
		return nil, fmt.Errorf("invalid empty port from request with valid IP")
	}

	return &EngineBinary{
		name:  request.VolumeName,
		image: request.EngineImage,
		ip:    request.IP,
		port:  request.Port,
		cURL:  imutil.GetURL(request.IP, request.Port),
	}, nil
}

func (e *EngineBinary) Name() string {
	return e.name
}

func (e *EngineBinary) LonghornEngineBinary() string {
	return filepath.Join(types.GetEngineBinaryDirectoryOnHostForImage(e.image), "longhorn")
}

func (e *EngineBinary) ExecuteEngineBinary(args ...string) (string, error) {
	args = append([]string{"--url", e.cURL}, args...)
	return util.Execute([]string{}, e.LonghornEngineBinary(), args...)
}

func (e *EngineBinary) ExecuteEngineBinaryWithTimeout(timeout time.Duration, args ...string) (string, error) {
	args = append([]string{"--url", e.cURL}, args...)
	return util.ExecuteWithTimeout(timeout, []string{}, e.LonghornEngineBinary(), args...)
}

func (e *EngineBinary) ExecuteEngineBinaryWithoutTimeout(envs []string, args ...string) (string, error) {
	args = append([]string{"--url", e.cURL}, args...)
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
		return nil, errors.Wrapf(err, "failed to list replicas from controller '%s'", e.name)
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
func (e *EngineBinary) ReplicaAdd(engine *longhorn.Engine, url string, isRestoreVolume bool) error {
	if err := ValidateReplicaURL(url); err != nil {
		return err
	}
	cmd := []string{"add", url}
	if isRestoreVolume {
		cmd = append(cmd, "--restore")
	}
	if _, err := e.ExecuteEngineBinaryWithoutTimeout([]string{}, cmd...); err != nil {
		return errors.Wrapf(err, "failed to add replica address='%s' to controller '%s'", url, e.name)
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
		return errors.Wrapf(err, "failed to rm replica address='%s' from controller '%s'", url, e.name)
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
	frontendName, err := GetEngineProcessFrontend(engine.Spec.Frontend)
	if err != nil {
		return err
	}
	if frontendName == "" {
		return fmt.Errorf("cannot start empty frontend")
	}

	if _, err := e.ExecuteEngineBinary("frontend", "start", frontendName); err != nil {
		return errors.Wrapf(err, "error starting frontend %v", frontendName)
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

// ReplicaRebuildVerify calls engine binary
// TODO: Deprecated, replaced by gRPC proxy
func (e *EngineBinary) ReplicaRebuildVerify(engine *longhorn.Engine, url string) error {
	if err := ValidateReplicaURL(url); err != nil {
		return err
	}
	cmd := []string{"verify-rebuild-replica", url}
	if _, err := e.ExecuteEngineBinaryWithoutTimeout([]string{}, cmd...); err != nil {
		return errors.Wrapf(err, "failed to verify rebuilding for the replica from address %s", url)
	}
	return nil
}

// Close engine proxy client connection.
// Do not panic this method because this is could be called by the fallback client.
func (e *EngineBinary) Close() {
}
