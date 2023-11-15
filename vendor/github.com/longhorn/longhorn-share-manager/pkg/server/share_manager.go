package server

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/longhorn/longhorn-share-manager/pkg/crypto"
	"github.com/longhorn/longhorn-share-manager/pkg/server/nfs"
	"github.com/longhorn/longhorn-share-manager/pkg/types"
	"github.com/longhorn/longhorn-share-manager/pkg/volume"
)

const waitBetweenChecks = time.Second * 5
const healthCheckInterval = time.Second * 10
const configPath = "/tmp/vfs.conf"

type ShareManager struct {
	logger logrus.FieldLogger

	volume volume.Volume

	context  context.Context
	shutdown context.CancelFunc

	nfsServer *nfs.Server
}

func NewShareManager(logger logrus.FieldLogger, volume volume.Volume) (*ShareManager, error) {
	m := &ShareManager{
		volume: volume,
		logger: logger.WithField("volume", volume.Name).WithField("encrypted", volume.IsEncrypted()),
	}
	m.context, m.shutdown = context.WithCancel(context.Background())

	nfsServer, err := nfs.NewServer(logger, configPath, types.ExportPath, volume.Name)
	if err != nil {
		return nil, err
	}
	m.nfsServer = nfsServer
	return m, nil
}

func (m *ShareManager) GetVolumeName() string {
	return m.volume.Name
}

func (m *ShareManager) Run() error {
	vol := m.volume
	mountPath := types.GetMountPath(vol.Name)
	devicePath := types.GetVolumeDevicePath(vol.Name, false)

	defer func() {
		// if the server is exiting, try to unmount & teardown device before we terminate the container
		if err := volume.UnmountVolume(mountPath); err != nil {
			m.logger.WithError(err).Error("Failed to unmount volume")
		}

		if err := tearDownDevice(m.logger, vol); err != nil {
			m.logger.WithError(err).Error("Failed to tear down volume")
		}

		m.Shutdown()
	}()

	for ; ; time.Sleep(waitBetweenChecks) {
		select {
		case <-m.context.Done():
			m.logger.Info("NFS server is shutting down")
			return nil
		default:
			if !volume.CheckDeviceValid(devicePath) {
				m.logger.Warn("Waiting with nfs server start, volume is not attached")
				break
			}

			devicePath, err := setupDevice(m.logger, vol, devicePath)
			if err != nil {
				return err
			}

			if err := mountVolume(m.logger, vol, devicePath, mountPath); err != nil {
				m.logger.WithError(err).Warn("Failed to mount volume")
				return err
			}

			if err := resizeVolume(m.logger, devicePath, mountPath); err != nil {
				m.logger.WithError(err).Warn("Failed to resize volume after mount")
				return err
			}

			if err := volume.SetPermissions(mountPath, 0777); err != nil {
				m.logger.WithError(err).Error("Failed to set permissions for volume")
				return err
			}

			m.logger.Info("Starting nfs server, volume is ready for export")
			go m.runHealthCheck()

			// This blocks until server exist
			if err := m.nfsServer.Run(m.context); err != nil {
				m.logger.WithError(err).Error("NFS server exited with error")
			}
			return err
		}
	}
}

// setupDevice will return a path where the device file can be found
// for encrypted volumes, it will try formatting the volume on first use
// then open it and expose a crypto device at the returned path
func setupDevice(logger logrus.FieldLogger, vol volume.Volume, devicePath string) (string, error) {
	diskFormat, err := volume.GetDiskFormat(devicePath)
	if err != nil {
		return "", errors.Wrap(err, "failed to determine filesystem format of volume error")
	}
	logger.Debugf("Volume %v device %v contains filesystem of format %v", vol.Name, devicePath, diskFormat)

	if vol.IsEncrypted() || diskFormat == "luks" {
		if vol.Passphrase == "" {
			return "", fmt.Errorf("missing passphrase for encrypted volume %v", vol.Name)
		}

		// initial setup of longhorn device for crypto
		if diskFormat == "" {
			logger.Info("Encrypting new volume before first use")
			if err := crypto.EncryptVolume(devicePath, vol.Passphrase, vol.CryptoKeyCipher, vol.CryptoKeyHash, vol.CryptoKeySize, vol.CryptoPBKDF); err != nil {
				return "", errors.Wrapf(err, "failed to encrypt volume %v", vol.Name)
			}
		}

		cryptoDevice := types.GetVolumeDevicePath(vol.Name, true)
		logger.Infof("Volume %s requires crypto device %s", vol.Name, cryptoDevice)
		if err := crypto.OpenVolume(vol.Name, devicePath, vol.Passphrase); err != nil {
			logger.WithError(err).Error("Failed to open encrypted volume")
			return "", err
		}

		// update the device path to point to the new crypto device
		return cryptoDevice, nil
	}

	return devicePath, nil
}

func tearDownDevice(logger logrus.FieldLogger, vol volume.Volume) error {
	// close any matching crypto device for this volume
	cryptoDevice := types.GetVolumeDevicePath(vol.Name, true)
	if isOpen, err := crypto.IsDeviceOpen(cryptoDevice); err != nil {
		return err
	} else if isOpen {
		logger.Infof("Volume %s has active crypto device %s", vol.Name, cryptoDevice)
		if err := crypto.CloseVolume(vol.Name); err != nil {
			return err
		}
		logger.Infof("Volume %s closed active crypto device %s", vol.Name, cryptoDevice)
	}

	return nil
}

func mountVolume(logger logrus.FieldLogger, vol volume.Volume, devicePath, mountPath string) error {
	fsType := vol.FsType
	mountOptions := vol.MountOptions

	// https://github.com/longhorn/longhorn/issues/2991
	// pre v1.2 we ignored the fsType and always formatted as ext4
	// after v1.2 we include the user specified fsType to be able to
	// mount priorly created volumes we need to switch to the existing fsType
	diskFormat, err := volume.GetDiskFormat(devicePath)
	if err != nil {
		logger.WithError(err).Error("Failed to evaluate disk format")
		return err
	}

	// `unknown data, probably partitions` is used when the disk contains a partition table
	if diskFormat != "" && !strings.Contains(diskFormat, "unknown data") && fsType != diskFormat {
		logger.Warnf("Disk is already formatted to %v but user requested fs is %v using existing device fs type for mount", diskFormat, fsType)
		fsType = diskFormat
	}

	return volume.MountVolume(devicePath, mountPath, fsType, mountOptions)
}

func resizeVolume(logger logrus.FieldLogger, devicePath, mountPath string) error {
	if resized, err := volume.ResizeVolume(devicePath, mountPath); err != nil {
		logger.WithError(err).Error("Failed to resize filesystem for volume")
		return err
	} else if resized {
		logger.Info("Resized filesystem for volume after mount")
	}

	return nil
}

func (m *ShareManager) runHealthCheck() {
	m.logger.Info("Starting health check for volume")
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.context.Done():
			m.logger.Info("NFS server is shutting down")
			return
		case <-ticker.C:
			if !m.hasHealthyVolume() {
				m.logger.Error("Volume health check failed, terminating")
				m.Shutdown()
				return
			}
		}
	}
}

func (m *ShareManager) hasHealthyVolume() bool {
	mountPath := types.GetMountPath(m.volume.Name)
	err := exec.CommandContext(m.context, "ls", mountPath).Run()
	return err == nil
}

func (m *ShareManager) Shutdown() {
	m.shutdown()
}
