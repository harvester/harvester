package util

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/handlers"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
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

	HostProcPath                 = "/host/proc"
	ReplicaDirectory             = "/replicas/"
	DeviceDirectory              = "/dev/longhorn/"
	TemporaryMountPointDirectory = "/tmp/mnt/"

	DefaultKubernetesTolerationKey = "kubernetes.io"

	DiskConfigFile = "longhorn-disk.cfg"

	SizeAlignment     = 2 * 1024 * 1024
	MinimalVolumeSize = 10 * 1024 * 1024

	RandomIDLenth = 8
)

var (
	cmdTimeout     = time.Minute // one minute by default
	reservedLabels = []string{"KubernetesStatus", "ranchervm-base-image"}

	APIRetryInterval       = 500 * time.Millisecond
	APIRetryJitterInterval = 50 * time.Millisecond
	APIRetryCounts         = 10
)

type MetadataConfig struct {
	DriverName          string
	Image               string
	OrcImage            string
	DriverContainerName string
}

type DiskStat struct {
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
		if time.Since(startTime) > maxDuration {
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
	return uuid.New().String()
}

// WaitForDevice timeout in second
func WaitForDevice(dev string, timeout int) error {
	for i := 0; i < timeout; i++ {
		st, err := os.Stat(dev)
		if err == nil {
			if st.Mode()&os.ModeDevice == 0 {
				return fmt.Errorf("invalid mode for %v: 0x%x", dev, st.Mode())
			}
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout waiting for %v", dev)
}

func RandomID() string {
	return UUID()[:RandomIDLenth]
}

func ValidateRandomID(id string) bool {
	regex := fmt.Sprintf(`^[a-zA-Z0-9]{%d}$`, RandomIDLenth)
	validName := regexp.MustCompile(regex)
	return validName.MatchString(id)
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

func Execute(envs []string, binary string, args ...string) (string, error) {
	return ExecuteWithTimeout(cmdTimeout, envs, binary, args...)
}

func ExecuteWithTimeout(timeout time.Duration, envs []string, binary string, args ...string) (string, error) {
	var err error
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), envs...)
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
		return "", fmt.Errorf("timeout executing: %v %v, output %s, stderr, %s, error %v",
			binary, args, output.String(), stderr.String(), err)
	}

	if err != nil {
		return "", fmt.Errorf("failed to execute: %v %v, output %s, stderr, %s, error %v",
			binary, args, output.String(), stderr.String(), err)
	}
	return output.String(), nil
}

func ExecuteWithoutTimeout(envs []string, binary string, args ...string) (string, error) {
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), envs...)

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

func ValidateChecksumSHA512(checksum string) bool {
	validChecksum := regexp.MustCompile(`^[a-f0-9]{128}$`)
	return validChecksum.MatchString(checksum)
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
		return "", fmt.Errorf("invalid name parsed, got %v and %v", backupName, volumeName)
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

func GetSortedKeysFromMap(maps interface{}) []string {
	v := reflect.ValueOf(maps)
	if v.Kind() != reflect.Map {
		return nil
	}
	mapKeys := v.MapKeys()
	keys := make([]string, 0, len(mapKeys))
	for _, k := range mapKeys {
		keys = append(keys, k.String())
	}
	sort.Strings(keys)
	return keys
}

// AutoCorrectName converts name to lowercase, and correct overlength name by
// replaces the name suffix with 8 char from its checksum to ensure uniquenedoss.
func AutoCorrectName(name string, maxLength int) string {
	newName := strings.ToLower(name)
	if len(name) > maxLength {
		logrus.Warnf("Name %v is too long, auto-correct to fit %v characters", name, maxLength)
		checksum := GetStringChecksum(name)
		newNameSuffix := "-" + checksum[:8]
		newNamePrefix := strings.TrimRight(newName[:maxLength-len(newNameSuffix)], "-")
		newName = newNamePrefix + newNameSuffix
	}
	if newName != name {
		logrus.Warnf("Name auto-corrected from %v to %v", name, newName)
	}
	return newName
}

func GetStringChecksum(data string) string {
	return GetChecksumSHA512([]byte(data))
}

func GetChecksumSHA512(data []byte) string {
	checksum := sha512.Sum512(data)
	return hex.EncodeToString(checksum[:])
}

func GetStringChecksumSHA256(data string) string {
	return GetChecksumSHA256([]byte(data))
}

func GetChecksumSHA256(data []byte) string {
	checksum := sha256.Sum256(data)
	return hex.EncodeToString(checksum[:])
}

func GetStringHash(data string) string {
	hash := fnv.New32a()
	hash.Write([]byte(data))
	return fmt.Sprint(strconv.FormatInt(int64(hash.Sum32()), 16))
}

func CheckBackupType(backupTarget string) (string, error) {
	u, err := url.Parse(backupTarget)
	if err != nil {
		return "", err
	}

	return u.Scheme, nil
}

func GetDiskStat(directory string) (stat *DiskStat, err error) {
	defer func() {
		err = errors.Wrapf(err, "cannot get disk stat of directory %v", directory)
	}()
	initiatorNSPath := iscsi_util.GetHostNamespacePath(HostProcPath)
	mountPath := fmt.Sprintf("--mount=%s/mnt", initiatorNSPath)
	output, err := Execute([]string{}, "nsenter", mountPath, "stat", "-fc", "{\"path\":\"%n\",\"fsid\":\"%i\",\"type\":\"%T\",\"freeBlock\":%f,\"totalBlock\":%b,\"blockSize\":%S}", directory)
	if err != nil {
		return nil, err
	}
	output = strings.Replace(output, "\n", "", -1)

	diskStat := &DiskStat{}
	err = json.Unmarshal([]byte(output), diskStat)
	if err != nil {
		return nil, err
	}

	diskStat.StorageMaximum = diskStat.TotalBlock * diskStat.BlockSize
	diskStat.StorageAvailable = diskStat.FreeBlock * diskStat.BlockSize

	return diskStat, nil
}

func RetryOnConflictCause(fn func() (interface{}, error)) (interface{}, error) {
	return RetryOnErrorCondition(fn, apierrors.IsConflict)
}

func RetryOnNotFoundCause(fn func() (interface{}, error)) (interface{}, error) {
	return RetryOnErrorCondition(fn, apierrors.IsNotFound)
}

func RetryOnErrorCondition(fn func() (interface{}, error), predicate func(error) bool) (interface{}, error) {
	for i := 0; i < APIRetryCounts; i++ {
		obj, err := fn()
		if err == nil {
			return obj, nil
		}
		if !predicate(err) {
			return nil, err
		}
		time.Sleep(APIRetryInterval + APIRetryJitterInterval*time.Duration(rand.Intn(5)))
	}
	return nil, fmt.Errorf("cannot finish API request due to too many error retries")
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

func MinInt(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

func Contains(list []string, item string) bool {
	for _, i := range list {
		if i == item {
			return true
		}
	}
	return false
}

// HasLocalStorageInDeployment returns true if deployment has any local storage.
func HasLocalStorageInDeployment(deployment *appsv1.Deployment) bool {
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if isLocalVolume(&volume) {
			return true
		}
	}
	return false
}

func isLocalVolume(volume *v1.Volume) bool {
	return volume.HostPath != nil || volume.EmptyDir != nil
}

func GetPossibleReplicaDirectoryNames(diskPath string) (replicaDirectoryNames map[string]string, err error) {
	defer func() {
		err = errors.Wrapf(err, "cannot list replica directories in the disk %v", diskPath)
	}()

	replicaDirectoryNames = make(map[string]string, 0)

	directory := filepath.Join(diskPath, "replicas")

	initiatorNSPath := iscsi_util.GetHostNamespacePath(HostProcPath)
	mountPath := fmt.Sprintf("--mount=%s/mnt", initiatorNSPath)
	command := fmt.Sprintf("find %s -type d -maxdepth 1 -mindepth 1 -regextype posix-extended -regex \".*-[a-zA-Z0-9]{8}$\" -exec basename {} \\;", directory)
	output, err := Execute([]string{}, "nsenter", mountPath, "sh", "-c", command)
	if err != nil {
		return replicaDirectoryNames, err
	}

	names := strings.Split(output, "\n")
	for _, name := range names {
		if name != "" {
			replicaDirectoryNames[name] = ""
		}
	}

	return replicaDirectoryNames, nil
}

func DeleteReplicaDirectoryName(diskPath, replicaDirectoryName string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "cannot delete replica directory %v in disk %v", replicaDirectoryName, diskPath)
	}()

	path := filepath.Join(diskPath, "replicas", replicaDirectoryName)

	initiatorNSPath := iscsi_util.GetHostNamespacePath(HostProcPath)
	mountPath := fmt.Sprintf("--mount=%s/mnt", initiatorNSPath)
	_, err = Execute([]string{}, "nsenter", mountPath, "rm", "-rf", path)
	if err != nil {
		return err
	}

	return nil
}

type VolumeMeta struct {
	Size            int64
	Head            string
	Dirty           bool
	Rebuilding      bool
	Error           string
	Parent          string
	SectorSize      int64
	BackingFilePath string
	BackingFile     interface{}
}

func GetVolumeMeta(path string) (*VolumeMeta, error) {
	nsPath := iscsi_util.GetHostNamespacePath(HostProcPath)
	nsExec, err := iscsi_util.NewNamespaceExecutor(nsPath)
	if err != nil {
		return nil, err
	}

	output, err := nsExec.Execute("cat", []string{path})
	if err != nil {
		return nil, fmt.Errorf("cannot find volume meta %v on host: %v", path, err)
	}

	meta := &VolumeMeta{}
	if err := json.Unmarshal([]byte(output), meta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %v content %v on host: %v", path, output, err)
	}
	return meta, nil
}
