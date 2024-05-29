package gocommon

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const (
	defaultFilePerm = 0644
	defaultTempDir  = "/tmp"
)

// Generate Temp file with YAML content, return the file name
func GenerateYAMLTempFile(obj interface{}, prefix string) (string, error) {
	return GenerateYAMLTempFileFullOptions(obj, prefix, defaultTempDir, defaultFilePerm)
}

func GenerateYAMLTempFileWithPerm(obj interface{}, prefix string, perm fs.FileMode) (string, error) {
	return GenerateYAMLTempFileFullOptions(obj, prefix, defaultTempDir, perm)
}

func GenerateYAMLTempFileWithDir(obj interface{}, prefix, dirPath string) (string, error) {
	return GenerateYAMLTempFileFullOptions(obj, prefix, dirPath, defaultFilePerm)
}

// Generate Temp file with YAML content and permission, return the file name
func GenerateYAMLTempFileFullOptions(obj interface{}, prefix, dirPath string, perm fs.FileMode) (string, error) {
	tempFile, err := os.CreateTemp(dirPath, prefix)
	if err != nil {
		logrus.Errorf("Create temp file failed. err: %v", err)
		return "", err
	}
	defer tempFile.Close()

	if err = tempFile.Chmod(perm); err != nil {
		logrus.Errorf("Chmod temp file failed. err: %v", err)
		return "", err
	}

	bytes, err := yaml.Marshal(obj)
	if err != nil {
		logrus.Errorf("Generate YAML content failed. err: %v", err)
		return "", err
	}
	if _, err := tempFile.Write(bytes); err != nil {
		logrus.Errorf("Write YAML content to file failed. err: %v", err)
		return "", err
	}

	logrus.Debugf("Content of %s: %s", tempFile.Name(), string(bytes))

	return tempFile.Name(), nil
}

// Generate Temp file with buffer, return the file name
func GenerateTempFile(buf []byte, prefix string) (string, error) {
	return GenerateTempFileFullOptions(buf, prefix, defaultTempDir, defaultFilePerm)
}

// Generate Temp file with buffer and permission, return the file name
func GenerateTempFileWithPerm(buf []byte, prefix string, perm fs.FileMode) (string, error) {
	return GenerateTempFileFullOptions(buf, prefix, defaultTempDir, perm)
}

// Generate Temp file with buffer and directory, return the fullpath
func GenerateTempFileWithDir(buf []byte, prefix, dirPath string) (string, error) {
	return GenerateTempFileFullOptions(buf, prefix, dirPath, defaultFilePerm)
}

// Generate Temp file with buffer, directory and permission, return the file name
func GenerateTempFileFullOptions(buf []byte, prefix, dirPath string, perm fs.FileMode) (string, error) {
	tempFile, err := os.CreateTemp(dirPath, prefix)
	if err != nil {
		logrus.Errorf("Create temp file failed. err: %v", err)
		return "", err
	}
	defer tempFile.Close()

	if err = tempFile.Chmod(perm); err != nil {
		logrus.Errorf("Chmod temp file failed. err: %v", err)
		return "", err
	}

	if _, err := tempFile.Write(buf); err != nil {
		logrus.Errorf("Write buf to file failed. err: %v", err)
		return "", err
	}

	logrus.Debugf("Content of %s: %s", tempFile.Name(), string(buf))

	return tempFile.Name(), nil
}

func BackupFile(source string) (string, error) {
	return BackupFileToDirWithSuffix(source, "", "bak")
}

func BackupFileToDir(sourcePath, dstDir string) (string, error) {
	return BackupFileToDirWithSuffix(sourcePath, dstDir, "bak")
}

func BackupFileToDirWithSuffix(sourcePath, dstDir, suffix string) (string, error) {
	srcStat, err := os.Stat(sourcePath)
	if err != nil {
		logrus.Errorf("Stat file failed. err: %v", err)
		return "", err
	}

	if !srcStat.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", sourcePath)
	}

	src, err := os.Open(sourcePath)
	if err != nil {
		logrus.Errorf("Open file failed. err: %v", err)
		return "", err
	}
	defer src.Close()

	// if disDir is nil, keep the backup file in the same directory
	dstPath := fmt.Sprintf("%s.%s", sourcePath, suffix)
	if dstDir != "" {
		sourceFileName := GetFileName(sourcePath)
		dstPath = fmt.Sprintf("%s/%s.%s", dstDir, sourceFileName, suffix)
	}

	dst, err := os.Create(dstPath)
	if err != nil {
		logrus.Errorf("Create NTP config backup file failed. err: %v", err)
		return "", err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		logrus.Errorf("Copy file failed. err: %v", err)
		return "", err
	}
	return dstPath, nil
}

func RemoveFiles(path ...string) error {
	for _, target := range path {
		if err := os.Remove(target); err != nil {
			logrus.Errorf("Remove file %s failed. err: %v", target, err)
			return err
		}
	}
	return nil
}

func GetFileName(path string) string {
	return path[strings.LastIndex(path, "/")+1:]
}
