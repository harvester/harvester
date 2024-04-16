package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/mount-utils"

	spdkhelpertypes "github.com/longhorn/go-spdk-helper/pkg/types"
)

const (
	DefaulCmdTimeout = time.Minute // one minute by default

	GRPCHealthProbe = "/usr/local/bin/grpc_health_probe"
)

func Execute(binary string, args ...string) (string, error) {
	return ExecuteWithTimeout(DefaulCmdTimeout, binary, args...)
}

func ExecuteWithTimeout(timeout time.Duration, binary string, args ...string) (string, error) {
	var err error
	cmd := exec.Command(binary, args...)
	done := make(chan struct{})

	var output, stderr bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr

	go func() {
		err = cmd.Run()
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		if cmd.Process != nil {
			if err := cmd.Process.Kill(); err != nil {
				logrus.WithError(err).Warnf("Problem killing process pid=%v", cmd.Process.Pid)
			}

		}
		return "", errors.Wrapf(err, "timeout executing: %v %v, output %s, stderr %s",
			binary, args, output.String(), stderr.String())
	}

	if err != nil {
		return "", errors.Wrapf(err, "failed to execute: %v %v, output %s, stderr %s",
			binary, args, output.String(), stderr.String())
	}
	return output.String(), nil
}

func PrintJSON(obj interface{}) error {
	output, err := json.MarshalIndent(obj, "", "\t")
	if err != nil {
		return err
	}

	fmt.Println(string(output))
	return nil
}

func GetURL(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func RemoveFile(file string) error {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		// file doesn't exist
		return nil
	}

	if _, err := Execute("rm", file); err != nil {
		return errors.Wrapf(err, "failed to remove file %v", file)
	}

	return nil
}

func GRPCServiceReadinessProbe(address string) bool {
	if _, err := Execute(GRPCHealthProbe, "-addr", address); err != nil {
		return false
	}
	return true
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func UUID() string {
	return uuid.New().String()
}

func ParsePortRange(portRange string) (int32, int32, error) {
	parts := strings.Split(portRange, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid format for SPDK port range %s", portRange)
	}

	portStart, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, err
	}

	portEnd, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, err
	}

	return int32(portStart), int32(portEnd), nil
}

// IsSPDKTgtReady checks if SPDK target is ready
func IsSPDKTgtReady(timeout time.Duration) bool {
	for i := 0; i < int(timeout.Seconds()); i++ {
		conn, err := net.DialTimeout(spdkhelpertypes.DefaultJSONServerNetwork, spdkhelpertypes.DefaultUnixDomainSocketPath, 1*time.Second)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(time.Second)
	}
	return false
}

func GetVolumeMountPointMap() (map[string]mount.MountPoint, error) {
	volumeMountPointMap := make(map[string]mount.MountPoint)

	mounter := mount.New("")
	mountPoints, err := mounter.List()
	if err != nil {
		return nil, err
	}

	regex := regexp.MustCompile(`.*/globalmount$`)

	for _, mp := range mountPoints {
		if regex.MatchString(mp.Path) {
			volumeNameSHAStr := GetVolumeNameSHAStrFromPath(mp.Path)
			volumeMountPointMap[volumeNameSHAStr] = mp
		}
	}
	return volumeMountPointMap, nil
}

func GetVolumeNameSHAStrFromPath(path string) string {
	// mount path for volume: "/host/var/lib/kubelet/plugins/kubernetes.io/csi/driver.longhorn.io/${VolumeNameSHAStr}/globalmount"
	pathSlices := strings.Split(path, "/")
	volumeNameSHAStr := pathSlices[len(pathSlices)-2]
	return volumeNameSHAStr
}

func ProcessNameToVolumeName(processName string) string {
	// process name: "pvc-e130e369-274d-472d-98d1-f6074d2725e8-e-0"
	nameSlices := strings.Split(processName, "-")
	volumeName := strings.Join(nameSlices[:len(nameSlices)-2], "-")
	return volumeName
}
