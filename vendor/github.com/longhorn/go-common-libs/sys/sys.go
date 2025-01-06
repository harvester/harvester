package sys

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/longhorn/go-common-libs/types"

	commonio "github.com/longhorn/go-common-libs/io"
)

// GetKernelRelease returns the kernel release string.
func GetKernelRelease() (string, error) {
	utsname := &unix.Utsname{}
	if err := unix.Uname(utsname); err != nil {
		logrus.WithError(err).Warn("Failed to get kernel release")
		return "", err
	}

	// Extract the kernel release from the Utsname structure
	release := make([]byte, 0, len(utsname.Release))
	for _, b := range utsname.Release {
		if b == 0x00 {
			logrus.Trace("Found end of kernel release string [0x00]")
			break
		}
		release = append(release, byte(b))
	}
	return string(release), nil
}

// GetOSDistro reads the /etc/os-release file and returns the ID field.
func GetOSDistro(osReleaseContent string) (string, error) {
	var err error
	defer func() {
		err = errors.Wrapf(err, "failed to get host OS distro")
	}()

	lines := strings.Split(osReleaseContent, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "ID=") {
			id := strings.TrimPrefix(line, "ID=")
			id = strings.Trim(id, `"`)
			logrus.Tracef("Found OS distro: %v", id)
			return id, nil
		}
	}

	return "", errors.Errorf("failed to find ID field in %v", types.OsReleaseFilePath)
}

// GetSystemBlockDeviceInfo returns the block device info for the system.
func GetSystemBlockDeviceInfo() (map[string]types.BlockDeviceInfo, error) {
	return getSystemBlockDeviceInfo(types.SysClassBlockDirectory, os.ReadDir, os.ReadFile)
}

// getSystemBlockDeviceInfo returns the block device info for the system.
// It injects the readDirFn and readFileFn for testing.
func getSystemBlockDeviceInfo(sysClassBlockDirectory string, readDirFn func(string) ([]os.DirEntry, error), readFileFn func(string) ([]byte, error)) (map[string]types.BlockDeviceInfo, error) {
	devices, err := readDirFn(sysClassBlockDirectory)
	if err != nil {
		return nil, err
	}

	readDeviceNumber := func(numbers []string, index int) (int64, error) {
		if len(numbers) <= index {
			return 0, errors.Errorf("invalid file format")
		}

		number, err := strconv.ParseInt(numbers[index], 10, 64)
		if err != nil {
			return 0, err
		}
		return number, nil
	}

	deviceInfo := make(map[string]types.BlockDeviceInfo, len(devices))
	for _, device := range devices {
		deviceName := device.Name()
		devicePath := filepath.Join(sysClassBlockDirectory, deviceName, "dev")

		if _, err := os.Stat(devicePath); os.IsNotExist(err) {
			// If the device path does not exist, check if the device path exists in the "device" directory.
			// Some devices such as "nvme0cn1" created from SPDK do not have "dev" file under their sys/class/block directory.
			alternativeDevicePath := filepath.Join(sysClassBlockDirectory, deviceName, "device", "dev")
			if _, altErr := os.Stat(alternativeDevicePath); os.IsNotExist(altErr) {
				errs := fmt.Errorf("primary error: %w; alternative error: %w", err, altErr)
				logrus.WithFields(logrus.Fields{
					"device":          deviceName,
					"primaryPath":     devicePath,
					"alternativePath": alternativeDevicePath,
				}).WithError(errs).Debugf("failed to find dev file in either primary or alternative path")
				continue
			}

			devicePath = alternativeDevicePath
		}

		data, err := readFileFn(devicePath)
		if err != nil {
			return nil, err
		}

		numbers := strings.Split(strings.TrimSpace(string(data)), ":")
		major, err := readDeviceNumber(numbers, 0)
		if err != nil {
			logrus.WithError(err).Warnf("failed to read device %s major", deviceName)
			continue
		}

		minor, err := readDeviceNumber(numbers, 1)
		if err != nil {
			logrus.WithError(err).Warnf("failed to read device %s minor", deviceName)
			continue
		}

		deviceInfo[deviceName] = types.BlockDeviceInfo{
			Name:  deviceName,
			Major: int(major),
			Minor: int(minor),
		}
	}
	return deviceInfo, nil
}

// GetBootKernelConfigMap reads the kernel config into a key-value map. It tries to read kernel config from
// ${bootDir}/config-${kernelVersion}, and comments are ignored. If the bootDir is empty, it points to /boot by default.
func GetBootKernelConfigMap(bootDir, kernelVersion string) (configMap map[string]string, err error) {
	if kernelVersion == "" {
		return nil, fmt.Errorf("kernelVersion cannot be empty")
	}
	if bootDir == "" {
		bootDir = types.SysBootDirectory
	}

	defer func() {
		err = errors.Wrapf(err, "failed to get kernel config map from %s", bootDir)
	}()

	configPath := filepath.Join(bootDir, "config-"+kernelVersion)
	configContent, err := commonio.ReadFileContent(configPath)
	if err != nil {
		return nil, err
	}
	return parseKernelModuleConfigMap(strings.NewReader(configContent))
}

// GetProcKernelConfigMap reads the kernel config into a key-value map. It tries to read kernel config from
// procfs mounted at procDir from the view of processName in namespace. If the procDir is empty, it points to /proc by
// default.
func GetProcKernelConfigMap(procDir string) (configMap map[string]string, err error) {
	if procDir == "" {
		procDir = types.SysProcDirectory
	}

	defer func() {
		err = errors.Wrapf(err, "failed to get kernel config map from %s", procDir)
	}()

	configPath := filepath.Join(procDir, types.SysKernelConfigGz)
	configFile, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer configFile.Close()
	gzReader, err := gzip.NewReader(configFile)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()
	return parseKernelModuleConfigMap(gzReader)
}

// parseKernelModuleConfigMap parses the kernel config into key-value map. All commented items will be ignored.
func parseKernelModuleConfigMap(contentReader io.Reader) (map[string]string, error) {
	configMap := map[string]string{}

	scanner := bufio.NewScanner(contentReader)
	for scanner.Scan() {
		config := scanner.Text()
		if !strings.HasPrefix(config, "CONFIG_") {
			continue
		}
		key, val, parsable := strings.Cut(config, "=")
		if !parsable {
			return nil, fmt.Errorf("failed to parse kernel config %s", config)
		}
		configMap[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return configMap, nil
}
