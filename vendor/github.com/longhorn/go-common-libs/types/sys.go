package types

const OsReleaseFilePath = "/etc/os-release"
const SysClassBlockDirectory = "/sys/class/block/"

const OSDistroTalosLinux = "talos"

// BlockDeviceInfo is a struct that contains the block device info.
type BlockDeviceInfo struct {
	Name  string // Name of the block device (e.g. sda, sdb, sdc, etc.).
	Major int    // Major number of the block device.
	Minor int    // Minor number of the block device.
}
