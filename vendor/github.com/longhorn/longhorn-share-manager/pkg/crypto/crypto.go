package crypto

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	lhns "github.com/longhorn/go-common-libs/ns"
	lhtypes "github.com/longhorn/go-common-libs/types"

	"github.com/longhorn/longhorn-share-manager/pkg/types"
)

// EncryptVolume encrypts provided device with LUKS.
func EncryptVolume(devicePath, passphrase, keyCipher, keyHash, keySize, pbkdf string) error {
	namespaces := []lhtypes.Namespace{lhtypes.NamespaceMnt, lhtypes.NamespaceIpc}
	nsexec, err := lhns.NewNamespaceExecutor(lhtypes.ProcessNone, lhtypes.HostProcDirectory, namespaces)
	if err != nil {
		return err
	}

	logrus.Debugf("Encrypting device %s with LUKS", devicePath)
	if _, err := nsexec.LuksFormat(devicePath, passphrase, keyCipher, keyHash, keySize, pbkdf, lhtypes.LuksTimeout); err != nil {
		return errors.Wrapf(err, "failed to encrypt device %s with LUKS", devicePath)
	}
	return nil
}

// OpenVolume opens volume so that it can be used by the client.
func OpenVolume(volume, devicePath, passphrase string) error {
	devPath := types.GetVolumeDevicePath(volume, true)
	if isOpen, _ := IsDeviceOpen(devPath); isOpen {
		logrus.Debugf("Device %s is already opened at %s", devicePath, devPath)
		return nil
	}

	namespaces := []lhtypes.Namespace{lhtypes.NamespaceMnt, lhtypes.NamespaceIpc}
	nsexec, err := lhns.NewNamespaceExecutor(lhtypes.ProcessNone, lhtypes.HostProcDirectory, namespaces)
	if err != nil {
		return err
	}

	logrus.Debugf("Opening device %s with LUKS on %s", devicePath, volume)
	_, err = nsexec.LuksOpen(volume, devicePath, passphrase, lhtypes.LuksTimeout)
	if err != nil {
		logrus.WithError(err).Warnf("Failed to open LUKS device %s", devicePath)
	}
	return err
}

// CloseVolume closes encrypted volume so it can be detached.
func CloseVolume(volume string) error {
	namespaces := []lhtypes.Namespace{lhtypes.NamespaceMnt, lhtypes.NamespaceIpc}
	nsexec, err := lhns.NewNamespaceExecutor(lhtypes.ProcessNone, lhtypes.HostProcDirectory, namespaces)
	if err != nil {
		return err
	}

	logrus.Debugf("Closing LUKS device %s", volume)
	_, err = nsexec.LuksClose(volume, lhtypes.LuksTimeout)
	return err
}

// IsDeviceOpen determines if encrypted device is already open.
func IsDeviceOpen(device string) (bool, error) {
	_, mappedFile, err := DeviceEncryptionStatus(device)
	return mappedFile != "", err
}

// DeviceEncryptionStatus looks to identify if the passed device is a LUKS mapping
// and if so what the device is and the mapper name as used by LUKS.
// If not, just returns the original device and an empty string.
func DeviceEncryptionStatus(devicePath string) (mappedDevice, mapper string, err error) {
	if !strings.HasPrefix(devicePath, types.MapperDevPath) {
		return devicePath, "", nil
	}

	namespaces := []lhtypes.Namespace{lhtypes.NamespaceMnt, lhtypes.NamespaceIpc}
	nsexec, err := lhns.NewNamespaceExecutor(lhtypes.ProcessNone, lhtypes.HostProcDirectory, namespaces)
	if err != nil {
		return devicePath, "", err
	}

	volume := strings.TrimPrefix(devicePath, types.MapperDevPath+"/")
	stdout, err := nsexec.LuksStatus(volume, lhtypes.LuksTimeout)
	if err != nil {
		logrus.WithError(err).Debugf("Device %s is not an active LUKS device", devicePath)
		return devicePath, "", nil
	}
	lines := strings.Split(string(stdout), "\n")
	if len(lines) < 1 {
		return "", "", fmt.Errorf("device encryption status returned no stdout for %s", devicePath)
	}
	if !strings.HasSuffix(lines[0], " is active.") {
		// Implies this is not a LUKS device
		return devicePath, "", nil
	}
	for i := 1; i < len(lines); i++ {
		kv := strings.SplitN(strings.TrimSpace(lines[i]), ":", 2)
		if len(kv) < 1 {
			return "", "", fmt.Errorf("device encryption status output for %s is badly formatted: %s",
				devicePath, lines[i])
		}
		if strings.Compare(kv[0], "device") == 0 {
			return strings.TrimSpace(kv[1]), volume, nil
		}
	}
	// Identified as LUKS, but failed to identify a mapped device
	return "", "", fmt.Errorf("mapped device not found in path %s", devicePath)
}
