package common

import (
	"fmt"

	"github.com/prometheus/procfs"
)

const (
	DockerdProcess        = "dockerd"
	ContainerdProcess     = "containerd"
	ContainerdProcessShim = "containerd-shim"
)

func getPidProc(hostProcPath string, pid int) (*procfs.Proc, error) {
	fs, err := procfs.NewFS(hostProcPath)
	if err != nil {
		return nil, err
	}
	proc, err := fs.Proc(pid)
	if err != nil {
		return nil, err
	}
	return &proc, nil
}

func getSelfProc(hostProcPath string) (*procfs.Proc, error) {
	fs, err := procfs.NewFS(hostProcPath)
	if err != nil {
		return nil, err
	}
	proc, err := fs.Self()
	if err != nil {
		return nil, err
	}
	return &proc, nil
}

func findAncestorByName(hostProcPath string, ancestorProcess string) (*procfs.Proc, error) {
	proc, err := getSelfProc(hostProcPath)
	if err != nil {
		return nil, err
	}

	for {
		st, err := proc.Stat()
		if err != nil {
			return nil, err
		}
		if st.Comm == ancestorProcess {
			return proc, nil
		}
		if st.PPID == 0 {
			break
		}
		proc, err = getPidProc(hostProcPath, st.PPID)
		if err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("failed to find the ancestor process: %s", ancestorProcess)
}

func GetHostNamespacePath(hostProcPath string) string {
	containerNames := []string{DockerdProcess, ContainerdProcess, ContainerdProcessShim}
	for _, name := range containerNames {
		proc, err := findAncestorByName(hostProcPath, name)
		if err == nil {
			return fmt.Sprintf("%s/%d/ns/", hostProcPath, proc.PID)
		}
	}
	return fmt.Sprintf("%s/%d/ns/", hostProcPath, 1)
}
