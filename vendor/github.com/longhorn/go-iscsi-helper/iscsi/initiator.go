package iscsi

import (
	"bufio"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	lhns "github.com/longhorn/go-common-libs/ns"
	lhtypes "github.com/longhorn/go-common-libs/types"
)

var (
	DeviceWaitRetryCounts   = 10
	DeviceWaitRetryInterval = 1 * time.Second

	ScsiNodesDirs = []string{
		"/etc/iscsi/nodes/",
		"/var/lib/iscsi/nodes/",
	}
)

const (
	iscsiBinary    = "iscsiadm"
	scanModeManual = "manual"
	scanModeAuto   = "auto"
	ScanTimeout    = 10 * time.Second
)

func CheckForInitiatorExistence(nsexec *lhns.Executor) error {
	opts := []string{
		"--version",
	}
	_, err := nsexec.Execute(nil, iscsiBinary, opts, lhtypes.ExecuteDefaultTimeout)
	return err
}

func UpdateScsiDeviceTimeout(devName string, timeout int64, nsexec *lhns.Executor) error {
	deviceTimeoutFile := filepath.Join("/sys/block", devName, "device", "timeout")
	return lhns.WriteFile(deviceTimeoutFile, fmt.Sprint(timeout))
}

func UpdateIscsiDeviceAbortTimeout(target string, timeout int64, nsexec *lhns.Executor) error {
	opts := []string{
		"-m", "node",
		"-T", target,
		"-o", "update",
		"-n", "node.session.err_timeo.abort_timeout",
		"-v", strconv.FormatInt(timeout, 10),
	}
	_, err := nsexec.Execute(nil, iscsiBinary, opts, lhtypes.ExecuteDefaultTimeout)
	return err
}

func DiscoverTarget(ip, target string, nsexec *lhns.Executor) error {
	opts := []string{
		"-m", "discovery",
		"-t", "sendtargets",
		"-p", ip,
	}
	output, err := nsexec.Execute(nil, iscsiBinary, opts, lhtypes.ExecuteDefaultTimeout)
	if err != nil {
		return err
	}
	// Sometime iscsiadm won't return error but showing e.g.:
	//  iscsiadm: Could not stat /etc/iscsi/nodes//,3260,-1/default to
	//  delete node: No such file or directory\n\niscsiadm: Could not
	//  add/update [tcp:[hw=,ip=,net_if=,iscsi_if=default] 172.18.0.5,3260,1
	//  iqn.2019-10.io.longhorn:vol9]\n172.18.0.5:3260,1
	//  iqn.2019-10.io.longhorn:vol9\n"
	if strings.Contains(output, "Could not") {
		return fmt.Errorf("cannot discover target: %s", output)
	}
	if !strings.Contains(output, target) {
		return fmt.Errorf("cannot find target %s in discovered targets %s", target, output)
	}
	return nil
}

func DeleteDiscoveredTarget(ip, target string, nsexec *lhns.Executor) error {
	opts := []string{
		"-m", "node",
		"-o", "delete",
		"-T", target,
	}
	if ip != "" {
		opts = append(opts, "-p", ip)
	}
	_, err := nsexec.Execute(nil, iscsiBinary, opts, lhtypes.ExecuteDefaultTimeout)
	return err
}

func IsTargetDiscovered(ip, target string, nsexec *lhns.Executor) bool {
	opts := []string{
		"-m", "node",
		"-T", target,
	}
	if ip != "" {
		opts = append(opts, "-p", ip)
	}
	_, err := nsexec.Execute(nil, iscsiBinary, opts, lhtypes.ExecuteDefaultTimeout)
	return err == nil
}

func LoginTarget(ip, target string, nsexec *lhns.Executor) error {
	opts := []string{
		"-m", "node",
		"-T", target,
		"-p", ip,
		"--login",
	}
	_, err := nsexec.Execute(nil, iscsiBinary, opts, lhtypes.ExecuteDefaultTimeout)
	if err != nil {
		return err
	}

	scanMode, err := getIscsiNodeSessionScanMode(ip, target, nsexec)
	if err != nil {
		return errors.Wrap(err, "Failed to get node.session.scan mode")
	}

	if scanMode == scanModeManual {
		logrus.Infof("Manually rescan LUNs of the target %v:%v", target, ip)
		if err := manualScanSession(ip, target, nsexec); err != nil {
			return errors.Wrapf(err, "failed to manually rescan iscsi session of target %v:%v", target, ip)
		}
	} else {
		logrus.Infof("default: automatically rescan all LUNs of all iscsi sessions")
	}

	return nil
}

// LogoutTarget will logout all sessions if ip == ""
func LogoutTarget(ip, target string, nsexec *lhns.Executor) error {
	opts := []string{
		"-m", "node",
		"-T", target,
		"--logout",
	}
	if ip != "" {
		opts = append(opts, "-p", ip)
	}
	_, err := nsexec.Execute(nil, iscsiBinary, opts, lhtypes.ExecuteDefaultTimeout)
	return err
}

func GetDevice(ip, target string, lun int, nsexec *lhns.Executor) (*lhtypes.BlockDeviceInfo, error) {
	var err error

	var dev *lhtypes.BlockDeviceInfo
	for i := 0; i < DeviceWaitRetryCounts; i++ {
		dev, err = findScsiDevice(ip, target, lun, nsexec)
		if err == nil {
			break
		}
		time.Sleep(DeviceWaitRetryInterval)
	}
	if err != nil {
		return nil, err
	}
	return dev, nil
}

// IsTargetLoggedIn check all portals if ip == ""
func IsTargetLoggedIn(ip, target string, nsexec *lhns.Executor) bool {
	opts := []string{
		"-m", "session",
	}

	output, err := nsexec.Execute(nil, iscsiBinary, opts, lhtypes.ExecuteDefaultTimeout)
	if err != nil {
		return false
	}
	/* It will looks like:
		tcp: [463] 172.17.0.2:3260,1 iqn.2019-10.io.longhorn:test-volume
	or:
		tcp: [463] 172.17.0.2:3260,1 iqn.2019-10.io.longhorn:test-volume (non-flash)
	*/
	found := false
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, ip+":") {
			if strings.HasSuffix(line, " "+target) ||
				strings.Contains(scanner.Text(), " "+target+" ") {
				found = true
				break
			}
		}
	}

	return found
}

func manualScanSession(ip, target string, nsexec *lhns.Executor) error {
	opts := []string{
		"-m", "node",
		"-T", target,
		"-p", ip,
		"--rescan",
	}
	_, err := nsexec.Execute(nil, iscsiBinary, opts, ScanTimeout)
	return err
}

func getIscsiNodeSessionScanMode(ip, target string, nsexec *lhns.Executor) (string, error) {
	opts := []string{
		"-m", "node",
		"-T", target,
		"-p", ip,
		"-o", "show",
	}
	output, err := nsexec.Execute(nil, iscsiBinary, opts, ScanTimeout)
	if err != nil {
		return "", err
	}
	if strings.Contains(output, "node.session.scan = manual") {
		return scanModeManual, nil
	}
	return scanModeAuto, nil
}

func findScsiDevice(ip, target string, lun int, nsexec *lhns.Executor) (*lhtypes.BlockDeviceInfo, error) {
	name := ""

	opts := []string{
		"-m", "session",
		"-P", "3",
	}
	output, err := nsexec.Execute(nil, iscsiBinary, opts, lhtypes.ExecuteDefaultTimeout)
	if err != nil {
		return nil, err
	}
	/*
		Now we got something like this in output, and need to parse it
		Target: iqn.2019-10.io.longhorn:for.all (non-flash)
			Current Portal: 172.17.0.2:3260,1
			Persistent Portal: 172.17.0.2:3260,1
			...
			Attached SCSI devices:
			...
			scsi12 Channel 00 Id 0 Lun: 0
			scsi12 Channel 00 Id 0 Lun: 1
				Attached scsi disk sdb		State: running
		...
		Target: ...
	*/
	scanner := bufio.NewScanner(strings.NewReader(output))
	targetLine := "Target: " + target
	ipLine := " " + ip + ":"
	lunLine := "Lun: " + strconv.Itoa(lun)
	diskPrefix := "Attached scsi disk"
	stateLine := "State:"

	inTarget := false
	inIP := false
	inLun := false
	for scanner.Scan() {
		/* Target line can be:
			Target: iqn.2019-10.io.longhorn:for.all (non-flash)
		or:
			Target: iqn.2019-10.io.longhorn:for.all
		*/
		if !inTarget &&
			(strings.Contains(scanner.Text(), targetLine+" ") ||
				strings.HasSuffix(scanner.Text(), targetLine)) {
			inTarget = true
			continue
		}
		if inTarget && strings.Contains(scanner.Text(), ipLine) {
			inIP = true
			continue
		}
		if inIP && strings.Contains(scanner.Text(), lunLine) {
			inLun = true
			continue
		}
		// The line we need
		if inLun {
			line := scanner.Text()
			if !strings.Contains(line, diskPrefix) {
				return nil, fmt.Errorf("invalid output format, cannot find disk in: %s\n %s", line, output)
			}
			line = strings.TrimSpace(strings.Split(line, stateLine)[0])
			line = strings.TrimPrefix(line, diskPrefix)
			name = strings.TrimSpace(line)
			break
		}
	}

	if name == "" {
		return nil, fmt.Errorf("cannot find iSCSI device")
	}

	// TODO: replace with namespace joiner
	// now that we know the device is mapped, we can get it's (major:minor)
	devices, err := lhns.GetSystemBlockDevices()
	if err != nil {
		return nil, err
	}

	dev, known := devices[name]
	if !known {
		return nil, fmt.Errorf("cannot find kernel device for iSCSI device: %s", name)
	}

	return &dev, nil
}

func CleanupScsiNodes(target string) error {
	for _, dir := range ScsiNodesDirs {
		if _, err := lhns.GetFileInfo(dir); err != nil {
			continue
		}

		targetDir := filepath.Join(dir, target)
		if _, err := lhns.GetFileInfo(targetDir); err != nil {
			continue
		}

		// Remove all empty files in the directory
		emptyFilePaths, err := lhns.GetEmptyFiles(targetDir)
		if err != nil {
			return err
		}

		for _, emptyFilePath := range emptyFilePaths {
			err := lhns.DeletePath(emptyFilePath)
			if err != nil {
				return errors.Wrapf(err, "failed to clean up empty iSCSI node file %v", emptyFilePath)
			}
		}

		// Try to remove the upper level directory containing empty files.
		// We don't mind if it fails.
		dirContainEmptyFiles := make(map[string]bool)
		for _, emptyFilePath := range emptyFilePaths {
			dirContainEmptyFiles[filepath.Dir(emptyFilePath)] = true
		}
		for dir := range dirContainEmptyFiles {
			err := lhns.DeleteDirectory(dir)
			if err != nil {
				logrus.WithError(err).Warnf("Failed to clean up iSCSI node directory %v", dir)
			}
		}
	}
	return nil
}

func RescanTarget(ip, target string, nsexec *lhns.Executor) error {
	opts := []string{
		"-m", "node",
		"-T", target,
		"-R",
	}
	if ip != "" {
		opts = append(opts, "-p", ip)
	}
	_, err := nsexec.Execute(nil, iscsiBinary, opts, lhtypes.ExecuteDefaultTimeout)
	return err
}
