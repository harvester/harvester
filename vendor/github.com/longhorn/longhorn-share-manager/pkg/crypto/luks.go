package crypto

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	iscsi_util "github.com/longhorn/go-iscsi-helper/util"
)

const hostProcPath = "/host/proc"
const luksTimeout = time.Minute

func luksOpen(volume, devicePath, passphrase string) (stdout string, err error) {
	return cryptSetupWithPassphrase(passphrase,
		"luksOpen", devicePath, volume, "-d", "/dev/stdin")
}

func luksClose(volume string) (stdout string, err error) {
	return cryptSetup("luksClose", volume)
}

func luksFormat(devicePath, passphrase, keyCipher, keyHash, keySize, pbkdf string) (stdout string, err error) {
	return cryptSetupWithPassphrase(passphrase,
		"-q", "luksFormat", "--type", "luks2", "--cipher", keyCipher, "--hash", keyHash, "--key-size", keySize, "--pbkdf", pbkdf,
		devicePath, "-d", "/dev/stdin")
}

func luksStatus(volume string) (stdout string, err error) {
	return cryptSetup("status", volume)
}

func cryptSetup(args ...string) (stdout string, err error) {
	return cryptSetupWithPassphrase("", args...)
}

// cryptSetupWithPassphrase runs cryptsetup via nsenter inside of the host namespaces
// cryptsetup returns 0 on success and a non-zero value on error.
// 1 wrong parameters, 2 no permission (bad passphrase),
// 3 out of memory, 4 wrong device specified,
// 5 device already exists or device is busy.
func cryptSetupWithPassphrase(passphrase string, args ...string) (stdout string, err error) {
	// NOTE: cryptsetup needs to be run in the host IPC/MNT
	// if you only use MNT the binary will not return but still do the appropriate action.
	ns := iscsi_util.GetHostNamespacePath(hostProcPath)
	// ns := fmt.Sprintf("%s/%d/ns/", hostProcPath, 1)
	nsArgs := prepareCommandArgs(ns, "cryptsetup", args)
	ctx, cancel := context.WithTimeout(context.TODO(), luksTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "nsenter", nsArgs...)

	var stdoutBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	if len(passphrase) > 0 {
		cmd.Stdin = strings.NewReader(passphrase)
	}

	output := string(stdoutBuf.Bytes())
	if err := cmd.Run(); err != nil {
		return output, fmt.Errorf("failed to run cryptsetup args: %v output: %v error: %v", args, output, err)
	}

	return stdoutBuf.String(), nil
}

func prepareCommandArgs(ns, cmd string, args []string) []string {
	cmdArgs := []string{
		"--mount=" + filepath.Join(ns, "mnt"),
		"--ipc=" + filepath.Join(ns, "ipc"),
		cmd,
	}
	return append(cmdArgs, args...)
}
