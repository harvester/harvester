package util

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
)

const (
	QemuImgBinary = "qemu-img"
)

func PrintJSON(obj interface{}) error {
	output, err := json.MarshalIndent(obj, "", "\t")
	if err != nil {
		return err
	}

	fmt.Println(string(output))
	return nil
}

func GetFileChecksum(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func CopyFile(srcPath, dstPath string) (int64, error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return 0, err
	}
	defer src.Close()

	if _, err := os.Stat(dstPath); err == nil || !os.IsNotExist(err) {
		if err := os.RemoveAll(dstPath); err != nil {
			return 0, errors.Wrapf(err, "failed to clean up the dst file path before copy")
		}
	}
	dst, err := os.Create(dstPath)
	if err != nil {
		return 0, err
	}
	defer dst.Close()

	return io.Copy(dst, src)
}

func IsGRPCErrorNotFound(err error) bool {
	return IsGRPCErrorMatchingCode(err, codes.NotFound)
}

func IsGRPCErrorMatchingCode(err error, errCode codes.Code) bool {
	gRPCStatus, ok := status.FromError(err)
	return ok && gRPCStatus.Code() == errCode
}

func DetectGRPCServerAvailability(address string, waitIntervalInSecond int, shouldAvailable bool) bool {
	endTime := time.Now().Add(time.Duration(waitIntervalInSecond) * time.Second)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for time.Now().Before(endTime) {
		<-ticker.C

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		conn, err := grpc.DialContext(ctx, address, grpc.WithInsecure())
		cancel()
		if !shouldAvailable {
			if err != nil {
				return true
			}
			state := conn.GetState()
			if state != connectivity.Ready && state != connectivity.Idle && state != connectivity.Connecting {
				return true
			}
		}
		if shouldAvailable && err == nil {
			state := conn.GetState()
			if state == connectivity.Ready || state == connectivity.Idle {
				return true
			}
		}
	}

	return false
}

// DiskConfigFile should be the same as the schema in longhorn-manager/util
const (
	DiskConfigFile = "longhorn-disk.cfg"
)

type DiskConfig struct {
	DiskUUID string `json:"diskUUID"`
}

func GetDiskConfig(diskPath string) (string, error) {
	filePath := filepath.Join(diskPath, DiskConfigFile)
	output, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("cannot find disk config file %v: %v", filePath, err)
	}

	cfg := &DiskConfig{}
	if err := json.Unmarshal([]byte(output), cfg); err != nil {
		return "", fmt.Errorf("failed to unmarshal %v content %v: %v", filePath, output, err)
	}
	return cfg.DiskUUID, nil
}

const (
	SyncingFileConfigFileSuffix = ".cfg"
)

type SyncingFileConfig struct {
	FilePath         string `json:"name"`
	UUID             string `json:"uuid"`
	Size             int64  `json:"size"`
	ExpectedChecksum string `json:"expectedChecksum"`
	CurrentChecksum  string `json:"currentChecksum"`
	ModificationTime string `json:"modificationTime"`
}

func GetSyncingFileConfigFilePath(syncingFilePath string) string {
	return fmt.Sprintf("%s%s", syncingFilePath, SyncingFileConfigFileSuffix)
}

func WriteSyncingFileConfig(configFilePath string, config *SyncingFileConfig) (err error) {
	encoded, err := json.Marshal(config)
	if err != nil {
		return errors.Wrapf(err, "BUG: Cannot marshal %+v", config)
	}

	defer func() {
		if err != nil {
			if delErr := os.Remove(configFilePath); delErr != nil && !os.IsNotExist(delErr) {
				err = errors.Wrapf(err, "cleaning up syncing file config %v failed with error: %v", configFilePath, delErr)
			}
		}
	}()
	// We don't care the previous config file content.
	return os.WriteFile(configFilePath, encoded, 0666)
}

func ReadSyncingFileConfig(configFilePath string) (*SyncingFileConfig, error) {
	output, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot find the syncing file config file %v", configFilePath)
	}

	config := &SyncingFileConfig{}
	if err := json.Unmarshal(output, config); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal %v content %v", configFilePath, output)
	}
	return config, nil
}

func Execute(envs []string, binary string, args ...string) (string, error) {
	return ExecuteWithTimeout(time.Minute, envs, binary, args...)
}

func ExecuteWithTimeout(timeout time.Duration, envs []string, binary string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var err error
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), envs...)
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
	case <-ctx.Done():
		if cmd.Process != nil {
			if err := cmd.Process.Kill(); err != nil {
				logrus.Warnf("problem killing process pid=%v: %s", cmd.Process.Pid, err)
			}
		}
		return "", fmt.Errorf("timeout executing: %v %v, output %s, stderr, %s, error %v",
			binary, args, output.String(), stderr.String(), err)
	}

	if err != nil {
		return "", fmt.Errorf("failed to execute: %v %v, output %s, stderr, %s, error %v",
			binary, args, output.String(), stderr.String(), err)
	}
	return output.String(), nil
}

func DetectFileFormat(filePath string) (string, error) {

	/* Example command outputs
	   $ qemu-img info parrot.raw
	   image: parrot.raw
	   file format: raw
	   virtual size: 32M (33554432 bytes)
	   disk size: 2.2M

	   $ qemu-img info parrot.qcow2
	   image: parrot.qcow2
	   file format: qcow2
	   virtual size: 32M (33554432 bytes)
	   disk size: 2.3M
	   cluster_size: 65536
	   Format specific information:
	       compat: 1.1
	       lazy refcounts: false
	       refcount bits: 16
	       corrupt: false
	*/

	output, err := Execute([]string{}, QemuImgBinary, "info", filePath)
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "file format: ") {
			return strings.TrimPrefix(line, "file format: "), nil
		}
	}

	return "", fmt.Errorf("cannot find the file format in the output %s", output)
}

func ConvertFromRawToQcow2(filePath string) error {
	if format, err := DetectFileFormat(filePath); err != nil {
		return err
	} else if format == "qcow2" {
		return nil
	}

	tmpFilePath := filePath + ".qcow2tmp"
	defer os.RemoveAll(tmpFilePath)

	if _, err := Execute([]string{}, QemuImgBinary, "convert", "-f", "raw", "-O", "qcow2", filePath, tmpFilePath); err != nil {
		return err
	}
	if err := os.RemoveAll(filePath); err != nil {
		return err
	}
	return os.Rename(tmpFilePath, filePath)
}

func FileModificationTime(filePath string) string {
	fi, err := os.Stat(filePath)
	if err != nil {
		return ""
	}
	return fi.ModTime().UTC().String()
}
