package utils

import (
	"strings"
	"time"
)

const (
	GRPCServiceTimeout = 3 * time.Minute
)

func GetGRPCAddress(address string) string {
	address = strings.TrimPrefix(address, "tcp://")
	address = strings.TrimPrefix(address, "http://")

	return address
}
