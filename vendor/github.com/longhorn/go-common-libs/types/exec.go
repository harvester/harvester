package types

import (
	"time"
)

const (
	BinaryCryptsetup = "cryptsetup"
	BinaryFstrim     = "fstrim"
)

const (
	ExecuteNoTimeout      = time.Duration(-1)
	ExecuteDefaultTimeout = time.Minute
)
