package types

import "fmt"

const (
	DefaultJSONServerNetwork    = "unix"
	DefaultUnixDomainSocketPath = "/var/tmp/spdk.sock"

	LocalIP = "127.0.0.1"

	MiB = 1 << 20

	FrontendSPDKTCPNvmf     = "spdk-tcp-nvmf"
	FrontendSPDKTCPBlockdev = "spdk-tcp-blockdev"

	DefaultCtrlrLossTimeoutSec = 30
	// DefaultReconnectDelaySec can't be more than DefaultFastIoFailTimeoutSec, same for non-default values.
	DefaultReconnectDelaySec    = 5
	DefaultFastIOFailTimeoutSec = 15
	// By default, error detection on a qpair is very slow for TCP transports. For fast error
	// detection, transport_ack_timeout should be set.
	// Ack timeout fomula is 2^(transport_ack_timeout) msec.
	// DefaultTransportAckTimeout is 14, so the default timeout is 2^14 = 16384 msec = 16.384 sec.
	DefaultTransportAckTimeout = 14
)

func GetNQN(name string) string {
	return fmt.Sprintf("nqn.2023-01.io.longhorn.spdk:%s", name)
}
