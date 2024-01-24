package types

import (
	"fmt"
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

	DefaultCtrlrLossTimeoutSec = 30
	// DefaultReconnectDelaySec can't be more than DefaultFastIoFailTimeoutSec, same for non-default values.
	DefaultReconnectDelaySec    = 5
	DefaultFastIOFailTimeoutSec = 15
	// DefaultTransportAckTimeout value is not the timeout second.
	// The timeout formula is 2^(transport_ack_timeout) msec.
	// DefaultTransportAckTimeout is 14, so the default timeout is 2^14 = 16384 msec = 16.384 sec.
	// By default, error detection on a qpair is very slow for TCP transports. For fast error
	// detection, transport_ack_timeout should be set.
	DefaultTransportAckTimeout = 14

	ExecuteTimeout = 60 * time.Second
)

func GetNQN(name string) string {
	return fmt.Sprintf("%s:%s", NQNPrefix, name)
}
