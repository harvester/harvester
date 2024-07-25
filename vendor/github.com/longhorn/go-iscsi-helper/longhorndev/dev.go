package longhorndev

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/longhorn/go-iscsi-helper/iscsidev"
	"github.com/longhorn/go-iscsi-helper/types"
	"github.com/longhorn/go-iscsi-helper/util"
)

const (
	SocketDirectory = "/var/run"
	DevPath         = "/dev/longhorn/"

	WaitInterval = time.Second
	WaitCount    = 30
)

type LonghornDevice struct {
	*sync.RWMutex
	name                      string //VolumeName
	size                      int64
	frontend                  string
	endpoint                  string
	scsiTimeout               int64
	iscsiAbortTimeout         int64
	iscsiTargetRequestTimeout int64

	scsiDevice *iscsidev.Device
}

type DeviceService interface {
	GetFrontend() string
	SetFrontend(frontend string) error
	UnsetFrontendCheck() error
	UnsetFrontend()
	GetEndpoint() string
	Enabled() bool

	InitDevice() error
	Start() error
	Shutdown() error
	PrepareUpgrade() error
	FinishUpgrade() error
	Expand(size int64) error
}

type DeviceCreator interface {
	NewDevice(name string, size int64, frontend string) (DeviceService, error)
}

type LonghornDeviceCreator struct{}

func (ldc *LonghornDeviceCreator) NewDevice(name string, size int64, frontend string, scsiTimeout, iscsiAbortTimeout, iscsiTargetRequestTimeout int64) (DeviceService, error) {
	if name == "" || size == 0 {
		return nil, fmt.Errorf("invalid parameter for creating Longhorn device")
	}
	dev := &LonghornDevice{
		RWMutex:                   &sync.RWMutex{},
		name:                      name,
		size:                      size,
		scsiTimeout:               scsiTimeout,
		iscsiAbortTimeout:         iscsiAbortTimeout,
		iscsiTargetRequestTimeout: iscsiTargetRequestTimeout,
	}
	if err := dev.SetFrontend(frontend); err != nil {
		return nil, err
	}
	return dev, nil
}

func (d *LonghornDevice) InitDevice() error {
	d.Lock()
	defer d.Unlock()

	if d.scsiDevice != nil {
		return nil
	}

	if err := d.initScsiDevice(); err != nil {
		return err
	}

	// Try to cleanup possible leftovers.
	return d.shutdownFrontend()
}

// call with lock hold
func (d *LonghornDevice) initScsiDevice() error {
	bsOpts := fmt.Sprintf("size=%v;request_timeout=%v", d.size, d.iscsiTargetRequestTimeout)
	scsiDev, err := iscsidev.NewDevice(d.name, d.GetSocketPath(), "longhorn", bsOpts, d.scsiTimeout, d.iscsiAbortTimeout)
	if err != nil {
		return err
	}
	d.scsiDevice = scsiDev

	return nil
}

func (d *LonghornDevice) Start() error {
	stopCh := make(chan struct{})
	if err := <-d.WaitForSocket(stopCh); err != nil {
		return err
	}

	return d.startScsiDevice(true)
}

func (d *LonghornDevice) startScsiDevice(startScsiDevice bool) (err error) {
	d.Lock()
	defer d.Unlock()

	switch d.frontend {
	case types.FrontendTGTBlockDev:
		// If iSCSI device is not started here, e.g., device upgrade,
		// d.scsiDevice.KernelDevice is nil.
		if startScsiDevice {
			if d.scsiDevice == nil {
				return fmt.Errorf("there is no iSCSI device during the frontend %v starts", d.frontend)
			}
			if err := d.scsiDevice.CreateTarget(); err != nil {
				return err
			}
			if err := d.scsiDevice.StartInitator(); err != nil {
				return err
			}
			if err := d.createDev(); err != nil {
				return err
			}
			logrus.Infof("device %v: iSCSI device %s created", d.name, d.scsiDevice.KernelDevice.Name)
		} else {
			if err := d.scsiDevice.ReloadTargetID(); err != nil {
				return err
			}
			if err := d.scsiDevice.ReloadInitiator(); err != nil {
				return err
			}
			logrus.Infof("device %v: iSCSI device %s reloaded the target and the initiator", d.name, d.scsiDevice.KernelDevice.Name)
		}

		d.endpoint = d.getDev()

	case types.FrontendTGTISCSI:
		if startScsiDevice {
			if d.scsiDevice == nil {
				return fmt.Errorf("there is no iSCSI device during the frontend %v starts", d.frontend)
			}
			if err := d.scsiDevice.CreateTarget(); err != nil {
				return err
			}
			logrus.Infof("device %v: iSCSI target %s created", d.name, d.scsiDevice.Target)
		} else {
			if err := d.scsiDevice.ReloadTargetID(); err != nil {
				return err
			}
			logrus.Infof("device %v: iSCSI target %s reloaded the target ID", d.name, d.scsiDevice.Target)
		}

		d.endpoint = d.scsiDevice.Target

	default:
		return fmt.Errorf("unknown frontend %v", d.frontend)
	}

	logrus.Debugf("device %v: frontend start succeed", d.name)
	return nil
}

func (d *LonghornDevice) Shutdown() error {
	d.Lock()
	defer d.Unlock()

	if d.scsiDevice == nil {
		return nil
	}

	if err := d.shutdownFrontend(); err != nil {
		return err
	}

	d.scsiDevice = nil
	d.endpoint = ""

	return nil
}

// call with lock hold
func (d *LonghornDevice) shutdownFrontend() error {
	switch d.frontend {
	case types.FrontendTGTBlockDev:
		dev := d.getDev()
		if err := util.RemoveDevice(dev); err != nil {
			return errors.Wrapf(err, "device %v: failed to remove device %s", d.name, dev)
		}
		if err := d.scsiDevice.StopInitiator(); err != nil {
			return errors.Wrapf(err, "device %v: failed to stop iSCSI device", d.name)
		}
		if err := d.scsiDevice.DeleteTarget(); err != nil {
			return errors.Wrapf(err, "device %v: failed to delete target %v", d.name, d.scsiDevice.Target)
		}
		logrus.Infof("device %v: iSCSI device %v shutdown", d.name, dev)
	case types.FrontendTGTISCSI:
		if err := d.scsiDevice.DeleteTarget(); err != nil {
			return errors.Wrapf(err, "device %v: failed to delete target %v", d.name, d.scsiDevice.Target)
		}
		logrus.Infof("device %v: iSCSI target %v ", d.name, d.scsiDevice.Target)
	case "":
		logrus.Infof("device %v: skip shutdown frontend since it's not enabled", d.name)
	default:
		return fmt.Errorf("device %v: unknown frontend %v", d.name, d.frontend)
	}

	return nil
}

func (d *LonghornDevice) WaitForSocket(stopCh chan struct{}) chan error {
	errCh := make(chan error)
	go func(errCh chan error, stopCh chan struct{}) {
		socket := d.GetSocketPath()
		timeout := time.After(time.Duration(WaitCount) * WaitInterval)
		ticker := time.NewTicker(WaitInterval)
		defer ticker.Stop()
		tick := ticker.C
		for {
			select {
			case <-timeout:
				errCh <- fmt.Errorf("device %v: wait for socket %v timed out", d.name, socket)
			case <-tick:
				if _, err := os.Stat(socket); err == nil {
					errCh <- nil
					return
				}
				logrus.Infof("device %v: waiting for socket %v to show up", d.name, socket)
			case <-stopCh:
				logrus.Infof("device %v: stop wait for socket routine", d.name)
				return
			}
		}
	}(errCh, stopCh)

	return errCh
}

func (d *LonghornDevice) GetSocketPath() string {
	return filepath.Join(SocketDirectory, "longhorn-"+d.name+".sock")
}

// call with lock hold
func (d *LonghornDevice) getDev() string {
	return filepath.Join(DevPath, d.name)
}

// call with lock hold
func (d *LonghornDevice) createDev() error {
	if _, err := os.Stat(DevPath); os.IsNotExist(err) {
		if err := os.MkdirAll(DevPath, 0755); err != nil {
			logrus.Fatalf("device %v: cannot create directory %v", d.name, DevPath)
		}
	}

	dev := d.getDev()
	if _, err := os.Stat(dev); err == nil {
		logrus.Warnf("Device %s already exists, clean it up", dev)
		if err := util.RemoveDevice(dev); err != nil {
			return errors.Wrapf(err, "cannot clean up block device file %v", dev)
		}
	}

	if err := util.DuplicateDevice(d.scsiDevice.KernelDevice, dev); err != nil {
		return err
	}

	logrus.Debugf("device %v: Device %s is ready", d.name, dev)

	return nil
}

func (d *LonghornDevice) PrepareUpgrade() error {
	if d.frontend == "" {
		return nil
	}

	if err := util.RemoveFile(d.GetSocketPath()); err != nil {
		return errors.Wrapf(err, "failed to remove socket %v", d.GetSocketPath())
	}
	return nil
}

func (d *LonghornDevice) FinishUpgrade() (err error) {
	if d.frontend == "" {
		return nil
	}

	stopCh := make(chan struct{})
	socketError := d.WaitForSocket(stopCh)
	err = <-socketError
	if err != nil {
		err = errors.Wrap(err, "error waiting for the socket")
		logrus.Error(err)
	}

	close(stopCh)
	close(socketError)

	if err != nil {
		return err
	}

	// TODO: Need to fix `ReloadSocketConnection` since it doesn't work for frontend `FrontendTGTISCSI`.
	if err := d.ReloadSocketConnection(); err != nil {
		return err
	}

	d.Lock()
	if err := d.initScsiDevice(); err != nil {
		d.Unlock()
		return err
	}
	d.Unlock()

	return d.startScsiDevice(false)
}

func (d *LonghornDevice) ReloadSocketConnection() error {
	d.RLock()
	dev := d.getDev()
	d.RUnlock()

	cmd := exec.Command("sg_raw", dev, "a6", "00", "00", "00", "00", "00")
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "failed to reload socket connection at %v", dev)
	}
	logrus.Infof("Reloaded completed for device %v", dev)
	return nil
}

func (d *LonghornDevice) SetFrontend(frontend string) error {
	if frontend != types.FrontendTGTBlockDev && frontend != types.FrontendTGTISCSI && frontend != "" {
		return fmt.Errorf("invalid frontend %v", frontend)
	}

	d.Lock()
	defer d.Unlock()
	if d.frontend != "" {
		if d.frontend != frontend {
			return fmt.Errorf("engine frontend %v is already up and cannot be set to %v", d.frontend, frontend)
		}
		if d.scsiDevice != nil {
			logrus.Infof("Engine frontend %v is already up", frontend)
			return nil
		}
		// d.scsiDevice == nil
		return fmt.Errorf("engine frontend had been set to %v, but its frontend cannot be started before engine manager shutdown its frontend", frontend)
	}

	if d.scsiDevice != nil {
		return fmt.Errorf("BUG: engine launcher frontend is empty but scsi device hasn't been cleanup in frontend start")
	}

	d.frontend = frontend

	return nil
}

func (d *LonghornDevice) UnsetFrontendCheck() error {
	d.Lock()
	defer d.Unlock()

	if d.scsiDevice == nil {
		d.frontend = ""
		logrus.Debugf("Engine frontend is already down")
		return nil
	}

	if d.frontend == "" {
		return fmt.Errorf("BUG: engine launcher frontend is empty but scsi device hasn't been cleanup in frontend shutdown")
	}
	return nil
}

func (d *LonghornDevice) UnsetFrontend() {
	d.Lock()
	defer d.Unlock()

	d.frontend = ""
}

func (d *LonghornDevice) Enabled() bool {
	d.RLock()
	defer d.RUnlock()
	return d.scsiDevice != nil
}

func (d *LonghornDevice) GetEndpoint() string {
	d.RLock()
	defer d.RUnlock()
	return d.endpoint
}

func (d *LonghornDevice) GetFrontend() string {
	d.RLock()
	defer d.RUnlock()
	return d.frontend
}

func (d *LonghornDevice) Expand(size int64) (err error) {
	d.Lock()
	defer d.Unlock()

	if d.size > size {
		return fmt.Errorf("device %v: cannot expand the device from size %v to a smaller size %v", d.name, d.size, size)
	} else if d.size == size {
		return nil
	}

	defer func() {
		if err == nil {
			d.size = size
		}
	}()

	if d.scsiDevice == nil {
		logrus.Info("Device: No need to do anything for the expansion since the frontend is shutdown")
		return nil
	}
	if err := d.scsiDevice.UpdateScsiBackingStore("longhorn", fmt.Sprintf("size=%v", size)); err != nil {
		return err
	}

	switch d.frontend {
	case types.FrontendTGTBlockDev:
		logrus.Infof("Device %v: Expanding frontend %v target %v", d.name, d.frontend, d.scsiDevice.Target)
		if err := d.scsiDevice.ExpandTarget(size); err != nil {
			return fmt.Errorf("device %v: fail to expand target %v: %v", d.name, d.scsiDevice.Target, err)
		}
		logrus.Infof("Device %v: Refreshing/Rescanning frontend %v initiator for the expansion", d.name, d.frontend)
		if err := d.scsiDevice.RefreshInitiator(); err != nil {
			return fmt.Errorf("device %v: fail to refresh iSCSI initiator: %v", d.name, err)
		}
		logrus.Infof("Device %v: Expanded frontend %v size to %d", d.name, d.frontend, size)
	case types.FrontendTGTISCSI:
		logrus.Infof("Device %v: Frontend is expanding the target %v", d.name, d.scsiDevice.Target)
		if err := d.scsiDevice.ExpandTarget(size); err != nil {
			return fmt.Errorf("device %v: fail to expand target %v: %v", d.name, d.scsiDevice.Target, err)
		}
		logrus.Infof("Device %v: Expanded frontend %v size to %d, users need to refresh/rescan the initiator by themselves", d.name, d.frontend, size)
	case "":
		logrus.Infof("Device %v: skip expansion since the frontend not enabled", d.name)
	default:
		return fmt.Errorf("failed to expand device %v: unknown frontend %v", d.name, d.frontend)
	}

	return nil
}
