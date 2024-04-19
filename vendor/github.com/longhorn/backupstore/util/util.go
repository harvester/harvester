package util

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	lz4 "github.com/pierrec/lz4/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.uber.org/multierr"
	"golang.org/x/sys/unix"

	"k8s.io/apimachinery/pkg/util/wait"
	mount "k8s.io/mount-utils"
)

const (
	PreservedChecksumLength = 64

	MountDir = "/var/lib/longhorn-backupstore-mounts"
)

var (
	cmdTimeout = time.Minute // one minute by default

	forceCleanupMountTimeout = 30 * time.Second
)

// NopCloser wraps an io.Witer as io.WriteCloser
// with noop Close
type NopCloser struct {
	io.Writer
}

func (NopCloser) Close() error { return nil }

func fstypeToKind(fstype int64) (string, error) {
	switch fstype {
	case unix.NFS_SUPER_MAGIC:
		return "nfs", nil
	case unix.CIFS_SUPER_MAGIC, unix.SMB2_SUPER_MAGIC, unix.SMB_SUPER_MAGIC:
		return "cifs", nil
	default:
		return "", fmt.Errorf("unknown fstype %v", fstype)
	}
}

// GenerateName generates a 16-byte name
func GenerateName(prefix string) string {
	suffix := strings.Replace(NewUUID(), "-", "", -1)
	return prefix + "-" + suffix[:16]
}

// NewUUID generates an UUID
func NewUUID() string {
	return uuid.New().String()
}

// GetChecksum gets the SHA256 of the given data
func GetChecksum(data []byte) string {
	checksumBytes := sha512.Sum512(data)
	checksum := hex.EncodeToString(checksumBytes[:])[:PreservedChecksumLength]
	return checksum
}

// GetFileChecksum calculates the SHA256 of the file's content
func GetFileChecksum(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// CompressData compresses the given data using the specified compression method
func CompressData(method string, data []byte) (io.ReadSeeker, error) {
	if method == "none" {
		return bytes.NewReader(data), nil
	}

	var buffer bytes.Buffer

	w, err := newCompressionWriter(method, &buffer)
	if err != nil {
		return nil, err
	}

	if _, err := w.Write(data); err != nil {
		w.Close()
		return nil, err
	}
	w.Close()
	return bytes.NewReader(buffer.Bytes()), nil
}

// DecompressAndVerify decompresses the given data and verifies the data integrity
func DecompressAndVerify(method string, src io.Reader, checksum string) (io.Reader, error) {
	r, err := newDecompressionReader(method, src)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create decompression reader")
	}
	defer r.Close()
	block, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read decompressed data")
	}
	if GetChecksum(block) != checksum {
		return nil, fmt.Errorf("checksum verification failed for block")
	}
	return bytes.NewReader(block), nil
}

// DecompressAndVerifyWithFallback decompresses the given data and verifies the data integrity.
// If the decompression fails, it will try to decompress with the fallback method.
func DecompressAndVerifyWithFallback(decompression string, rc io.ReadCloser, checksum string) (io.Reader, error) {
	r, err := DecompressAndVerify(decompression, rc, checksum)
	if err == nil {
		return r, nil
	}
	// Fall back to other decompression method if the current one fails
	// The mitigation will be removed after identifying https://github.com/longhorn/longhorn/issues/7687
	// Seek rc to offset 0
	seeker, ok := rc.(io.Seeker)
	if !ok {
		return nil, errors.Wrapf(err, "failed to cast to io.Seeker for block %v", checksum)
	}

	_, errFallback := seeker.Seek(0, io.SeekStart)
	if errFallback != nil {
		// Merge the err1 and err2 and error out
		return nil, errors.Wrapf(multierr.Append(err, errFallback), "failed to seek to offset 0 for block %v", checksum)
	}

	if strings.Contains(err.Error(), gzip.ErrHeader.Error()) {
		r, errFallback = DecompressAndVerify("lz4", rc, checksum)
	} else if strings.Contains(err.Error(), "lz4: bad magic number") {
		r, errFallback = DecompressAndVerify("gzip", rc, checksum)
	}
	if errFallback != nil {
		return nil, errors.Wrapf(multierr.Append(err, errFallback), "failed to decompress and verify block %v with fallback", checksum)
	}

	return r, err
}

func newCompressionWriter(method string, buffer io.Writer) (io.WriteCloser, error) {
	switch method {
	case "gzip":
		return gzip.NewWriter(buffer), nil
	case "lz4":
		return lz4.NewWriter(buffer), nil
	default:
		return nil, fmt.Errorf("unsupported compression method: %v", method)
	}
}

func newDecompressionReader(method string, r io.Reader) (io.ReadCloser, error) {
	switch method {
	case "none":
		return io.NopCloser(r), nil
	case "gzip":
		return gzip.NewReader(r)
	case "lz4":
		return io.NopCloser(lz4.NewReader(r)), nil
	default:
		return nil, fmt.Errorf("unsupported decompression method: %v", method)
	}
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func UnorderedEqual(x, y []string) bool {
	if len(x) != len(y) {
		return false
	}
	known := make(map[string]struct{})
	for _, value := range x {
		known[value] = struct{}{}
	}
	for _, value := range y {
		if _, present := known[value]; !present {
			return false
		}
	}
	return true
}

func Filter(elements []string, predicate func(string) bool) []string {
	var filtered []string
	for _, elem := range elements {
		if predicate(elem) {
			filtered = append(filtered, elem)
		}
	}
	return filtered
}

func ExtractNames(names []string, prefix, suffix string) []string {
	result := []string{}
	for _, f := range names {
		// Remove additional slash if exists
		f = strings.TrimLeft(f, "/")

		// missing prefix or suffix
		if !strings.HasPrefix(f, prefix) || !strings.HasSuffix(f, suffix) {
			continue
		}

		f = strings.TrimPrefix(f, prefix)
		f = strings.TrimSuffix(f, suffix)
		if !ValidateName(f) {
			logrus.Errorf("Invalid name %v was processed to extract name with prefix %v suffix %v",
				f, prefix, suffix)
			continue
		}
		result = append(result, f)
	}
	return result
}

// ValidateName validate the given string
func ValidateName(name string) bool {
	validName := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]+$`)
	return validName.MatchString(name)
}

// Execute executes a command
func Execute(binary string, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	return execute(ctx, binary, args)
}

// ExecuteWithCustomTimeout executes a command with a specified timeout
func ExecuteWithCustomTimeout(binary string, args []string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return execute(ctx, binary, args)
}

func execute(ctx context.Context, binary string, args []string) (string, error) {
	var output []byte
	var err error

	cmd := exec.CommandContext(ctx, binary, args...)
	done := make(chan struct{})

	go func() {
		output, err = cmd.CombinedOutput()
		close(done)
	}()

	select {
	case <-done:
		break
	case <-ctx.Done():
		return "", fmt.Errorf("timeout executing: %v %v, output %v, error %v", binary, args, string(output), err)
	}

	if err != nil {
		return "", fmt.Errorf("failed to execute: %v %v, output %v, error %v", binary, args, string(output), err)
	}

	return string(output), nil
}

// UnescapeURL converts a escape character to a normal one.
func UnescapeURL(url string) string {
	// Deal with escape in url inputted from bash
	result := strings.Replace(url, "\\u0026", "&", 1)
	result = strings.Replace(result, "u0026", "&", 1)
	return result
}

// IsMounted checks if the mount point is mounted
func IsMounted(mountPoint string) bool {
	output, err := Execute("mount", []string{})
	if err != nil {
		return false
	}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, " "+mountPoint+" ") {
			return true
		}
	}
	return false
}

func cleanupMount(mountDir string, mounter mount.Interface, log logrus.FieldLogger) error {
	forceUnmounter, ok := mounter.(mount.MounterForceUnmounter)
	if ok {
		log.Infof("Trying to force clean up mount point %v", mountDir)
		return mount.CleanupMountWithForce(mountDir, forceUnmounter, false, forceCleanupMountTimeout)
	}

	log.Infof("Trying to clean up mount point %v", mountDir)
	return mount.CleanupMountPoint(mountDir, forceUnmounter, false)
}

// EnsureMountPoint checks if the mount point is valid. If it is invalid, clean up mount point.
func EnsureMountPoint(Kind, mountPoint string, mounter mount.Interface, log logrus.FieldLogger) (mounted bool, err error) {
	defer func() {
		if !mounted && err == nil {
			if mkdirErr := os.MkdirAll(mountPoint, 0700); mkdirErr != nil {
				err = errors.Wrapf(err, "cannot create mount directory %v", mountPoint)
			}
		}
	}()

	notMounted, err := mount.IsNotMountPoint(mounter, mountPoint)
	if err == fs.ErrNotExist {
		return false, nil
	}

	IsCorruptedMnt := mount.IsCorruptedMnt(err)
	if !IsCorruptedMnt {
		log.Warnf("Trying reading mount point %v to make sure it is healthy", mountPoint)
		if _, readErr := os.ReadDir(mountPoint); readErr != nil {
			log.WithError(readErr).Warnf("Mount point %v was identified as corrupted by ReadDir", mountPoint)
			IsCorruptedMnt = true
		}
	}

	if IsCorruptedMnt {
		log.Warnf("Failed to check mount point %v (mounted=%v)", mountPoint, mounted)
		if mntErr := cleanupMount(mountPoint, mounter, log); mntErr != nil {
			return true, errors.Wrapf(mntErr, "failed to clean up corrupted mount point %v", mountPoint)
		}
		notMounted = true
	}

	if notMounted {
		return false, nil
	}

	var stat syscall.Statfs_t

	if err := syscall.Statfs(mountPoint, &stat); err != nil {
		return true, errors.Wrapf(err, "failed to statfs for mount point %v", mountPoint)
	}

	kind, err := fstypeToKind(int64(stat.Type))
	if err != nil {
		return true, errors.Wrapf(err, "failed to get kind for mount point %v", mountPoint)
	}

	if strings.Contains(kind, Kind) {
		return true, nil
	}

	log.Warnf("Cleaning up the mount point %v because the fstype %v is changed to %v", mountPoint, kind, Kind)

	if mntErr := cleanupMount(mountPoint, mounter, log); mntErr != nil {
		return true, errors.Wrapf(mntErr, "failed to clean up mount point %v (%v) for %v protocol", kind, mountPoint, Kind)
	}

	return false, nil
}

// MountWithTimeout mounts the backup store to a given mount point with a specified timeout
func MountWithTimeout(mounter mount.Interface, source string, target string, fstype string,
	options []string, sensitiveOptions []string, interval, timeout time.Duration) error {
	mountComplete := false
	err := wait.PollImmediate(interval, timeout, func() (bool, error) {
		err := mounter.MountSensitiveWithoutSystemd(source, target, fstype, options, sensitiveOptions)
		mountComplete = true
		return true, err
	})
	if !mountComplete {
		return errors.Wrapf(err, "mounting %v share %v on %v timed out", fstype, source, target)
	}
	return err
}

// CleanUpMountPoints tries to clean up all existing mount points for existing backup stores
func CleanUpMountPoints(mounter mount.Interface, log logrus.FieldLogger) error {
	var errs error

	filepath.Walk(MountDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			errs = multierr.Append(errs, errors.Wrapf(err, "failed to get file info of %v", path))
			return nil
		}

		if !info.IsDir() {
			return nil
		}

		notMounted, err := mount.IsNotMountPoint(mounter, path)
		if err != nil {
			errs = multierr.Append(errs, errors.Wrapf(err, "failed to check if %s is not mounted", path))
			return nil
		}

		if notMounted {
			return nil
		}

		if err := cleanupMount(path, mounter, log); err != nil {
			errs = multierr.Append(errs, errors.Wrapf(err, "failed to clean up mount point %v", path))
		}

		return nil
	})

	return errs
}

func CheckBackupType(backupTarget string) (string, error) {
	u, err := url.Parse(backupTarget)
	if err != nil {
		return "", err
	}

	return u.Scheme, nil
}

func SplitMountOptions(options []string) []string {
	logrus.Infof("Splitting array of %d strings %v ", len(options), options)
	if len(options) > 1 {
		// Options in the form "nfsOptions=soft&nfsOptions=timeo=450&nfsOptions=retrans=3" are legal and
		// are already split by url.Parse.
		return options
	}
	// Options in the form "nfsOptions=soft,timeo=450,retrans=3" are more likely, but we must split them.
	return strings.Split(options[0], ",")
}
