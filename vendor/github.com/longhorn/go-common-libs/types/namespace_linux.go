package types

import (
	"golang.org/x/sys/unix"
)

func (ns Namespace) Flag() uintptr {
	switch ns {
	case NamespaceNet:
		return unix.CLONE_NEWNET
	default:
		return 0
	}
}
