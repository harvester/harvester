package util

import (
	"fmt"

	lhproc "github.com/longhorn/go-common-libs/proc"
)

const ISCSIdProcess = "iscsid"

func GetISCSIdNamespaceDirectory(procDir string) (string, error) {
	pids, err := lhproc.GetProcessPIDs(ISCSIdProcess, procDir)
	if err != nil {
		return "", err
	}

	return lhproc.GetNamespaceDirectory(procDir, fmt.Sprint(pids[0])), nil
}
