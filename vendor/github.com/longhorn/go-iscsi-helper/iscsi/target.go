package iscsi

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	lhexec "github.com/longhorn/go-common-libs/exec"
	lhtypes "github.com/longhorn/go-common-libs/types"
)

var (
	TgtdRetryCounts   = 5
	TgtdRetryInterval = 1 * time.Second
)

const (
	tgtBinary = "tgtadm"

	maxTargetID = 4095

	logFile = "/var/log/tgtd.log"
)

// CreateTarget will create a iSCSI target using the name specified. If name is
// unspecified, a name will be generated. Notice the name must comply with iSCSI
// name format.
func CreateTarget(tid int, name string) error {
	opts := []string{
		"--lld", "iscsi",
		"--op", "new",
		"--mode", "target",
		"--tid", strconv.Itoa(tid),
		"-T", name,
	}
	_, err := lhexec.NewExecutor().Execute(nil, tgtBinary, opts, lhtypes.ExecuteDefaultTimeout)
	return err
}

// DeleteTarget will remove a iSCSI target specified by tid
func DeleteTarget(tid int) error {
	opts := []string{
		"--lld", "iscsi",
		"--op", "delete",
		"--mode", "target",
		"--tid", strconv.Itoa(tid),
	}
	_, err := lhexec.NewExecutor().Execute(nil, tgtBinary, opts, lhtypes.ExecuteDefaultTimeout)
	return err
}

// AddLunBackedByFile will add a LUN in an existing target, which backing by
// specified file.
func AddLunBackedByFile(tid int, lun int, backingFile string) error {
	opts := []string{
		"--lld", "iscsi",
		"--op", "new",
		"--mode", "logicalunit",
		"--tid", strconv.Itoa(tid),
		"--lun", strconv.Itoa(lun),
		"-b", backingFile,
	}
	_, err := lhexec.NewExecutor().Execute(nil, tgtBinary, opts, lhtypes.ExecuteDefaultTimeout)
	return err
}

// AddLun will add a LUN in an existing target, which backing by
// specified file, using AIO backing-store
func AddLun(tid int, lun int, backingFile string, bstype string, bsopts string) error {
	if !CheckTargetForBackingStore(bstype) {
		return fmt.Errorf("backing-store %s is not supported", bstype)
	}
	opts := []string{
		"--lld", "iscsi",
		"--op", "new",
		"--mode", "logicalunit",
		"--tid", strconv.Itoa(tid),
		"--lun", strconv.Itoa(lun),
		"-b", backingFile,
		"--bstype", bstype,
	}
	if bsopts != "" {
		opts = append(opts, "--bsopts", bsopts)
	}
	_, err := lhexec.NewExecutor().Execute(nil, tgtBinary, opts, lhtypes.ExecuteDefaultTimeout)
	return err
}

// UpdateLun will update parameters for the LUN
func UpdateLun(tid int, lun int, params map[string]string) error {
	opts := []string{
		"--lld", "iscsi",
		"--op", "update",
		"--mode", "logicalunit",
		"--tid", strconv.Itoa(tid),
		"--lun", strconv.Itoa(lun),
	}
	if len(params) != 0 {
		paramStr := ""
		for k, v := range params {
			paramStr += fmt.Sprintf("%s=%s,", k, v)
		}
		opts = append(opts, "--params", strings.TrimSuffix(paramStr, ","))
	}
	_, err := lhexec.NewExecutor().Execute(nil, tgtBinary, opts, lhtypes.ExecuteDefaultTimeout)
	return err
}

// SetLunThinProvisioning will set param thin_provisioning to true for the LUN
func SetLunThinProvisioning(tid int, lun int) error {
	return UpdateLun(tid, lun, map[string]string{"thin_provisioning": "1"})
}

// DisableWriteCache will set param write-cache to false for the LUN
func DisableWriteCache(tid int, lun int) error {
	// Mode page 8 is the caching mode page
	// Refer to "Caching Mode page (08h)" in SCSI Commands Reference Manual for more information.
	// https://www.seagate.com/files/staticfiles/support/docs/manual/Interface%20manuals/100293068j.pdf
	// https://github.com/fujita/tgt/blob/master/scripts/tgt-admin#L418
	return UpdateLun(tid, lun, map[string]string{"mode_page": "8:0:18:0x10:0:0xff:0xff:0:0:0xff:0xff:0xff:0xff:0x80:0x14:0:0:0:0:0:0"})
}

// DeleteLun will remove a LUN from an target
func DeleteLun(tid int, lun int) error {
	opts := []string{
		"--lld", "iscsi",
		"--op", "delete",
		"--mode", "logicalunit",
		"--tid", strconv.Itoa(tid),
		"--lun", strconv.Itoa(lun),
	}
	_, err := lhexec.NewExecutor().Execute(nil, tgtBinary, opts, lhtypes.ExecuteDefaultTimeout)
	return err
}

// ExpandLun will update the size for the LUN.
// This is valid only for the customized tgt https://github.com/rancher/tgt/
func ExpandLun(tid, lun int, size int64) error {
	opts := []string{
		"--lld", "iscsi",
		"--op", "update",
		"--mode", "logicalunit",
		"--tid", strconv.Itoa(tid),
		"--lun", strconv.Itoa(lun),
		"--params", fmt.Sprintf("bsopts=size=%d", size),
	}
	_, err := lhexec.NewExecutor().Execute(nil, tgtBinary, opts, lhtypes.ExecuteDefaultTimeout)
	return err
}

// BindInitiator will add permission to allow certain initiator(s) to connect to
// certain target. "ALL" is a special initiator which is the wildcard
func BindInitiator(tid int, initiator string) error {
	opts := []string{
		"--lld", "iscsi",
		"--op", "bind",
		"--mode", "target",
		"--tid", strconv.Itoa(tid),
		"-I", initiator,
	}
	_, err := lhexec.NewExecutor().Execute(nil, tgtBinary, opts, lhtypes.ExecuteDefaultTimeout)
	return err
}

// UnbindInitiator will remove permission to allow certain initiator(s) to connect to
// certain target.
func UnbindInitiator(tid int, initiator string) error {
	opts := []string{
		"--lld", "iscsi",
		"--op", "unbind",
		"--mode", "target",
		"--tid", strconv.Itoa(tid),
		"-I", initiator,
	}
	_, err := lhexec.NewExecutor().Execute(nil, tgtBinary, opts, lhtypes.ExecuteDefaultTimeout)
	return err
}

// StartDaemon will start tgtd daemon, prepare for further commands
func StartDaemon(debug bool) error {
	if CheckTargetForBackingStore("rdwr") {
		fmt.Fprintf(os.Stderr, "go-iscsi-helper: tgtd is already running\n")
		return nil
	}

	logf, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	go startDaemon(logf, debug)

	// Wait until daemon is up
	daemonIsRunning := false
	for i := 0; i < TgtdRetryCounts; i++ {
		if CheckTargetForBackingStore("rdwr") {
			daemonIsRunning = true
			break
		}
		time.Sleep(TgtdRetryInterval)
	}
	if !daemonIsRunning {
		return fmt.Errorf("failed to start tgtd daemon")
	}
	return nil
}

func startDaemon(logf *os.File, debug bool) {
	defer logf.Close()

	opts := []string{
		"-f",
	}
	if debug {
		opts = append(opts, "-d", "1")
	}
	cmd := exec.Command("tgtd", opts...)
	mw := io.MultiWriter(os.Stderr, logf)
	cmd.Stdout = mw
	cmd.Stderr = mw
	if err := cmd.Run(); err != nil {
		if CheckTargetForBackingStore("rdwr") {
			fmt.Fprintf(mw, "go-iscsi-helper: tgtd is already running\n")
			return
		}
		fmt.Fprintf(mw, "go-iscsi-helper: command failed: %v\n", err)
		panic(err)
	}
	fmt.Fprintln(mw, "go-iscsi-helper: done")
}

func CheckTargetForBackingStore(name string) bool {
	opts := []string{
		"--lld", "iscsi",
		"--op", "show",
		"--mode", "system",
	}
	output, err := lhexec.NewExecutor().Execute(nil, tgtBinary, opts, lhtypes.ExecuteDefaultTimeout)
	if err != nil {
		return false
	}
	return strings.Contains(output, " "+name)
}

// GetTargetTid If returned TID is -1, then target doesn't exist, but we won't
// return error
func GetTargetTid(name string) (int, error) {
	opts := []string{
		"--lld", "iscsi",
		"--op", "show",
		"--mode", "target",
	}
	output, err := lhexec.NewExecutor().Execute(nil, tgtBinary, opts, lhtypes.ExecuteDefaultTimeout)
	if err != nil {
		return -1, err
	}
	/* Output will looks like:
	Target 1: iqn.2016-08.com.example:a
		System information:
		...
	Target 2: iqn.2016-08.com.example:b
		System information:
		...
	*/
	tid := -1
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		if strings.HasSuffix(scanner.Text(), " "+name) {
			tidString := strings.Fields(strings.Split(scanner.Text(), ":")[0])[1]
			tid, err = strconv.Atoi(tidString)
			if err != nil {
				return -1, errors.Wrapf(err, "BUG: Failed to parse %s", tidString)
			}
			break
		}
	}
	return tid, nil
}

func ShutdownTgtd() error {
	// Step 1: Show all targets
	showOpts := []string{"--op", "show", "--mode", "target"}
	output, err := lhexec.NewExecutor().Execute(nil, tgtBinary, showOpts, lhtypes.ExecuteDefaultTimeout)
	if err != nil {

		return fmt.Errorf("failed to show targets: %v", err)
	}

	// Step 2: Parse target IDs
	scanner := bufio.NewScanner(strings.NewReader(output))
	var targetIDs []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Target") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				targetIDs = append(targetIDs, fields[1])
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return errors.Wrapf(err, "failed to parse targets")
	}

	// Step 3: Delete each target
	for _, tid := range targetIDs {
		deleteOpts := []string{"--op", "delete", "--mode", "target", "--tid", tid}
		_, err := lhexec.NewExecutor().Execute(nil, tgtBinary, deleteOpts, lhtypes.ExecuteDefaultTimeout)
		if err != nil {
			return errors.Wrapf(err, "failed to delete target %s", tid)
		}
	}

	return nil
}

func GetTargetConnections(tid int) (map[string][]string, error) {
	opts := []string{
		"--lld", "iscsi",
		"--op", "show",
		"--mode", "conn",
		"--tid", strconv.Itoa(tid),
	}
	output, err := lhexec.NewExecutor().Execute(nil, tgtBinary, opts, lhtypes.ExecuteDefaultTimeout)
	if err != nil {
		return nil, err
	}
	/* Output will looks like:
	Session: 11
	    Connection: 0
	        Initiator: iqn.2016-08.com.example:a
	        IP Address: 192.168.0.1
	Session: 12
	    Connection: 1
	        Initiator: iqn.2016-08.com.example:a
	        IP Address: 192.168.0.2
		...
	*/
	res := map[string][]string{}
	currentSIDString := ""
	currentCIDStringList := []string{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Session: ") {
			if currentSIDString != "" {
				res[currentSIDString] = currentCIDStringList
			}
			sidFields := strings.Split(line, ": ")
			if len(sidFields) != 2 {
				return nil, fmt.Errorf("failed to parse and get session id from line %v", line)
			}
			sidString := sidFields[1]
			if _, err := strconv.Atoi(sidString); err != nil {
				return nil, err
			}
			currentSIDString = sidString
			currentCIDStringList = []string{}
		}
		if strings.HasPrefix(line, "Connection: ") {
			cidFields := strings.Split(line, ": ")
			if len(cidFields) != 2 {
				return nil, fmt.Errorf("failed to parse and get connection id from line %v", line)
			}
			cidString := cidFields[1]
			if _, err := strconv.Atoi(cidString); err != nil {
				return nil, err
			}
			currentCIDStringList = append(currentCIDStringList, cidString)
		}
	}
	if len(currentCIDStringList) != 0 {
		res[currentSIDString] = currentCIDStringList
	}
	return res, nil
}

func CloseConnection(tid int, sid, cid string) error {
	opts := []string{
		"--lld", "iscsi",
		"--op", "delete",
		"--mode", "conn",
		"--tid", strconv.Itoa(tid),
		"--sid", sid,
		"--cid", cid,
	}
	_, err := lhexec.NewExecutor().Execute(nil, tgtBinary, opts, lhtypes.ExecuteDefaultTimeout)
	return err
}

func FindNextAvailableTargetID() (int, error) {
	existingTids := map[int]struct{}{}
	opts := []string{
		"--lld", "iscsi",
		"--op", "show",
		"--mode", "target",
	}
	output, err := lhexec.NewExecutor().Execute(nil, tgtBinary, opts, lhtypes.ExecuteDefaultTimeout)
	if err != nil {
		return -1, err
	}
	/* Output will looks like:
	Target 1: iqn.2016-08.com.example:a
		System information:
		...
	Target 2: iqn.2016-08.com.example:b
		System information:
		...
	*/
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "Target ") {
			tidString := strings.Fields(strings.Split(scanner.Text(), ":")[0])[1]
			tid, err := strconv.Atoi(tidString)
			if err != nil {
				return -1, errors.Wrapf(err, "BUG: Failed to parse %s", tidString)
			}
			existingTids[tid] = struct{}{}
		}
	}
	for i := 1; i < maxTargetID; i++ {
		if _, exists := existingTids[i]; !exists {
			return i, nil
		}
	}
	return -1, fmt.Errorf("cannot find an available target ID")
}
