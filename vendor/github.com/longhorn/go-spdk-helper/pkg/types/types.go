package types

import (
	"fmt"
	"strings"
	"time"
)

const (
	NQNPrefix = "nqn.2023-01.io.longhorn.spdk"

	DefaultJSONServerNetwork    = "unix"
	DefaultUnixDomainSocketPath = "/var/tmp/spdk.sock"

	LocalIP = "127.0.0.1"

	MiB = 1 << 20

	FrontendSPDKTCPNvmf     = "spdk-tcp-nvmf"
	FrontendSPDKTCPBlockdev = "spdk-tcp-blockdev"

	ShallowCopyStateInProgress = "in progress"
	ShallowCopyStateComplete   = "complete"
	ShallowCopyStateError      = "error"

	ExecuteTimeout = 60 * time.Second
)

const (
	ErrorMessageCannotFindValidNvmeDevice = "cannot find a valid NVMe device"
	ErrorMessageDeviceOrResourceBusy      = "device or resource busy"
)

const (
	// Sequence of Events:
	// 1. Issuing I/O Command: The system sends a read command to the NVMe SSD.
	// 2. Waiting for ACK: The system waits for an acknowledgment from the NVMe SSD.
	// 3. Timeout: If no ACK is received within 2^transport_ack_timeout milliseconds,
	//    the system considers the I/O operation as a failure and attempts to resend it.
	// 4. Fast I/O Failure: If multiple retries fail consecutively and the total elapsed
	//    time exceeds fast_io_fail_timeout_sec seconds, the system determines that the I/O
	//    operation is stuck and takes further actions, such as raising an alarm or switching
	//    to a backup path.
	// 5. Controller Loss: If the NVMe SSD does not respond at all for more than
	//    ctrlr_loss_timeout_sec seconds, the system considers the controller as lost.
	// 6. Reconnect Attempt: The system attempts to reconnect to the NVMe SSD every
	//    2Reconnect_Delay_Sec seconds.

	DefaultCtrlrLossTimeoutSec = 15
	// DefaultReconnectDelaySec can't be more than DefaultFastIOFailTimeoutSec.
	DefaultReconnectDelaySec    = 2
	DefaultFastIOFailTimeoutSec = 10

	// DefaultTransportAckTimeout value is not the timeout second.
	// The timeout formula is 2^(transport_ack_timeout) msec.
	// DefaultTransportAckTimeout is set to 10, so the default timeout is 2^10 = 1024 msec = 1.024 sec.
	// By default, error detection on a qpair is very slow for TCP transports. For fast error
	// detection, transport_ack_timeout should be set.
	DefaultTransportAckTimeout = 10

	DefaultKeepAliveTimeoutMs = 10000
	DefaultMultipath          = "disable"
)

func GetNQN(name string) string {
	return fmt.Sprintf("%s:%s", NQNPrefix, name)
}

type DiskStatus struct {
	Bdf          string
	Type         string
	Driver       string
	Vendor       string
	Numa         string
	Device       string
	BlockDevices string
}

func ErrorIsDeviceOrResourceBusy(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), ErrorMessageDeviceOrResourceBusy)
}

func ErrorIsValidNvmeDeviceNotFound(err error) bool {
	return strings.Contains(err.Error(), ErrorMessageCannotFindValidNvmeDevice)
}
