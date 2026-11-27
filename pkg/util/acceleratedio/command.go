package acceleratedio

import (
	"fmt"
	"strings"
)

const binaryPath = "/usr/bin/harvester"

// ReadCommand streams a block device to stdout using io-mode.
func ReadCommand(devicePath string) string {
	return command(devicePath, "read")
}

// WriteCommand writes stdin to a block device using io-mode.
func WriteCommand(devicePath string) string {
	return command(devicePath, "write")
}

func command(devicePath, mode string) string {
	device := "'" + strings.ReplaceAll(devicePath, "'", "'\"'\"'") + "'"
	return fmt.Sprintf("%s io-mode -device %s -mode=%s", binaryPath, device, mode)
}
