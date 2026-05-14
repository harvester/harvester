package util

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var (
	sleepInterval = 5 * time.Second
)

// SleepAndReboot do sleep and exec reboot
func SleepAndReboot() error {
	time.Sleep(sleepInterval)
	return exec.Command("/usr/sbin/reboot", "-f").Run()
}

// Get disk size in bytes
func GetDiskSizeBytes(devPath string) (uint64, error) {
	cmd := exec.Command("/usr/bin/lsblk", "--bytes", "--nodeps", "--raw", "--noheadings", "--output=SIZE", devPath)
	bytes, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	output := strings.TrimSpace(string(bytes))
	sizeBytes, err := strconv.ParseUint(output, 10, 64)
	if err != nil {
		return 0, err
	}

	return uint64(sizeBytes), nil
}

func SystemIsBIOS() bool {
	if _, err := os.Stat("/sys/firmware/efi"); os.IsNotExist(err) {
		return true
	}
	return false
}
