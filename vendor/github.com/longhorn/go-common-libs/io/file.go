package io

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/sirupsen/logrus"

	"github.com/longhorn/go-common-libs/types"
)

// CreateDirectory creates a directory at the specified path, and sets the modification time.
func CreateDirectory(path string, modTime time.Time) (string, error) {
	// Check if the directory already exists
	if fileInfo, err := os.Stat(path); err == nil && fileInfo.IsDir() {
		return filepath.Abs(path)
	}

	// Create the directory and its parent directories if they don't exist.
	err := os.MkdirAll(path, 0755)
	if err != nil {
		return "", err
	}

	// Set the modification time of the directory.
	err = os.Chtimes(path, modTime, modTime)
	if err != nil {
		return "", errors.Wrapf(err, "failed to set the modification time for %v", path)
	}

	// Return the absolute path of the directory.
	return filepath.Abs(path)
}

// CopyDirectory copies the directory from source to destination.
func CopyDirectory(sourcePath, destinationPath string, doOverWrite bool) error {
	var err error
	defer func() {
		err = errors.Wrapf(err, "failed to copy directory %v to %v", sourcePath, destinationPath)
	}()

	sourcePathInfo, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}

	destinationAbsPath, err := CreateDirectory(destinationPath, sourcePathInfo.ModTime())
	if err != nil {
		return err
	}

	return CopyFiles(sourcePath, destinationAbsPath, doOverWrite)
}

// CopyFiles copies the files from source to destination.
func CopyFiles(sourcePath, destinationPath string, doOverWrite bool) error {
	srcFileInfo, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}

	if !srcFileInfo.IsDir() {
		return CopyFile(sourcePath, destinationPath, doOverWrite)
	}

	srcFileInfos, err := os.ReadDir(sourcePath)
	if err != nil {
		return errors.Wrapf(err, "failed to read source directory %v", sourcePath)
	}

	for _, srcFileInfo := range srcFileInfos {
		srcFileInfo, err := srcFileInfo.Info()
		if err != nil {
			return err
		}

		dstFilePath := filepath.Join(destinationPath, srcFileInfo.Name())
		srcFilePath := filepath.Join(sourcePath, srcFileInfo.Name())

		if srcFileInfo.IsDir() {
			if err := CopyDirectory(srcFilePath, dstFilePath, doOverWrite); err != nil {
				return err
			}
		} else {
			if err := CopyFile(srcFilePath, dstFilePath, doOverWrite); err != nil {
				return err
			}
		}
	}

	return nil
}

// CopyFile copies the file from source to destination.
func CopyFile(sourcePath, destinationPath string, overWrite bool) error {
	var err error
	defer func() {
		err = errors.Wrapf(err, "failed to copy file %v to %v", sourcePath, destinationPath)
	}()

	sourceFileInfo, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}

	if !overWrite {
		if _, err := os.Stat(destinationPath); err == nil {
			logrus.Warnf("destination file %v already exists", destinationPath)
			return nil
		}
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	_, err = CreateDirectory(filepath.Dir(destinationPath), sourceFileInfo.ModTime())
	if err != nil {
		return err
	}

	destinationFile, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return err
	}

	// Set the modification time of the file.
	sourceFileModTime := sourceFileInfo.ModTime()
	return os.Chtimes(destinationPath, sourceFileModTime, sourceFileModTime)
}

// GetEmptyFiles retrieves a list of paths for all empty files within the specified directory.
// It uses filepath.Walk to traverse the directory and finds files with zero size.
// It returns a slice of filePaths and an error if any.
func GetEmptyFiles(directory string) (filePaths []string, err error) {
	err = filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.Wrapf(err, "failed to walk directory %v", directory)
		}
		if info.IsDir() {
			return nil
		}
		if info.Size() == 0 {
			filePaths = append(filePaths, path)
		}
		return nil
	})
	return filePaths, errors.Wrapf(err, "failed to get empty files in %s", directory)
}

// FindFiles searches for files in the specified directory with the given fileName.
// If fileName is empty, it retrieves all files in the directory.
// It returns a slice of filePaths and an error if any.
func FindFiles(directory, fileName string) (filePaths []string, err error) {
	err = filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fileName == "" || info.Name() == fileName {
			filePaths = append(filePaths, path)
		}
		return nil
	})
	return filePaths, errors.Wrapf(err, "failed to find files in %s", directory)
}

// ReadFileContent reads the content of the file.
func ReadFileContent(filePath string) (string, error) {
	if _, err := os.Stat(filePath); err != nil {
		return "", errors.Wrapf(err, "cannot find file %v", filePath)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// SyncFile syncs the file to the disk.
func SyncFile(filePath string) error {
	file, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	return file.Sync()
}

// GetDiskStat returns the disk stat for the specified path.
func GetDiskStat(path string) (diskStat types.DiskStat, err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to get fs stat for %v", path)
	}()

	var statfs syscall.Statfs_t
	if err := syscall.Statfs(path, &statfs); err != nil {
		return diskStat, err
	}

	usage, err := disk.Usage(path)
	if err != nil {
		return diskStat, err
	}

	// Convert the FSID components to a single uint64 FSID value
	var fsidValue uint64
	for _, component := range statfs.Fsid.X__val {
		// Combine components using bit manipulation
		fsidValue = (fsidValue << 32) | uint64(uint32(component))
	}

	// Format the FSID value with leading zeros
	fsidFormatted := fmt.Sprintf("%012x", fsidValue)

	return types.DiskStat{
		DiskID:           fsidFormatted,
		Path:             path,
		Type:             usage.Fstype,
		FreeBlocks:       int64(statfs.Bfree),
		TotalBlocks:      int64(statfs.Blocks),
		BlockSize:        statfs.Bsize,
		StorageMaximum:   int64(statfs.Blocks) * statfs.Bsize,
		StorageAvailable: int64(statfs.Bfree) * statfs.Bsize,
	}, nil
}
