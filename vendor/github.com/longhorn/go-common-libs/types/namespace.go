package types

import (
	"time"

	"golang.org/x/sys/unix"
)

const (
	HostProcDirectory = "/host/proc"
	ProcDirectory     = "/proc"
)

const NsBinary = "nsenter"

const (
	ErrNamespaceCastResultFmt = "failed casting result to %T: %v"
	ErrNamespaceFuncFmt       = "failed function: %v"
)

var NsJoinerDefaultTimeout = 24 * time.Hour

type Namespace string

const (
	NamespaceIpc = Namespace("ipc")
	NamespaceMnt = Namespace("mnt")
	NamespaceNet = Namespace("net")
)

func (ns Namespace) Flag() uintptr {
	switch ns {
	case NamespaceNet:
		return unix.CLONE_NEWNET
	default:
		return 0
	}
}

func (ns Namespace) String() string {
	return string(ns)
}
