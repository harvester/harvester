package util

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

const (
	NSBinary    = "nsenter"
	LSBLKBinary = "lsblk"
)

var (
	cmdTimeout = time.Minute // one minute by default
)

type KernelDevice struct {
	Name  string
	Major int
	Minor int
}

func getIPFromAddrs(addrs []net.Addr) string {
	for _, addr := range addrs {
		if ip, ok := addr.(*net.IPNet); ok && ip.IP.IsGlobalUnicast() {
			return strings.Split(ip.IP.String(), "/")[0]
		}
	}
	return ""
}

func GetIPToHost() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	// TODO: This is a workaround, we want to get the interface IP connect
	// to the host, it's likely eth1 with one network attached to the host.
	for _, iface := range ifaces {
		if iface.Name == "eth1" {
			addrs, err := iface.Addrs()
			if err != nil {
				return "", err
			}
			ip := getIPFromAddrs(addrs)
			if ip != "" {
				return ip, nil
			}
		}
	}
	// And there is no eth1, so get the first real ip
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	ip := getIPFromAddrs(addrs)
	if ip != "" {
		return ip, nil
	}
	return "", fmt.Errorf("Cannot find IP connect to the host")
}

type NamespaceExecutor struct {
	ns string
}

func NewNamespaceExecutor(ns string) (*NamespaceExecutor, error) {
	ne := &NamespaceExecutor{
		ns: ns,
	}

	if ns == "" {
		return ne, nil
	}
	mntNS := filepath.Join(ns, "mnt")
	netNS := filepath.Join(ns, "net")
	if _, err := Execute(NSBinary, []string{"-V"}); err != nil {
		return nil, fmt.Errorf("Cannot find nsenter for namespace switching")
	}
	if _, err := Execute(NSBinary, []string{"--mount=" + mntNS, "mount"}); err != nil {
		return nil, fmt.Errorf("Invalid mount namespace %v, error %v", mntNS, err)
	}
	if _, err := Execute(NSBinary, []string{"--net=" + netNS, "ip", "addr"}); err != nil {
		return nil, fmt.Errorf("Invalid net namespace %v, error %v", netNS, err)
	}
	return ne, nil
}

func (ne *NamespaceExecutor) prepareCommandArgs(name string, args []string) []string {
	cmdArgs := []string{
		"--mount=" + filepath.Join(ne.ns, "mnt"),
		"--net=" + filepath.Join(ne.ns, "net"),
		name,
	}
	return append(cmdArgs, args...)
}

func (ne *NamespaceExecutor) Execute(name string, args []string) (string, error) {
	if ne.ns == "" {
		return Execute(name, args)
	}
	return Execute(NSBinary, ne.prepareCommandArgs(name, args))
}

func (ne *NamespaceExecutor) ExecuteWithTimeout(timeout time.Duration, name string, args []string) (string, error) {
	if ne.ns == "" {
		return ExecuteWithTimeout(timeout, name, args)
	}
	return ExecuteWithTimeout(timeout, NSBinary, ne.prepareCommandArgs(name, args))
}

func (ne *NamespaceExecutor) ExecuteWithoutTimeout(name string, args []string) (string, error) {
	if ne.ns == "" {
		return ExecuteWithoutTimeout(name, args)
	}
	return ExecuteWithoutTimeout(NSBinary, ne.prepareCommandArgs(name, args))
}

func Execute(binary string, args []string) (string, error) {
	return ExecuteWithTimeout(cmdTimeout, binary, args)
}

func ExecuteWithTimeout(timeout time.Duration, binary string, args []string) (string, error) {
	var err error
	cmd := exec.Command(binary, args...)
	done := make(chan struct{})

	var output, stderr bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr

	go func() {
		err = cmd.Run()
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		if cmd.Process != nil {
			if err := cmd.Process.Kill(); err != nil {
				logrus.Warnf("Problem killing process pid=%v: %s", cmd.Process.Pid, err)
			}

		}
		return "", fmt.Errorf("Timeout executing: %v %v, output %s, stderr, %s, error %w",
			binary, args, output.String(), stderr.String(), err)
	}

	if err != nil {
		return "", fmt.Errorf("Failed to execute: %v %v, output %s, stderr, %s, error %w",
			binary, args, output.String(), stderr.String(), err)
	}
	return output.String(), nil
}

// TODO: Merge this with ExecuteWithTimeout

func ExecuteWithoutTimeout(binary string, args []string) (string, error) {
	var err error
	var output, stderr bytes.Buffer

	cmd := exec.Command(binary, args...)
	cmd.Stdout = &output
	cmd.Stderr = &stderr

	if err = cmd.Run(); err != nil {
		return "", fmt.Errorf("Failed to execute: %v %v, output %s, stderr, %s, error %w",
			binary, args, output.String(), stderr.String(), err)
	}
	return output.String(), nil
}

func (ne *NamespaceExecutor) ExecuteWithStdin(name string, args []string, stdinString string) (string, error) {
	if ne.ns == "" {
		return ExecuteWithStdin(name, args, stdinString)
	}
	cmdArgs := []string{
		"--mount=" + filepath.Join(ne.ns, "mnt"),
		"--net=" + filepath.Join(ne.ns, "net"),
		name,
	}
	cmdArgs = append(cmdArgs, args...)
	return ExecuteWithStdin(NSBinary, cmdArgs, stdinString)
}

func ExecuteWithStdin(binary string, args []string, stdinString string) (string, error) {
	var err error
	cmd := exec.Command(binary, args...)
	done := make(chan struct{})

	var output, stderr bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}

	go func() {
		defer stdin.Close()
		io.WriteString(stdin, stdinString)
	}()

	go func() {
		err = cmd.Run()
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-time.After(cmdTimeout):
		if cmd.Process != nil {
			if err := cmd.Process.Kill(); err != nil {
				logrus.Warnf("Problem killing process pid=%v: %s", cmd.Process.Pid, err)
			}

		}
		return "", fmt.Errorf("Timeout executing: %v %v, output %s, stderr, %s, error %w",
			binary, args, output.String(), stderr.String(), err)
	}

	if err != nil {
		return "", fmt.Errorf("Failed to execute: %v %v, output %s, stderr, %s, error %w",
			binary, args, output.String(), stderr.String(), err)
	}
	return output.String(), nil
}

func RemoveFile(file string) error {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		// file doesn't exist
		return nil
	}

	if err := remove(file); err != nil {
		return fmt.Errorf("fail to remove file %v: %w", file, err)
	}

	return nil
}

func RemoveDevice(dev string) error {
	if _, err := os.Stat(dev); err == nil {
		if err := remove(dev); err != nil {
			return fmt.Errorf("Failed to removing device %s, %w", dev, err)
		}
	}
	return nil
}

func GetKnownDevices(ne *NamespaceExecutor) (map[string]*KernelDevice, error) {
	knownDevices := make(map[string]*KernelDevice)

	/* Example command output
	   $ lsblk -l -n -o NAME,MAJ:MIN
	   sda           8:0
	   sdb           8:16
	   sdc           8:32
	   nvme0n1     259:0
	   nvme0n1p1   259:1
	   nvme0n1p128 259:2
	   nvme1n1     259:3
	*/

	opts := []string{
		"-l", "-n", "-o", "NAME,MAJ:MIN",
	}

	output, err := ne.Execute(LSBLKBinary, opts)
	if err != nil {
		return knownDevices, err
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		f := strings.Fields(line)
		if len(f) == 2 {
			dev := &KernelDevice{
				Name: f[0],
			}
			if _, err := fmt.Sscanf(f[1], "%d:%d", &dev.Major, &dev.Minor); err != nil {
				return nil, fmt.Errorf("Invalid major:minor %s for device %s", dev.Name, f[1])
			}
			knownDevices[dev.Name] = dev
		}
	}

	return knownDevices, nil
}

func DuplicateDevice(dev *KernelDevice, dest string) error {
	if err := mknod(dest, dev.Major, dev.Minor); err != nil {
		return fmt.Errorf("Cannot create device node %s for device %s", dest, dev.Name)
	}
	if err := os.Chmod(dest, 0660); err != nil {
		return fmt.Errorf("Couldn't change permission of the device %s: %w", dest, err)
	}
	return nil
}

func mknod(device string, major, minor int) error {
	var fileMode os.FileMode = 0660
	fileMode |= unix.S_IFBLK
	dev := int(unix.Mkdev(uint32(major), uint32(minor)))

	logrus.Infof("Creating device %s %d:%d", device, major, minor)
	return unix.Mknod(device, uint32(fileMode), dev)
}

func removeAsync(path string, done chan<- error) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logrus.Errorf("Unable to remove: %v", path)
		done <- err
	}
	done <- nil
}

func remove(path string) error {
	done := make(chan error)
	go removeAsync(path, done)
	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout trying to delete %s", path)
	}
}
