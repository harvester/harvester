package server

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	mount "k8s.io/mount-utils"

	"github.com/longhorn/longhorn-share-manager/pkg/crypto"
	"github.com/longhorn/longhorn-share-manager/pkg/server/nfs"
	"github.com/longhorn/longhorn-share-manager/pkg/types"
	"github.com/longhorn/longhorn-share-manager/pkg/volume"
)

const waitBetweenChecks = time.Second * 5
const healthCheckInterval = time.Second * 10
const configPath = "/tmp/vfs.conf"

const (
	UnhealthyErr = "UNHEALTHY: volume with mount path %v is unhealthy"
	ReadOnlyErr  = "READONLY: volume with mount path %v is read only"
)

type ShareManager struct {
	logger logrus.FieldLogger

	volume        volume.Volume
	shareExported bool

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

func (m *ShareManager) Run() error {
	vol := m.volume
	mountPath := types.GetMountPath(vol.Name)
	devicePath := types.GetVolumeDevicePath(vol.Name, false)

	defer func() {
		// if the server is exiting, try to unmount & teardown device before we terminate the container
		if err := volume.UnmountVolume(mountPath); err != nil {
			m.logger.WithError(err).Error("Failed to unmount volume")
		}

		if err := m.tearDownDevice(vol); err != nil {
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

			devicePath, err := m.setupDevice(vol, devicePath)
			if err != nil {
				return err
			}

			if err := m.MountVolume(vol, devicePath, mountPath); err != nil {
				m.logger.WithError(err).Warn("Failed to mount volume")
				return err
			}

			if err := m.resizeVolume(devicePath, mountPath); err != nil {
				m.logger.WithError(err).Warn("Failed to resize volume after mount")
				return err
			}

			if err := volume.SetPermissions(mountPath, 0777); err != nil {
				m.logger.WithError(err).Error("Failed to set permissions for volume")
				return err
			}

			m.logger.Info("Starting nfs server, volume is ready for export")
			go m.runHealthCheck()

			if _, err := m.nfsServer.CreateExport(vol.Name); err != nil {
				m.logger.WithError(err).Error("Failed to create nfs export")
				return err
			}

			m.SetShareExported(true)

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
func (m *ShareManager) setupDevice(vol volume.Volume, devicePath string) (string, error) {
	diskFormat, err := volume.GetDiskFormat(devicePath)
	if err != nil {
		return "", errors.Wrap(err, "failed to determine filesystem format of volume error")
	}
	m.logger.Infof("Volume %v device %v contains filesystem of format %v", vol.Name, devicePath, diskFormat)

	if vol.IsEncrypted() || diskFormat == "luks" {
		if vol.Passphrase == "" {
			return "", fmt.Errorf("missing passphrase for encrypted volume %v", vol.Name)
		}

		// initial setup of longhorn device for crypto
		if diskFormat == "" {
			m.logger.Info("Encrypting new volume before first use")
			if err := crypto.EncryptVolume(devicePath, vol.Passphrase, vol.CryptoKeyCipher, vol.CryptoKeyHash, vol.CryptoKeySize, vol.CryptoPBKDF); err != nil {
				return "", errors.Wrapf(err, "failed to encrypt volume %v", vol.Name)
			}
		}

		cryptoDevice := types.GetVolumeDevicePath(vol.Name, true)
		m.logger.Infof("Volume %s requires crypto device %s", vol.Name, cryptoDevice)
		if err := crypto.OpenVolume(vol.Name, devicePath, vol.Passphrase); err != nil {
			m.logger.WithError(err).Error("Failed to open encrypted volume")
			return "", err
		}

		// update the device path to point to the new crypto device
		return cryptoDevice, nil
	}

	return devicePath, nil
}

func (m *ShareManager) tearDownDevice(vol volume.Volume) error {
	// close any matching crypto device for this volume
	cryptoDevice := types.GetVolumeDevicePath(vol.Name, true)
	if isOpen, err := crypto.IsDeviceOpen(cryptoDevice); err != nil {
		return err
	} else if isOpen {
		m.logger.Infof("Volume %s has active crypto device %s", vol.Name, cryptoDevice)
		if err := crypto.CloseVolume(vol.Name); err != nil {
			return err
		}
		m.logger.Infof("Volume %s closed active crypto device %s", vol.Name, cryptoDevice)
	}

	return nil
}

func (m *ShareManager) MountVolume(vol volume.Volume, devicePath, mountPath string) error {
	fsType := vol.FsType
	mountOptions := vol.MountOptions

	// https://github.com/longhorn/longhorn/issues/2991
	// pre v1.2 we ignored the fsType and always formatted as ext4
	// after v1.2 we include the user specified fsType to be able to
	// mount priorly created volumes we need to switch to the existing fsType
	diskFormat, err := volume.GetDiskFormat(devicePath)
	if err != nil {
		m.logger.WithError(err).Error("Failed to evaluate disk format")
		return err
	}

	// `unknown data, probably partitions` is used when the disk contains a partition table
	if diskFormat != "" && !strings.Contains(diskFormat, "unknown data") && fsType != diskFormat {
		m.logger.Warnf("Disk is already formatted to %v but user requested fs is %v using existing device fs type for mount", diskFormat, fsType)
		fsType = diskFormat
	}

	return volume.MountVolume(devicePath, mountPath, fsType, mountOptions)
}

func (m *ShareManager) resizeVolume(devicePath, mountPath string) error {
	if resized, err := volume.ResizeVolume(devicePath, mountPath); err != nil {
		m.logger.WithError(err).Error("Failed to resize filesystem for volume")
		return err
	} else if resized {
		m.logger.Info("Resized filesystem for volume after mount")
	}

	return nil
}

func (m *ShareManager) runHealthCheck() {
	m.logger.Infof("Starting health check for volume mounted at: %v", types.GetMountPath(m.volume.Name))
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.context.Done():
			m.logger.Info("NFS server is shutting down")
			return
		case <-ticker.C:
			if err := m.hasHealthyVolume(); err != nil {
				if strings.Contains(err.Error(), "UNHEALTHY") {
					m.logger.WithError(err).Error("Terminating")
					m.Shutdown()
					return
				} else if strings.Contains(err.Error(), "READONLY") {
					m.logger.WithError(err).Warn("Recovering read only volume")
					if err := m.recoverReadOnlyVolume(); err != nil {
						m.logger.WithError(err).Error("Volume is unable to recover by remounting, terminating")
						m.Shutdown()
						return
					}
				}
			}
		}
	}
}

func (m *ShareManager) hasHealthyVolume() error {
	mountPath := types.GetMountPath(m.volume.Name)
	if err := exec.CommandContext(m.context, "ls", mountPath).Run(); err != nil {
		return fmt.Errorf(UnhealthyErr, mountPath)
	}

	mounter := mount.New("")
	mountPoints, _ := mounter.List()
	for _, mp := range mountPoints {
		if mp.Path == mountPath && isMountPointReadOnly(mp) {
			return fmt.Errorf(ReadOnlyErr, mountPath)
		}
	}
	return nil
}

func (m *ShareManager) recoverReadOnlyVolume() error {
	mountPath := types.GetMountPath(m.volume.Name)

	cmd := exec.CommandContext(m.context, "mount", "-o", "remount,rw", mountPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "remount failed with output: %s", out)
	}

	return nil
}

func (m *ShareManager) GetVolume() volume.Volume {
	return m.volume
}

func (m *ShareManager) SetShareExported(val bool) {
	m.shareExported = val
}

func (m *ShareManager) ShareIsExported() bool {
	return m.shareExported
}

func (m *ShareManager) Shutdown() {
	m.shutdown()
}

func isMountPointReadOnly(mp mount.MountPoint) bool {
	for _, opt := range mp.Opts {
		if opt == "ro" {
			return true
		}
	}
	return false
}
