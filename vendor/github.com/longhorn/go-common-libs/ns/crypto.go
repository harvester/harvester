package ns

import (
	"time"

	"github.com/longhorn/go-common-libs/types"
)

// LuksOpen runs cryptsetup luksOpen with the given passphrase and
// returns the stdout and error.
func (nsexec *Executor) LuksOpen(volume, devicePath, passphrase string, timeout time.Duration) (stdout string, err error) {
	args := []string{"luksOpen", devicePath, volume, "-d", "/dev/stdin"}
	return nsexec.CryptsetupWithPassphrase(passphrase, args, timeout)
}

// LuksClose runs cryptsetup luksClose and returns the stdout and error.
func (nsexec *Executor) LuksClose(volume string, timeout time.Duration) (stdout string, err error) {
	args := []string{"luksClose", volume}
	return nsexec.Cryptsetup(args, timeout)
}

// LuksFormat runs cryptsetup luksFormat with the given passphrase and
// returns the stdout and error.
func (nsexec *Executor) LuksFormat(devicePath, passphrase, keyCipher, keyHash, keySize, pbkdf string, timeout time.Duration) (stdout string, err error) {
	args := []string{
		"-q", "luksFormat",
		"--type", "luks2",
		"--cipher", keyCipher,
		"--hash", keyHash,
		"--key-size", keySize,
		"--pbkdf", pbkdf,
		devicePath, "-d", "/dev/stdin",
	}
	return nsexec.CryptsetupWithPassphrase(passphrase, args, timeout)
}

// LuksResize runs cryptsetup resize with the given passphrase and
// returns the stdout and error.
func (nsexec *Executor) LuksResize(volume, passphrase string, timeout time.Duration) (stdout string, err error) {
	args := []string{"resize", volume}
	return nsexec.CryptsetupWithPassphrase(passphrase, args, timeout)
}

// LuksStatus runs cryptsetup status and returns the stdout and error.
func (nsexec *Executor) LuksStatus(volume string, timeout time.Duration) (stdout string, err error) {
	args := []string{"status", volume}
	return nsexec.Cryptsetup(args, timeout)
}

// Cryptsetup runs cryptsetup without passphrase. It will return
// 0 on success and a non-zero value on error.
func (nsexec *Executor) Cryptsetup(args []string, timeout time.Duration) (stdout string, err error) {
	return nsexec.CryptsetupWithPassphrase("", args, timeout)
}

// CryptsetupWithPassphrase runs cryptsetup with passphrase. It will return
// 0 on success and a non-zero value on error.
// 1 wrong parameters, 2 no permission (bad passphrase),
// 3 out of memory, 4 wrong device specified,
// 5 device already exists or device is busy.
func (nsexec *Executor) CryptsetupWithPassphrase(passphrase string, args []string, timeout time.Duration) (stdout string, err error) {
	// NOTE: When using cryptsetup, ensure it is run in the host IPC/MNT namespace.
	// If only the MNT namespace is used, the binary will not return, but the
	// appropriate action will still be performed.
	// For Talos Linux, cryptsetup comes pre-installed in the host namespace
	// (ref: https://github.com/siderolabs/pkgs/blob/release-1.4/reproducibility/pkg.yaml#L10)
	// for the [Disk Encryption](https://www.talos.dev/v1.4/talos-guides/configuration/disk-encryption/).
	return nsexec.ExecuteWithStdin(types.BinaryCryptsetup, args, passphrase, timeout)
}
