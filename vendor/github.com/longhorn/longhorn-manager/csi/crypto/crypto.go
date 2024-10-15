package crypto

import (
	"fmt"
	"path"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	lhns "github.com/longhorn/go-common-libs/ns"
	lhtypes "github.com/longhorn/go-common-libs/types"
)

const (
	mapperFilePathPrefix = "/dev/mapper"

	CryptoKeyDefaultCipher = "aes-xts-plain64"
	CryptoKeyDefaultHash   = "sha256"
	CryptoKeyDefaultSize   = "256"
	CryptoDefaultPBKDF     = "argon2i"

	// Luks2MinimalVolumeSize the minimal volume size for the LUKS2format encryption.
	//  https://gitlab.com/cryptsetup/cryptsetup/-/wikis/FrequentlyAskedQuestions
	//  Section 10.10 What about the size of the LUKS2 header
	//  The default size is 16MB
	Luks2MinimalVolumeSize = 16 * 1024 * 1024
)

// EncryptParams keeps the customized cipher options from the secret CR
type EncryptParams struct {
	KeyProvider string
	KeyCipher   string
	KeyHash     string
	KeySize     string
	PBKDF       string
}

func NewEncryptParams(keyProvider, keyCipher, keyHash, keySize, pbkdf string) *EncryptParams {
	return &EncryptParams{KeyProvider: keyProvider, KeyCipher: keyCipher, KeyHash: keyHash, KeySize: keySize, PBKDF: pbkdf}
}

func (cp *EncryptParams) GetKeyCipher() string {
	if cp.KeyCipher == "" {
		return CryptoKeyDefaultCipher
	}
	return cp.KeyCipher
}

func (cp *EncryptParams) GetKeyHash() string {
	if cp.KeyHash == "" {
		return CryptoKeyDefaultHash
	}
	return cp.KeyHash
}

func (cp *EncryptParams) GetKeySize() string {
	if cp.KeySize == "" {
		return CryptoKeyDefaultSize
	}
	return cp.KeySize
}

func (cp *EncryptParams) GetPBKDF() string {
	if cp.PBKDF == "" {
		return CryptoDefaultPBKDF
	}
	return cp.PBKDF
}

// VolumeMapper returns the path for mapped encrypted device.
func VolumeMapper(volume string) string {
	return path.Join(mapperFilePathPrefix, volume)
}

// EncryptVolume encrypts provided device with LUKS.
func EncryptVolume(devicePath, passphrase string, cryptoParams *EncryptParams) error {
	namespaces := []lhtypes.Namespace{lhtypes.NamespaceMnt, lhtypes.NamespaceIpc}
	nsexec, err := lhns.NewNamespaceExecutor(lhtypes.ProcessNone, lhtypes.HostProcDirectory, namespaces)
	if err != nil {
		return err
	}

	logrus.Infof("Encrypting device %s with LUKS", devicePath)
	if _, err := nsexec.LuksFormat(
		devicePath, passphrase,
		cryptoParams.GetKeyCipher(),
		cryptoParams.GetKeyHash(),
		cryptoParams.GetKeySize(),
		cryptoParams.GetPBKDF(),
		lhtypes.LuksTimeout); err != nil {
		return errors.Wrapf(err, "failed to encrypt device %s with LUKS", devicePath)
	}
	return nil
}

// OpenVolume opens volume so that it can be used by the client.
func OpenVolume(volume, devicePath, passphrase string) error {
	if isOpen, _ := IsDeviceOpen(VolumeMapper(volume)); isOpen {
		logrus.Infof("Device %s is already opened at %s", devicePath, VolumeMapper(volume))
		return nil
	}

	namespaces := []lhtypes.Namespace{lhtypes.NamespaceMnt, lhtypes.NamespaceIpc}
	nsexec, err := lhns.NewNamespaceExecutor(lhtypes.ProcessNone, lhtypes.HostProcDirectory, namespaces)
	if err != nil {
		return err
	}

	logrus.Infof("Opening device %s with LUKS on %s", devicePath, volume)
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

	logrus.Infof("Closing LUKS device %s", volume)
	_, err = nsexec.LuksClose(volume, lhtypes.LuksTimeout)
	return err
}

func ResizeEncryptoDevice(volume, passphrase string) error {
	if isOpen, err := IsDeviceOpen(VolumeMapper(volume)); err != nil {
		return err
	} else if !isOpen {
		return fmt.Errorf("volume %v encrypto device is closed for resizing", volume)
	}

	namespaces := []lhtypes.Namespace{lhtypes.NamespaceMnt, lhtypes.NamespaceIpc}
	nsexec, err := lhns.NewNamespaceExecutor(lhtypes.ProcessNone, lhtypes.HostProcDirectory, namespaces)
	if err != nil {
		return err
	}

	_, err = nsexec.LuksResize(volume, passphrase, lhtypes.LuksTimeout)
	return err
}

// IsDeviceMappedToNullPath determines if encrypted device is already open at a null path. The command 'cryptsetup status [crypted_device]' show "device:  (null)"
func IsDeviceMappedToNullPath(device string) (bool, error) {
	devPath, mappedFile, err := DeviceEncryptionStatus(device)
	if err != nil {
		return false, err
	}

	return mappedFile != "" && strings.Compare(devPath, "(null)") == 0, nil
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
	if !strings.HasPrefix(devicePath, mapperFilePathPrefix) {
		return devicePath, "", nil
	}

	namespaces := []lhtypes.Namespace{lhtypes.NamespaceMnt, lhtypes.NamespaceIpc}
	nsexec, err := lhns.NewNamespaceExecutor(lhtypes.ProcessNone, lhtypes.HostProcDirectory, namespaces)
	if err != nil {
		return devicePath, "", err
	}

	volume := strings.TrimPrefix(devicePath, mapperFilePathPrefix+"/")
	stdout, err := nsexec.LuksStatus(volume, lhtypes.LuksTimeout)
	if err != nil {
		logrus.WithError(err).Warnf("Device %s is not an active LUKS device", devicePath)
		return devicePath, "", nil
	}

	lines := strings.Split(string(stdout), "\n")
	if len(lines) < 1 {
		return "", "", fmt.Errorf("device encryption status returned no stdout for %s", devicePath)
	}

	if !strings.Contains(lines[0], " is active") {
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
