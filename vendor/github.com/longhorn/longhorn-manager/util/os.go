package util

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	iscsiutil "github.com/longhorn/go-iscsi-helper/util"
)

const (
	OsReleasePath = "/etc/os-release"
)

// GetHostKernelRelease retrieves the kernel release version of the host.
func GetHostKernelRelease() (string, error) {
	initiatorNSPath := iscsiutil.GetHostNamespacePath(HostProcPath)
	mountPath := fmt.Sprintf("--mount=%s/mnt", initiatorNSPath)
	output, err := Execute([]string{}, "nsenter", mountPath, "uname", "-r")
	if err != nil {
		return "", err
	}
	return RemoveNewlines(output), nil
}

// GetHostOSDistro retrieves the operating system distribution of the host.
func GetHostOSDistro() (string, error) {
	initiatorNSPath := iscsiutil.GetHostNamespacePath(HostProcPath)
	mountPath := fmt.Sprintf("--mount=%s/mnt", initiatorNSPath)
	output, err := Execute([]string{}, "nsenter", mountPath, "cat", OsReleasePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read %v on host", OsReleasePath)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			osDistro := RemoveNewlines(strings.TrimPrefix(line, "ID="))
			return strings.ReplaceAll(osDistro, `"`, ""), nil
		}
	}
	return "", fmt.Errorf("failed to find ID field in %v", OsReleasePath)
}
