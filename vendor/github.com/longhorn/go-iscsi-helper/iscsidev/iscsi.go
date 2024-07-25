package iscsidev

import (
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/longhorn/go-iscsi-helper/iscsi"
	"github.com/longhorn/go-iscsi-helper/types"
	"github.com/longhorn/go-iscsi-helper/util"

	lhns "github.com/longhorn/go-common-libs/ns"
	lhtypes "github.com/longhorn/go-common-libs/types"
)

var (
	LockFile    = "/var/run/longhorn-iscsi.lock"
	LockTimeout = 120 * time.Second

	TargetLunID = 1

	RetryCounts           = 5
	RetryIntervalSCSI     = 3 * time.Second
	RetryIntervalTargetID = 500 * time.Millisecond
)

type ScsiDeviceParameters struct {
	ScsiTimeout int64
}

type IscsiDeviceParameters struct {
	IscsiAbortTimeout int64
}

type Device struct {
	Target       string
	KernelDevice *lhtypes.BlockDeviceInfo

	ScsiDeviceParameters
	IscsiDeviceParameters

	BackingFile string
	BSType      string
	BSOpts      string

	targetID int

	nsexec *lhns.Executor
}

func NewDevice(name, backingFile, bsType, bsOpts string, scsiTimeout, iscsiAbortTimeout int64) (*Device, error) {
	namespaces := []lhtypes.Namespace{lhtypes.NamespaceMnt, lhtypes.NamespaceNet}
	nsexec, err := lhns.NewNamespaceExecutor(util.ISCSIdProcess, lhtypes.HostProcDirectory, namespaces)
	if err != nil {
		return nil, err
	}

	dev := &Device{
		Target: GetTargetName(name),
		ScsiDeviceParameters: ScsiDeviceParameters{
			ScsiTimeout: scsiTimeout,
		},
		IscsiDeviceParameters: IscsiDeviceParameters{
			IscsiAbortTimeout: iscsiAbortTimeout,
		},
		BackingFile: backingFile,
		BSType:      bsType,
		BSOpts:      bsOpts,
		nsexec:      nsexec,
	}
	return dev, nil
}

func Volume2ISCSIName(name string) string {
	return strings.Replace(name, "_", ":", -1)
}

func GetTargetName(volumeName string) string {
	return "iqn.2019-10.io.longhorn:" + Volume2ISCSIName(volumeName)
}

func (dev *Device) ReloadTargetID() error {
	tid, err := iscsi.GetTargetTid(dev.Target)
	if err != nil {
		return err
	}
	dev.targetID = tid
	return nil
}

func (dev *Device) CreateTarget() (err error) {
	// Start tgtd daemon if it's not already running
	if err := iscsi.StartDaemon(false); err != nil {
		return err
	}

	tid := 0
	for i := 0; i < RetryCounts; i++ {
		if tid, err = iscsi.FindNextAvailableTargetID(); err != nil {
			return err
		}
		logrus.Infof("go-iscsi-helper: found available target id %v", tid)
		err = iscsi.CreateTarget(tid, dev.Target)
		if err == nil {
			dev.targetID = tid
			break
		}
		logrus.Infof("go-iscsi-helper: failed to use target id %v, retrying with a new target ID: err %v", tid, err)
		time.Sleep(RetryIntervalTargetID)
		continue
	}
	if err != nil {
		return err
	}

	if err := iscsi.AddLun(dev.targetID, TargetLunID, dev.BackingFile, dev.BSType, dev.BSOpts); err != nil {
		return err
	}
	// Cannot modify the parameters for the LUNs during the adding stage
	if err := iscsi.SetLunThinProvisioning(dev.targetID, TargetLunID); err != nil {
		return err
	}
	// Longhorn reads and writes data with direct io rather than buffer io, so
	// the write cache is actually disabled in the implementation.
	// Explicitly disable the write cache for meeting the SCSI specification.
	if err := iscsi.DisableWriteCache(dev.targetID, TargetLunID); err != nil {
		return err
	}
	if err := iscsi.BindInitiator(dev.targetID, "ALL"); err != nil {
		return err
	}
	return nil
}

func (dev *Device) StartInitator() error {
	lock := lhns.NewLock(LockFile, LockTimeout)
	if err := lock.Lock(); err != nil {
		return errors.Wrap(err, "failed to lock")
	}
	defer lock.Unlock()

	if err := iscsi.CheckForInitiatorExistence(dev.nsexec); err != nil {
		return err
	}

	localIP, err := util.GetIPToHost()
	if err != nil {
		return err
	}

	// Setup initiator
	for i := 0; i < RetryCounts; i++ {
		err := iscsi.DiscoverTarget(localIP, dev.Target, dev.nsexec)
		if iscsi.IsTargetDiscovered(localIP, dev.Target, dev.nsexec) {
			break
		}

		logrus.WithError(err).Warnf("Failed to discover")
		// This is a trick to recover from the case. Remove the
		// empty entries in /etc/iscsi/nodes/<target_name>. If one of the entry
		// is empty it will triggered the issue.
		if err := iscsi.CleanupScsiNodes(dev.Target); err != nil {
			logrus.WithError(err).Warnf("Failed to clean up nodes for %v", dev.Target)
		} else {
			logrus.Warnf("Nodes cleaned up for %v", dev.Target)
		}

		time.Sleep(RetryIntervalSCSI)
	}
	if err := iscsi.UpdateIscsiDeviceAbortTimeout(dev.Target, dev.IscsiAbortTimeout, dev.nsexec); err != nil {
		return err
	}
	if err := iscsi.LoginTarget(localIP, dev.Target, dev.nsexec); err != nil {
		return err
	}
	if dev.KernelDevice, err = iscsi.GetDevice(localIP, dev.Target, TargetLunID, dev.nsexec); err != nil {
		return err
	}
	if err := iscsi.UpdateScsiDeviceTimeout(dev.KernelDevice.Name, dev.ScsiTimeout, dev.nsexec); err != nil {
		return err
	}

	return nil
}

// ReloadInitiator does nothing for the iSCSI initiator/target except for
// updating the timeout. It is mainly responsible for initializing the struct
// field `dev.KernelDevice`.
func (dev *Device) ReloadInitiator() error {
	lock := lhns.NewLock(LockFile, LockTimeout)
	if err := lock.Lock(); err != nil {
		return errors.Wrap(err, "failed to lock")
	}
	defer lock.Unlock()

	if err := iscsi.CheckForInitiatorExistence(dev.nsexec); err != nil {
		return err
	}

	localIP, err := util.GetIPToHost()
	if err != nil {
		return err
	}

	if err := iscsi.DiscoverTarget(localIP, dev.Target, dev.nsexec); err != nil {
		return err
	}

	if !iscsi.IsTargetDiscovered(localIP, dev.Target, dev.nsexec) {
		return fmt.Errorf("failed to discover target %v for the initiator", dev.Target)
	}

	if err := iscsi.UpdateIscsiDeviceAbortTimeout(dev.Target, dev.IscsiAbortTimeout, dev.nsexec); err != nil {
		return err
	}
	if dev.KernelDevice, err = iscsi.GetDevice(localIP, dev.Target, TargetLunID, dev.nsexec); err != nil {
		return err
	}

	return iscsi.UpdateScsiDeviceTimeout(dev.KernelDevice.Name, dev.ScsiTimeout, dev.nsexec)
}

func (dev *Device) StopInitiator() error {
	lock := lhns.NewLock(LockFile, LockTimeout)
	if err := lock.Lock(); err != nil {
		return errors.Wrap(err, "failed to lock")
	}
	defer lock.Unlock()

	if err := LogoutTarget(dev.Target, dev.nsexec); err != nil {
		return errors.Wrapf(err, "failed to logout target")
	}
	return nil
}

func (dev *Device) RefreshInitiator() error {
	lock := lhns.NewLock(LockFile, LockTimeout)
	if err := lock.Lock(); err != nil {
		return errors.Wrap(err, "failed to lock")
	}
	defer lock.Unlock()

	if err := iscsi.CheckForInitiatorExistence(dev.nsexec); err != nil {
		return err
	}

	ip, err := util.GetIPToHost()
	if err != nil {
		return err
	}

	return iscsi.RescanTarget(ip, dev.Target, dev.nsexec)
}

func LogoutTarget(target string, nsexec *lhns.Executor) error {
	if err := iscsi.CheckForInitiatorExistence(nsexec); err != nil {
		return err
	}
	if iscsi.IsTargetLoggedIn("", target, nsexec) {
		var err error
		loggingOut := false

		logrus.Infof("Shutting down iSCSI device for target %v", target)
		for i := 0; i < RetryCounts; i++ {
			// New IP may be different from the IP in the previous record.
			// https://github.com/longhorn/longhorn/issues/1920
			err = iscsi.LogoutTarget("", target, nsexec)
			// Ignore Not Found error
			if err == nil || strings.Contains(err.Error(), "exit status 21") {
				err = nil
				break
			}
			// The timeout for response may return in the future,
			// check session to know if it's logged out or not
			if strings.Contains(err.Error(), "Timeout executing: ") {
				loggingOut = true
				break
			}
			time.Sleep(RetryIntervalSCSI)
		}
		// Wait for device to logout
		if loggingOut {
			logrus.Infof("Logging out iSCSI device timeout, waiting for logout complete")
			for i := 0; i < RetryCounts; i++ {
				if !iscsi.IsTargetLoggedIn("", target, nsexec) {
					err = nil
					break
				}
				time.Sleep(RetryIntervalSCSI)
			}
		}
		if err != nil {
			return errors.Wrapf(err, "failed to logout target")
		}
		/*
		 * Immediately delete target after logout may result in error:
		 *
		 * "Could not execute operation on all records: encountered
		 * iSCSI database failure" in iscsiadm
		 *
		 * This happens especially there are other iscsiadm db
		 * operations go on at the same time.
		 * Retry to workaround this issue. Also treat "exit status
		 * 21"(no record found) as valid result
		 */
		for i := 0; i < RetryCounts; i++ {
			if !iscsi.IsTargetDiscovered("", target, nsexec) {
				err = nil
				break
			}

			err = iscsi.DeleteDiscoveredTarget("", target, nsexec)
			// Ignore Not Found error
			if err == nil || strings.Contains(err.Error(), "exit status 21") {
				err = nil
				break
			}
			time.Sleep(RetryIntervalSCSI)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (dev *Device) DeleteTarget() error {
	if tid, err := iscsi.GetTargetTid(dev.Target); err == nil && tid != -1 {
		if tid != dev.targetID && dev.targetID != 0 {
			logrus.Errorf("BUG: Invalid TID %v found for %v, was %v", tid, dev.Target, dev.targetID)
		}

		logrus.Infof("Shutting down iSCSI target %v", dev.Target)

		// UnbindInitiator can return tgtadmSuccess, tgtadmAclNoexist or tgtadmNoTarget
		// Target is deleted in the last step, so tgtadmNoTarget error should not occur here.
		// Just ignore tgtadmAclNoexist and continue working on the remaining tasks.
		if err := iscsi.UnbindInitiator(tid, "ALL"); err != nil {
			if !strings.Contains(err.Error(), types.TgtadmAclNoexist) {
				return err
			}
			logrus.WithError(err).Warnf("failed to unbind initiator target id %v", tid)
		}

		sessionConnectionsMap, err := iscsi.GetTargetConnections(tid)
		if err != nil {
			return err
		}
		for sid, cidList := range sessionConnectionsMap {
			for _, cid := range cidList {
				if err := iscsi.CloseConnection(tid, sid, cid); err != nil {
					return err
				}
			}
		}

		if err := iscsi.DeleteLun(tid, TargetLunID); err != nil {
			return err
		}

		if err := iscsi.DeleteTarget(tid); err != nil {
			return err
		}
	}
	return nil
}

func (dev *Device) UpdateScsiBackingStore(bsType, bsOpts string) error {
	dev.BSType = bsType
	dev.BSOpts = bsOpts
	return nil
}

func (dev *Device) ExpandTarget(size int64) error {
	return iscsi.ExpandLun(dev.targetID, TargetLunID, size)
}
