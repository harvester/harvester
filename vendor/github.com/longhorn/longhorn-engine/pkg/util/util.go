package util

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/handlers"
	iutil "github.com/longhorn/go-iscsi-helper/util"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/longhorn/longhorn-engine/pkg/types"
)

var (
	MaximumVolumeNameSize = 64
	validVolumeName       = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]+$`)

	cmdTimeout = time.Minute // one minute by default

	HostProc = "/host/proc"
)

const (
	BlockSizeLinux = 512
)

func ParseAddresses(name string) (string, string, string, int, error) {
	host, strPort, err := net.SplitHostPort(name)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("invalid address %s : couldn't find host and port", name)
	}

	port, _ := strconv.Atoi(strPort)

	return net.JoinHostPort(host, strconv.Itoa(port)),
		net.JoinHostPort(host, strconv.Itoa(port+1)),
		net.JoinHostPort(host, strconv.Itoa(port+2)),
		port + 2, nil
}

func GetGRPCAddress(address string) string {
	if strings.HasPrefix(address, "tcp://") {
		address = strings.TrimPrefix(address, "tcp://")
	}

	if strings.HasPrefix(address, "http://") {
		address = strings.TrimPrefix(address, "http://")
	}

	if strings.HasSuffix(address, "/v1") {
		address = strings.TrimSuffix(address, "/v1")
	}

	return address
}

func GetPortFromAddress(address string) (int, error) {
	if strings.HasSuffix(address, "/v1") {
		address = strings.TrimSuffix(address, "/v1")
	}

	_, strPort, err := net.SplitHostPort(address)
	if err != nil {
		return 0, fmt.Errorf("invalid address %s, must have a port in it", address)
	}

	port, err := strconv.Atoi(strPort)
	if err != nil {
		return 0, err
	}

	return port, nil
}

func UUID() string {
	return uuid.New().String()
}

func Filter(list []string, check func(string) bool) []string {
	result := make([]string, 0, len(list))
	for _, i := range list {
		if check(i) {
			result = append(result, i)
		}
	}
	return result
}

func Contains(arr []string, val string) bool {
	for _, a := range arr {
		if a == val {
			return true
		}
	}
	return false
}

type filteredLoggingHandler struct {
	filteredPaths  map[string]struct{}
	handler        http.Handler
	loggingHandler http.Handler
}

func FilteredLoggingHandler(filteredPaths map[string]struct{}, writer io.Writer, router http.Handler) http.Handler {
	return filteredLoggingHandler{
		filteredPaths:  filteredPaths,
		handler:        router,
		loggingHandler: handlers.CombinedLoggingHandler(writer, router),
	}
}

func (h filteredLoggingHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		if _, exists := h.filteredPaths[req.URL.Path]; exists {
			h.handler.ServeHTTP(w, req)
			return
		}
	}
	h.loggingHandler.ServeHTTP(w, req)
}

func DuplicateDevice(src, dest string) error {
	stat := unix.Stat_t{}
	if err := unix.Stat(src, &stat); err != nil {
		return fmt.Errorf("cannot duplicate device because cannot find %s: %v", src, err)
	}
	major := int(stat.Rdev / 256)
	minor := int(stat.Rdev % 256)
	if err := mknod(dest, major, minor); err != nil {
		return fmt.Errorf("cannot duplicate device %s to %s", src, dest)
	}
	if err := os.Chmod(dest, 0660); err != nil {
		return fmt.Errorf("couldn't change permission of the device %s: %s", dest, err)
	}
	return nil
}

func mknod(device string, major, minor int) error {
	var fileMode os.FileMode = 0660
	fileMode |= unix.S_IFBLK
	dev := int((major << 8) | (minor & 0xff) | ((minor & 0xfff00) << 12))

	logrus.Infof("Creating device %s %d:%d", device, major, minor)
	return unix.Mknod(device, uint32(fileMode), dev)
}

func RemoveDevice(dev string) error {
	if _, err := os.Stat(dev); err == nil {
		if err := remove(dev); err != nil {
			return fmt.Errorf("failed to removing device %s, %v", dev, err)
		}
	}
	return nil
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

func ValidVolumeName(name string) bool {
	if len(name) > MaximumVolumeNameSize {
		return false
	}
	return validVolumeName.MatchString(name)
}

func Volume2ISCSIName(name string) string {
	return strings.Replace(name, "_", ":", -1)
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func GetFileActualSize(file string) int64 {
	var st syscall.Stat_t
	if err := syscall.Stat(file, &st); err != nil {
		logrus.Errorf("Fail to get size of file %v: %v", file, err)
		return -1
	}
	return st.Blocks * BlockSizeLinux
}

func GetHeadFileModifyTimeAndSize(file string) (int64, int64, error) {
	var st syscall.Stat_t

	if err := syscall.Stat(file, &st); err != nil {
		logrus.Errorf("Fail to head file %v stat, err %v", file, err)
		return 0, 0, err
	}

	return st.Mtim.Nano(), st.Blocks * BlockSizeLinux, nil
}

func ParseLabels(labels []string) (map[string]string, error) {
	result := map[string]string{}
	for _, label := range labels {
		kv := strings.SplitN(label, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid label not in <key>=<value> format %v", label)
		}
		key := kv[0]
		value := kv[1]
		if errList := IsQualifiedName(key); len(errList) > 0 {
			return nil, fmt.Errorf("invalid key %v for label: %v", key, errList[0])
		}
		// We don't need to validate the Label value since we're allowing for any form of data to be stored, similar
		// to Kubernetes Annotations. Of course, we should make sure it isn't empty.
		if value == "" {
			return nil, fmt.Errorf("invalid empty value for label with key %v", key)
		}
		result[key] = value
	}
	return result, nil
}

func UnescapeURL(url string) string {
	// Deal with escape in url inputted from bash
	result := strings.Replace(url, "\\u0026", "&", 1)
	result = strings.Replace(result, "u0026", "&", 1)
	result = strings.TrimLeft(result, "\"'")
	result = strings.TrimRight(result, "\"'")
	return result
}

func CheckBackupType(backupTarget string) (string, error) {
	u, err := url.Parse(backupTarget)
	if err != nil {
		return "", err
	}

	return u.Scheme, nil
}

func GetBackupCredential(backupURL string) (map[string]string, error) {
	credential := map[string]string{}
	backupType, err := CheckBackupType(backupURL)
	if err != nil {
		return nil, err
	}
	if backupType == "s3" {
		accessKey := os.Getenv(types.AWSAccessKey)
		secretKey := os.Getenv(types.AWSSecretKey)
		if accessKey == "" && secretKey != "" {
			return nil, errors.New("could not backup to s3 without setting credential access key")
		}
		if accessKey != "" && secretKey == "" {
			return nil, errors.New("could not backup to s3 without setting credential secret access key")
		}
		if accessKey != "" && secretKey != "" {
			credential[types.AWSAccessKey] = accessKey
			credential[types.AWSSecretKey] = secretKey
		}

		credential[types.AWSEndPoint] = os.Getenv(types.AWSEndPoint)
		credential[types.AWSCert] = os.Getenv(types.AWSCert)
		credential[types.HTTPSProxy] = os.Getenv(types.HTTPSProxy)
		credential[types.HTTPProxy] = os.Getenv(types.HTTPProxy)
		credential[types.NOProxy] = os.Getenv(types.NOProxy)
		credential[types.VirtualHostedStyle] = os.Getenv(types.VirtualHostedStyle)
	}
	return credential, nil
}

func ResolveBackingFilepath(fileOrDirpath string) (string, error) {
	fileOrDir, err := os.Open(fileOrDirpath)
	if err != nil {
		return "", err
	}
	defer fileOrDir.Close()

	fileOrDirInfo, err := fileOrDir.Stat()
	if err != nil {
		return "", err
	}

	if fileOrDirInfo.IsDir() {
		files, err := fileOrDir.Readdir(-1)
		if err != nil {
			return "", err
		}
		if len(files) != 1 {
			return "", fmt.Errorf("expected exactly one file, found %d files/subdirectories", len(files))
		}
		if files[0].IsDir() {
			return "", fmt.Errorf("expected exactly one file, found a subdirectory")
		}
		return filepath.Join(fileOrDirpath, files[0].Name()), nil
	}

	return fileOrDirpath, nil
}

func GetInitiatorNS() string {
	return iutil.GetHostNamespacePath(HostProc)
}

func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}
