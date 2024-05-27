package util

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"reflect"
	"strings"

	"github.com/rancher/rke/metadata"

	sv "github.com/blang/semver"
	"github.com/coreos/go-semver/semver"
	ref "github.com/docker/distribution/reference"
	"github.com/sirupsen/logrus"
)

const (
	WorkerThreads               = 50
	SemVerK8sVersion122OrHigher = ">=1.22.0-rancher0"
)

var ProxyEnvVars = [3]string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}

func StrToSemVer(version string) (*semver.Version, error) {
	v, err := semver.NewVersion(strings.TrimPrefix(version, "v"))
	if err != nil {
		return nil, err
	}
	return v, nil
}

func SemVerMatchRange(k8sVersion, versionRange string) (bool, error) {
	if len(k8sVersion) == 0 {
		return false, fmt.Errorf("Cluster version has zero length")
	}
	toMatch, err := sv.Make(k8sVersion[1:])
	if err != nil {
		return false, fmt.Errorf("Cluster version [%s] can not be parsed as semver: %v", k8sVersion, err)
	}
	semVerRange, err := sv.ParseRange(versionRange)
	if err != nil {
		return false, fmt.Errorf("Failed to parse semver range [%s]: %v", versionRange, err)
	}
	if semVerRange(toMatch) {
		logrus.Debugf("SemVerMatchRange: Cluster version [%s] matches range [%s]", k8sVersion, versionRange)
		return true, nil
	}
	return false, nil
}

func RemoveZFromBinds(binds []string) []string {
	var returnSlice []string
	for _, element := range binds {
		lastIndex := strings.LastIndex(element, ":")
		bindOptions := element[lastIndex+1:]

		if bindOptions != "" {
			var cleanBindOptions []string

			splitBindOptions := strings.Split(bindOptions, ",")
			if len(splitBindOptions) >= 1 {
				for _, bindOption := range splitBindOptions {
					if strings.EqualFold(bindOption, "z") {
						continue
					}
					cleanBindOptions = append(cleanBindOptions, bindOption)

				}
			}
			var newString string
			if len(cleanBindOptions) > 0 {
				newString = fmt.Sprintf("%s%s", element[:lastIndex], fmt.Sprintf(":%s", strings.Join(cleanBindOptions, ",")))
			} else {
				newString = fmt.Sprintf("%s%s", element[:lastIndex], strings.Join(cleanBindOptions, ","))

			}
			returnSlice = append(returnSlice, newString)
		}
	}
	return returnSlice
}

func GetObjectQueue(l interface{}) chan interface{} {
	s := reflect.ValueOf(l)
	c := make(chan interface{}, s.Len())

	for i := 0; i < s.Len(); i++ {
		c <- s.Index(i).Interface()
	}
	close(c)
	return c
}

func ErrList(e []error) error {
	if len(e) > 0 {
		return fmt.Errorf("%v", e)
	}
	return nil
}

// UniqueStringSlice - Input slice, retrun slice with unique elements. Will not maintain order.
func UniqueStringSlice(elements []string) []string {
	encountered := map[string]bool{}
	result := []string{}

	for v := range elements {
		if !encountered[elements[v]] {
			encountered[elements[v]] = true
			result = append(result, elements[v])
		}
	}
	return result
}

func IsSymlink(file string) (bool, error) {
	f, err := os.Lstat(file)
	if err != nil {
		return false, err
	}
	if f.Mode()&os.ModeSymlink != 0 {
		return true, nil
	}
	return false, nil
}

func GetTagMajorVersion(tag string) string {
	splitTag := strings.Split(tag, ".")
	if len(splitTag) < 2 {
		return ""
	}
	return strings.Join(splitTag[:2], ".")
}

func IsFileExists(filePath string) (bool, error) {
	if _, err := os.Stat(filePath); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}

func GetDefaultRKETools(image string) (string, error) {
	// don't override tag of custom system images
	if !strings.Contains(image, "rancher/rke-tools") {
		return image, nil
	}
	tag, err := GetImageTagFromImage(image)
	if err != nil || tag == "" {
		return "", fmt.Errorf("defaultRKETools: no tag %s", image)
	}
	defaultImage := metadata.K8sVersionToRKESystemImages[metadata.DefaultK8sVersion].Alpine
	toReplaceTag, err := GetImageTagFromImage(defaultImage)
	if err != nil || toReplaceTag == "" {
		return "", fmt.Errorf("defaultRKETools: no replace tag %s", defaultImage)
	}
	image = strings.Replace(image, tag, toReplaceTag, 1)
	return image, nil
}

func GetImageTagFromImage(image string) (string, error) {
	parsedImage, err := ref.ParseNormalizedNamed(image)
	if err != nil {
		return "", err
	}
	imageTag := parsedImage.(ref.Tagged).Tag()
	logrus.Debugf("Extracted version [%s] from image [%s]", imageTag, image)
	return imageTag, nil
}

func StripPasswordFromURL(URL string) (string, error) {
	u, err := url.Parse(URL)
	if err != nil {
		return "", err
	}
	_, passSet := u.User.Password()
	if passSet {
		return strings.Replace(u.String(), u.User.String()+"@", u.User.Username()+":***@", 1), nil
	}
	return u.String(), nil
}

// GetEnvVar will lookup a given environment variable by key and return the key and value (to show what case got matched) with uppercase key being preferred
func GetEnvVar(key string) (string, string, bool) {
	// Uppercase (has precedence over lowercase)
	if value, ok := os.LookupEnv(strings.ToUpper(key)); ok {
		return strings.ToUpper(key), value, true
	}
	// Lowercase
	if value, ok := os.LookupEnv(strings.ToLower(key)); ok {
		return strings.ToLower(key), value, true
	}
	return "", "", false
}

func PrintProxyEnvVars() {
	// Print proxy related environment variables
	for _, proxyEnvVar := range ProxyEnvVars {
		var err error
		// Lookup environment variable
		if key, value, ok := GetEnvVar(proxyEnvVar); ok {
			// If it can contain a password, strip it (HTTP_PROXY or HTTPS_PROXY)
			if strings.HasPrefix(strings.ToUpper(proxyEnvVar), "HTTP") {
				value, err = StripPasswordFromURL(value)
				if err != nil {
					// Don't error out of provisioning when parsing of environment variable fails
					logrus.Warnf("Error parsing proxy environment variable %s", key)
					continue
				}
			}
			logrus.Infof("Using proxy environment variable %s with value [%s]", key, value)
		}
	}
}

func CleanWindowsPath(s string) string {
	// clean backslashes added from encoding
	var new []string

	// squash multi backslashes
	sp := strings.Split(s, "\\")
	for _, v := range sp {
		if v != "" {
			new = append(new, v)
		}
	}

	// drive letter only, add a trailing slash
	if len(new) == 1 {
		new = append(new, "")
	}
	return strings.Join(new, "\\")
}

func ReplaceFileWithBackup(originalFile, prefixBackupFile string) error {
	fileExists, err := IsFileExists(originalFile)
	if err != nil {
		return err
	}
	if !fileExists {
		return nil
	}
	tmpfile, err := os.CreateTemp(".", prefixBackupFile)
	if err != nil {
		return err
	}
	err = os.Rename(originalFile, tmpfile.Name())
	if err != nil {
		return err
	}
	logrus.Infof("Moved file [%s] to new location [%s] as back-up", originalFile, tmpfile.Name())
	return nil
}

func CopyFileWithPrefix(originalFile, prefixDestFile string) error {
	fileExists, err := IsFileExists(originalFile)
	if err != nil {
		return err
	}
	if !fileExists {
		return nil
	}

	sourceFileStat, err := os.Stat(originalFile)
	if err != nil {
		return err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", originalFile)
	}

	source, err := os.Open(originalFile)
	if err != nil {
		return err
	}
	defer source.Close()

	destFile, err := os.CreateTemp(".", prefixDestFile)
	if err != nil {
		return err
	}

	destination, err := os.Create(destFile.Name())
	if err != nil {
		return err
	}
	defer destination.Close()
	_, err = io.Copy(destination, source)
	if err != nil {
		return err
	}
	logrus.Infof("Copied file [%s] to new location [%s] as back-up", originalFile, destFile.Name())
	return nil
}
