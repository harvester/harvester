package util

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/handlers"
	"github.com/pkg/errors"
	"github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"

	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/version"
	clientset "k8s.io/client-go/kubernetes"

	iscsi_util "github.com/longhorn/go-iscsi-helper/util"
)

const (
	VolumeStackPrefix     = "volume-"
	ControllerServiceName = "controller"
	ReplicaServiceName    = "replica"

	BackupStoreTypeS3 = "s3"
	AWSAccessKey      = "AWS_ACCESS_KEY_ID"
	AWSSecretKey      = "AWS_SECRET_ACCESS_KEY"
	AWSEndPoint       = "AWS_ENDPOINTS"
	AWSCert           = "AWS_CERT"

	HTTPSProxy = "HTTPS_PROXY"
	HTTPProxy  = "HTTP_PROXY"
	NOProxy    = "NO_PROXY"

	VirtualHostedStyle = "VIRTUAL_HOSTED_STYLE"

	HostProcPath                 = "/host/proc"
	ReplicaDirectory             = "/replicas/"
	DeviceDirectory              = "/dev/longhorn/"
	TemporaryMountPointDirectory = "/tmp/mnt/"

	DefaultKubernetesTolerationKey = "kubernetes.io"

	DiskConfigFile = "longhorn-disk.cfg"

	SizeAlignment     = 2 * 1024 * 1024
	MinimalVolumeSize = 10 * 1024 * 1024
)

var (
	cmdTimeout     = time.Minute // one minute by default
	reservedLabels = []string{"KubernetesStatus", "ranchervm-base-image"}

	ConflictRetryInterval = 20 * time.Millisecond
	ConflictRetryCounts   = 100
)

type MetadataConfig struct {
	DriverName          string
	Image               string
	OrcImage            string
	DriverContainerName string
}

type DiskInfo struct {
	Fsid             string
	Path             string
	Type             string
	FreeBlock        int64
	TotalBlock       int64
	BlockSize        int64
	StorageMaximum   int64
	StorageAvailable int64
}

func ConvertSize(size interface{}) (int64, error) {
	switch size := size.(type) {
	case int64:
		return size, nil
	case int:
		return int64(size), nil
	case string:
		if size == "" {
			return 0, nil
		}
		quantity, err := resource.ParseQuantity(size)
		if err != nil {
			return 0, errors.Wrapf(err, "error parsing size '%s'", size)
		}
		return quantity.Value(), nil
	}
	return 0, errors.Errorf("could not parse size '%v'", size)
}

func RoundUpSize(size int64) int64 {
	if size <= 0 {
		return SizeAlignment
	}
	r := size % SizeAlignment
	if r == 0 {
		return size
	}
	return size - r + SizeAlignment
}

func Backoff(maxDuration time.Duration, timeoutMessage string, f func() (bool, error)) error {
	startTime := time.Now()
	waitTime := 150 * time.Millisecond
	maxWaitTime := 2 * time.Second
	for {
		if time.Now().Sub(startTime) > maxDuration {
			return errors.New(timeoutMessage)
		}

		if done, err := f(); err != nil {
			return err
		} else if done {
			return nil
		}

		time.Sleep(waitTime)

		waitTime *= 2
		if waitTime > maxWaitTime {
			waitTime = maxWaitTime
		}
	}
}

func UUID() string {
	return uuid.NewV4().String()
}

// WaitForDevice timeout in second
func WaitForDevice(dev string, timeout int) error {
	for i := 0; i < timeout; i++ {
		st, err := os.Stat(dev)
		if err == nil {
			if st.Mode()&os.ModeDevice == 0 {
				return fmt.Errorf("Invalid mode for %v: 0x%x", dev, st.Mode())
			}
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout waiting for %v", dev)
}

func RandomID() string {
	return UUID()[:8]
}

func GetLocalIPs() ([]string, error) {
	results := []string{}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		if ip, ok := addr.(*net.IPNet); ok && !ip.IP.IsLoopback() {
			if ip.IP.To4() != nil {
				results = append(results, ip.IP.String())
			}
		}
	}
	return results, nil
}

// WaitForAPI timeout in second
func WaitForAPI(url string, timeout int) error {
	for i := 0; i < timeout; i++ {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout waiting for %v", url)
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func ParseTime(t string) (time.Time, error) {
	return time.Parse(time.RFC3339, t)

}

func Execute(binary string, args ...string) (string, error) {
	return ExecuteWithTimeout(cmdTimeout, binary, args...)
}

func ExecuteWithTimeout(timeout time.Duration, binary string, args ...string) (string, error) {
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
		return "", fmt.Errorf("Timeout executing: %v %v, output %s, stderr, %s, error %v",
			binary, args, output.String(), stderr.String(), err)
	}

	if err != nil {
		return "", fmt.Errorf("Failed to execute: %v %v, output %s, stderr, %s, error %v",
			binary, args, output.String(), stderr.String(), err)
	}
	return output.String(), nil
}

func ExecuteWithoutTimeout(binary string, args ...string) (string, error) {
	cmd := exec.Command(binary, args...)

	var output, stderr bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return output.String(), fmt.Errorf("failed to execute: %v %v, output %s, stderr, %s, error %v",
			binary, args, output.String(), stderr.String(), err)
	}
	return output.String(), nil
}

func TimestampAfterTimeout(ts string, timeout time.Duration) bool {
	now := time.Now()
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		logrus.Errorf("Cannot parse time %v", ts)
		return false
	}
	deadline := t.Add(timeout)
	return now.After(deadline)
}

func TimestampWithinLimit(latest time.Time, ts string, limit time.Duration) bool {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		logrus.Errorf("Cannot parse time %v", ts)
		return false
	}
	deadline := t.Add(limit)
	return deadline.After(latest)
}

func ValidateName(name string) bool {
	validName := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]+$`)
	return validName.MatchString(name)
}

func GetBackupID(backupURL string) (string, error) {
	u, err := url.Parse(backupURL)
	if err != nil {
		return "", err
	}
	v := u.Query()
	volumeName := v.Get("volume")
	backupName := v.Get("backup")
	if !ValidateName(volumeName) || !ValidateName(backupName) {
		return "", fmt.Errorf("Invalid name parsed, got %v and %v", backupName, volumeName)
	}
	return backupName, nil
}

func GetRequiredEnv(key string) (string, error) {
	env := os.Getenv(key)
	if env == "" {
		return "", fmt.Errorf("can't get required environment variable, env %v wasn't set", key)
	}
	return env, nil
}

// ParseLabels parses the provided Labels based on longhorn-engine's implementation:
// https://github.com/longhorn/longhorn-engine/blob/master/util/util.go
func ParseLabels(labels []string) (map[string]string, error) {
	result := map[string]string{}
	for _, label := range labels {
		kv := strings.SplitN(label, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid label not in <key>=<value> format %v", label)
		}
		key := kv[0]
		value := kv[1]
		if errList := validation.IsQualifiedName(key); len(errList) > 0 {
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

func RegisterShutdownChannel(done chan struct{}) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		logrus.Infof("Receive %v to exit", sig)
		close(done)
	}()
}

func SplitStringToMap(str, separator string) map[string]struct{} {
	ret := map[string]struct{}{}
	splits := strings.Split(str, separator)
	for _, str := range splits {
		// splits can have empty member
		str = strings.TrimSpace(str)
		if str == "" {
			continue
		}
		ret[str] = struct{}{}
	}
	return ret
}

func GetStringChecksum(data string) string {
	return GetChecksumSHA512([]byte(data))
}

func GetChecksumSHA512(data []byte) string {
	checksum := sha512.Sum512(data)
	return hex.EncodeToString(checksum[:])
}

func CheckBackupType(backupTarget string) (string, error) {
	u, err := url.Parse(backupTarget)
	if err != nil {
		return "", err
	}

	return u.Scheme, nil
}

func ConfigBackupCredential(backupTarget string, credential map[string]string) error {
	backupType, err := CheckBackupType(backupTarget)
	if err != nil {
		return err
	}
	if backupType == BackupStoreTypeS3 {
		// environment variable has been set in cronjob
		if credential != nil && credential[AWSAccessKey] != "" && credential[AWSSecretKey] != "" {
			os.Setenv(AWSAccessKey, credential[AWSAccessKey])
			os.Setenv(AWSSecretKey, credential[AWSSecretKey])
			os.Setenv(AWSEndPoint, credential[AWSEndPoint])
			os.Setenv(AWSCert, credential[AWSCert])
			os.Setenv(HTTPSProxy, credential[HTTPSProxy])
			os.Setenv(HTTPProxy, credential[HTTPProxy])
			os.Setenv(NOProxy, credential[NOProxy])
			os.Setenv(VirtualHostedStyle, credential[VirtualHostedStyle])
		} else if os.Getenv(AWSAccessKey) == "" || os.Getenv(AWSSecretKey) == "" {
			return fmt.Errorf("Could not backup for %s without credential secret", backupType)
		}
	}
	return nil
}

func ConfigEnvWithCredential(backupTarget string, credentialSecret string, hasEndpoint bool, hasCert bool,
	container *v1.Container) error {
	backupType, err := CheckBackupType(backupTarget)
	if err != nil {
		return err
	}
	if backupType == BackupStoreTypeS3 && credentialSecret != "" {
		accessKeyEnv := v1.EnvVar{
			Name: AWSAccessKey,
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: credentialSecret,
					},
					Key: AWSAccessKey,
				},
			},
		}
		container.Env = append(container.Env, accessKeyEnv)
		secretKeyEnv := v1.EnvVar{
			Name: AWSSecretKey,
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: credentialSecret,
					},
					Key: AWSSecretKey,
				},
			},
		}
		container.Env = append(container.Env, secretKeyEnv)
		if hasEndpoint {
			endpointEnv := v1.EnvVar{
				Name: AWSEndPoint,
				ValueFrom: &v1.EnvVarSource{
					SecretKeyRef: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{
							Name: credentialSecret,
						},
						Key: AWSEndPoint,
					},
				},
			}
			container.Env = append(container.Env, endpointEnv)
		}
		if hasCert {
			certEnv := v1.EnvVar{
				Name: AWSCert,
				ValueFrom: &v1.EnvVarSource{
					SecretKeyRef: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{
							Name: credentialSecret,
						},
						Key: AWSCert,
					},
				},
			}
			container.Env = append(container.Env, certEnv)
		}
	}
	return nil
}

func GetDiskInfo(directory string) (info *DiskInfo, err error) {
	defer func() {
		err = errors.Wrapf(err, "cannot get disk info of directory %v", directory)
	}()
	initiatorNSPath := iscsi_util.GetHostNamespacePath(HostProcPath)
	mountPath := fmt.Sprintf("--mount=%s/mnt", initiatorNSPath)
	output, err := Execute("nsenter", mountPath, "stat", "-fc", "{\"path\":\"%n\",\"fsid\":\"%i\",\"type\":\"%T\",\"freeBlock\":%f,\"totalBlock\":%b,\"blockSize\":%S}", directory)
	if err != nil {
		return nil, err
	}
	output = strings.Replace(output, "\n", "", -1)

	diskInfo := &DiskInfo{}
	err = json.Unmarshal([]byte(output), diskInfo)
	if err != nil {
		return nil, err
	}

	diskInfo.StorageMaximum = diskInfo.TotalBlock * diskInfo.BlockSize
	diskInfo.StorageAvailable = diskInfo.FreeBlock * diskInfo.BlockSize

	return diskInfo, nil
}

func RetryOnConflictCause(fn func() (interface{}, error)) (obj interface{}, err error) {
	for i := 0; i < ConflictRetryCounts; i++ {
		obj, err = fn()
		if err == nil {
			return obj, nil
		}
		if !apierrors.IsConflict(errors.Cause(err)) {
			return nil, err
		}
		time.Sleep(ConflictRetryInterval)
	}
	return nil, errors.Wrapf(err, "cannot finish API request due to too many conflicts")
}

func RunAsync(wg *sync.WaitGroup, f func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		f()
	}()
}

func RemoveHostDirectoryContent(directory string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to remove host directory %v", directory)
	}()

	dir, err := filepath.Abs(filepath.Clean(directory))
	if err != nil {
		return err
	}
	if strings.Count(dir, "/") < 2 {
		return fmt.Errorf("prohibit removing the top level of directory %v", dir)
	}
	initiatorNSPath := iscsi_util.GetHostNamespacePath(HostProcPath)
	nsExec, err := iscsi_util.NewNamespaceExecutor(initiatorNSPath)
	if err != nil {
		return err
	}
	// check if the directory already deleted
	if _, err := nsExec.Execute("ls", []string{dir}); err != nil {
		logrus.Warnf("cannot find host directory %v for removal", dir)
		return nil
	}
	if _, err := nsExec.Execute("rm", []string{"-rf", dir}); err != nil {
		return err
	}
	return nil
}

func CopyHostDirectoryContent(src, dest string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to copy the content from %v to %v for the host", src, dest)
	}()

	srcDir, err := filepath.Abs(filepath.Clean(src))
	if err != nil {
		return err
	}
	destDir, err := filepath.Abs(filepath.Clean(dest))
	if err != nil {
		return err
	}
	if strings.Count(srcDir, "/") < 2 || strings.Count(destDir, "/") < 2 {
		return fmt.Errorf("prohibit copying the content for the top level of directory %v or %v", srcDir, destDir)
	}

	initiatorNSPath := iscsi_util.GetHostNamespacePath(HostProcPath)
	nsExec, err := iscsi_util.NewNamespaceExecutor(initiatorNSPath)
	if err != nil {
		return err
	}

	// There can be no src directory, hence returning nil is fine.
	if _, err := nsExec.Execute("bash", []string{"-c", fmt.Sprintf("ls %s", filepath.Join(srcDir, "*"))}); err != nil {
		logrus.Infof("cannot list the content of the src directory %v for the copy, will do nothing: %v", srcDir, err)
		return nil
	}
	// Check if the dest directory exists.
	if _, err := nsExec.Execute("mkdir", []string{"-p", destDir}); err != nil {
		return err
	}
	// The flag `-n` means not overwriting an existing file.
	if _, err := nsExec.Execute("bash", []string{"-c", fmt.Sprintf("cp -an %s %s", filepath.Join(srcDir, "*"), destDir)}); err != nil {
		return err
	}
	return nil
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

func ValidateSnapshotLabels(labels map[string]string) (map[string]string, error) {
	validLabels := make(map[string]string)
	for key, val := range labels {
		if errList := validation.IsQualifiedName(key); len(errList) > 0 {
			return nil, fmt.Errorf("at least one error encountered while validating backup label with key %v: %v",
				key, errList[0])
		}
		if val == "" {
			return nil, fmt.Errorf("value for label with key %v cannot be empty", key)
		}
		validLabels[key] = val
	}

	for _, key := range reservedLabels {
		if _, ok := validLabels[key]; ok {
			return nil, fmt.Errorf("specified snapshot backup labels contain reserved keyword %v", key)
		}
	}

	return validLabels, nil
}

func ValidateTags(inputTags []string) ([]string, error) {
	foundTags := make(map[string]struct{})
	var tags []string
	for _, tag := range inputTags {
		if _, ok := foundTags[tag]; ok {
			continue
		}
		errList := validation.IsQualifiedName(tag)
		if len(errList) > 0 {
			return nil, fmt.Errorf("at least one error encountered while validating tags: %v", errList[0])
		}
		foundTags[tag] = struct{}{}
		tags = append(tags, tag)
	}

	sort.Strings(tags)

	return tags, nil
}

func CreateDiskPathReplicaSubdirectory(path string) error {
	nsPath := iscsi_util.GetHostNamespacePath(HostProcPath)
	nsExec, err := iscsi_util.NewNamespaceExecutor(nsPath)
	if err != nil {
		return err
	}
	if _, err := nsExec.Execute("mkdir", []string{"-p", filepath.Join(path, ReplicaDirectory)}); err != nil {
		return errors.Wrapf(err, "error creating data path %v on host", path)
	}

	return nil
}

func DeleteDiskPathReplicaSubdirectoryAndDiskCfgFile(
	nsExec *iscsi_util.NamespaceExecutor, path string) error {

	var err error
	dirPath := filepath.Join(path, ReplicaDirectory)
	filePath := filepath.Join(path, DiskConfigFile)

	// Check if the replica directory exist, delete it
	if _, err := nsExec.Execute("ls", []string{dirPath}); err == nil {
		if _, err := nsExec.Execute("rmdir", []string{dirPath}); err != nil {
			return errors.Wrapf(err, "error deleting data path %v on host", path)
		}
	}

	// Check if the disk cfg file exist, delete it
	if _, err := nsExec.Execute("ls", []string{filePath}); err == nil {
		if _, err := nsExec.Execute("rm", []string{filePath}); err != nil {
			err = errors.Wrapf(err, "error deleting disk cfg file %v on host", filePath)
		}
	}

	return err
}

func IsKubernetesDefaultToleration(toleration v1.Toleration) bool {
	if strings.Contains(toleration.Key, DefaultKubernetesTolerationKey) {
		return true
	}
	return false
}

func GetAnnotation(obj runtime.Object, annotationKey string) (string, error) {
	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return "", fmt.Errorf("cannot get annotation of invalid object %v: %v", obj, err)
	}

	annos := objMeta.GetAnnotations()
	if annos == nil {
		return "", nil
	}

	return annos[annotationKey], nil
}

func SetAnnotation(obj runtime.Object, annotationKey, annotationValue string) error {
	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return fmt.Errorf("cannot set annotation for invalid object %v: %v", obj, err)
	}

	annos := objMeta.GetAnnotations()
	if annos == nil {
		annos = map[string]string{}
	}

	annos[annotationKey] = annotationValue
	objMeta.SetAnnotations(annos)
	return nil
}

func GetDistinctTolerations(tolerationList []v1.Toleration) []v1.Toleration {
	res := []v1.Toleration{}
	tolerationMap := TolerationListToMap(tolerationList)
	for _, t := range tolerationMap {
		res = append(res, t)
	}
	return res
}

func TolerationListToMap(tolerationList []v1.Toleration) map[string]v1.Toleration {
	res := map[string]v1.Toleration{}
	for _, t := range tolerationList {
		// We use checksum of the toleration to separate 2 tolerations
		// with the same t.Key but different operator/effect/value
		res[GetTolerationChecksum(t)] = t
	}
	return res
}

func GetTolerationChecksum(t v1.Toleration) string {
	return GetStringChecksum(string(t.Key) + string(t.Operator) + string(t.Value) + string(t.Effect))
}

func ExpandFileSystem(volumeName string) (err error) {
	devicePath := filepath.Join(DeviceDirectory, volumeName)
	nsPath := iscsi_util.GetHostNamespacePath(HostProcPath)
	nsExec, err := iscsi_util.NewNamespaceExecutor(nsPath)
	if err != nil {
		return err
	}

	fsType, err := DetectFileSystem(volumeName)
	if err != nil {
		return err
	}
	if !IsSupportedFileSystem(fsType) {
		return fmt.Errorf("volume %v is using unsupported file system %v", volumeName, fsType)
	}

	// make sure there is a mount point for the volume before file system expansion
	tmpMountNeeded := true
	mountPoint := ""
	mountRes, err := nsExec.Execute("bash", []string{"-c", "mount | grep \"/" + volumeName + " \" | awk '{print $3}'"})
	if err != nil {
		logrus.Warnf("failed to use command mount to get the mount info of volume %v, consider the volume as unmounted: %v", volumeName, err)
	} else {
		// For empty `mountRes`, `mountPoints` is [""]
		mountPoints := strings.Split(strings.TrimSpace(mountRes), "\n")
		if !(len(mountPoints) == 1 && strings.TrimSpace(mountPoints[0]) == "") {
			// pick up a random mount point
			for _, m := range mountPoints {
				mountPoint = strings.TrimSpace(m)
				if mountPoint != "" {
					tmpMountNeeded = false
					break
				}
			}
			if tmpMountNeeded {
				logrus.Errorf("BUG: Found mount point records %v for volume %v but there is no valid(non-empty) mount point", mountRes, volumeName)
			}
		}
	}
	if tmpMountNeeded {
		mountPoint = filepath.Join(TemporaryMountPointDirectory, volumeName)
		logrus.Infof("The volume %v is unmounted, hence it will be temporarily mounted on %v for file system expansion", volumeName, mountPoint)
		if _, err := nsExec.Execute("mkdir", []string{"-p", mountPoint}); err != nil {
			return errors.Wrapf(err, "failed to create a temporary mount point %v before file system expansion", mountPoint)
		}
		if _, err := nsExec.Execute("mount", []string{devicePath, mountPoint}); err != nil {
			return errors.Wrapf(err, "failed to temporarily mount volume %v on %v before file system expansion", volumeName, mountPoint)
		}
	}

	switch fsType {
	case "ext2":
		fallthrough
	case "ext3":
		fallthrough
	case "ext4":
		if _, err = nsExec.Execute("resize2fs", []string{devicePath}); err != nil {
			return err
		}
	case "xfs":
		if _, err = nsExec.Execute("xfs_growfs", []string{mountPoint}); err != nil {
			return err
		}
	default:
		return fmt.Errorf("volume %v is using unsupported file system %v", volumeName, fsType)
	}

	// cleanup
	if tmpMountNeeded {
		if _, err := nsExec.Execute("umount", []string{mountPoint}); err != nil {
			return errors.Wrapf(err, "failed to unmount volume %v on the temporary mount point %v after file system expansion", volumeName, mountPoint)
		}
		if _, err := nsExec.Execute("rm", []string{"-r", mountPoint}); err != nil {
			return errors.Wrapf(err, "failed to remove the temporary mount point %v after file system expansion", mountPoint)
		}
	}

	return nil
}

func DetectFileSystem(volumeName string) (string, error) {
	devicePath := filepath.Join(DeviceDirectory, volumeName)
	nsPath := iscsi_util.GetHostNamespacePath(HostProcPath)
	nsExec, err := iscsi_util.NewNamespaceExecutor(nsPath)
	if err != nil {
		return "", err
	}

	// The output schema of `blkid` can be different.
	// For filesystem `btrfs`, the schema is: `<device path>: UUID="<filesystem UUID>" UUID_SUB="<filesystem UUID_SUB>" TYPE="<filesystem type>"`
	// For filesystem `ext4` or `xfs`, the schema is: `<device path>: UUID="<filesystem UUID>" TYPE="<filesystem type>"`
	cmd := fmt.Sprintf("blkid %s | sed 's/.*TYPE=//g'", devicePath)
	output, err := nsExec.Execute("bash", []string{"-c", cmd})
	if err != nil {
		return "", errors.Wrapf(err, "failed to get the file system info for volume %v, maybe there is no Linux file system on the volume", volumeName)
	}
	fsType := strings.Trim(strings.TrimSpace(output), "\"")
	if fsType == "" {
		return "", fmt.Errorf("cannot get the filesystem type by using the command %v", cmd)
	}
	return fsType, nil
}

func IsSupportedFileSystem(fsType string) bool {
	if fsType == "ext4" || fsType == "ext3" || fsType == "ext2" || fsType == "xfs" {
		return true
	}
	return false
}

func IsKubernetesVersionAtLeast(kubeClient clientset.Interface, vers string) (bool, error) {
	serverVersion, err := kubeClient.Discovery().ServerVersion()
	if err != nil {
		return false, errors.Wrap(err, "failed to get Kubernetes server version")
	}
	currentVersion := version.MustParseSemantic(serverVersion.GitVersion)
	minVersion := version.MustParseSemantic(vers)
	return currentVersion.AtLeast(minVersion), nil
}

type DiskConfig struct {
	DiskUUID string `json:"diskUUID"`
}

func GetDiskConfig(path string) (*DiskConfig, error) {
	nsPath := iscsi_util.GetHostNamespacePath(HostProcPath)
	nsExec, err := iscsi_util.NewNamespaceExecutor(nsPath)
	if err != nil {
		return nil, err
	}
	filePath := filepath.Join(path, DiskConfigFile)
	output, err := nsExec.Execute("cat", []string{filePath})
	if err != nil {
		return nil, fmt.Errorf("cannot find config file %v on host: %v", filePath, err)
	}

	cfg := &DiskConfig{}
	if err := json.Unmarshal([]byte(output), cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %v content %v on host: %v", filePath, output, err)
	}
	return cfg, nil
}

func GenerateDiskConfig(path string) (*DiskConfig, error) {
	cfg := &DiskConfig{
		DiskUUID: UUID(),
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("BUG: Cannot marshal %+v: %v", cfg, err)
	}

	nsPath := iscsi_util.GetHostNamespacePath(HostProcPath)
	nsExec, err := iscsi_util.NewNamespaceExecutor(nsPath)
	if err != nil {
		return nil, err
	}
	filePath := filepath.Join(path, DiskConfigFile)
	if _, err := nsExec.Execute("ls", []string{filePath}); err == nil {
		return nil, fmt.Errorf("disk cfg on %v exists, cannot override", filePath)
	}

	defer func() {
		if err != nil {
			if derr := DeleteDiskPathReplicaSubdirectoryAndDiskCfgFile(nsExec, path); derr != nil {
				err = errors.Wrapf(err, "cleaning up disk config path %v failed with error: %v", path, derr)
			}

		}
	}()

	if _, err := nsExec.ExecuteWithStdin("dd", []string{"of=" + filePath}, string(encoded)); err != nil {
		return nil, fmt.Errorf("cannot write to disk cfg on %v: %v", filePath, err)
	}
	if err := CreateDiskPathReplicaSubdirectory(path); err != nil {
		return nil, err
	}
	if _, err := nsExec.Execute("sync", []string{filePath}); err != nil {
		return nil, fmt.Errorf("cannot sync disk cfg on %v: %v", filePath, err)
	}

	return cfg, nil
}
