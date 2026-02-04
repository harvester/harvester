package io

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

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
// If maxDepth is greater than 0, it limits the search to the specified depth.
// It returns a slice of filePaths and an error if any.
func FindFiles(directory, fileName string, maxDepth int) (filePaths []string, err error) {
	err = filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// If the directory contains symbolic links, it might lead to a non-existing file error.
			// Ignore this error and continue walking the directory.
			logrus.WithError(err).Warn("Encountered error while searching for files")
			return nil
		}

		// Calculate the depth of the directory
		depth := strings.Count(strings.TrimPrefix(path, directory), string(os.PathSeparator))

		// Skip the directory if it exceeds the maximum depth
		if maxDepth > 0 && depth > maxDepth {
			if info.IsDir() {
				return filepath.SkipDir // Skip this directory
			}

			return nil // Skip this file
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

	var statfs unix.Statfs_t
	if err := unix.Statfs(path, &statfs); err != nil {
		return diskStat, err
	}

	usage, err := disk.Usage(path)
	if err != nil {
		return diskStat, err
	}

	// Convert the FSID components to a single uint64 FSID value
	var fsidValue uint64
	for _, component := range statfs.Fsid.Val {
		// Combine components using bit manipulation
		fsidValue = (fsidValue << 32) | uint64(uint32(component))
	}

	// Format the FSID value with leading zeros
	fsidFormatted := fmt.Sprintf("%012x", fsidValue)

	return types.DiskStat{
		DiskID:           fsidFormatted,
		Path:             path,
		Type:             usage.Fstype,
		Driver:           types.DiskDriverNone,
		FreeBlocks:       int64(statfs.Bfree),
		TotalBlocks:      int64(statfs.Blocks),
		BlockSize:        int64(statfs.Bsize),
		StorageMaximum:   int64(statfs.Blocks) * int64(statfs.Bsize),
		StorageAvailable: int64(statfs.Bfree) * int64(statfs.Bsize),
	}, nil
}

// ListOpenFiles returns a list of open files in the specified directory.
func ListOpenFiles(procDirectory, directory string) ([]string, error) {
	// Check if the specified directory exists
	if _, err := os.Stat(directory); err != nil {
		return nil, err
	}

	// Get the list of all processes in the provided procDirectory
	procs, err := os.ReadDir(procDirectory)
	if err != nil {
		return nil, err
	}

	// Iterate over each process in the procDirectory
	var openedFiles []string
	for _, proc := range procs {
		// Skip non-directory entries
		if !proc.IsDir() {
			continue
		}

		// Read the file descriptor directory for the process
		pid := proc.Name()
		fdDir := filepath.Join(procDirectory, pid, "fd")
		files, err := os.ReadDir(fdDir)
		if err != nil {
			logrus.WithError(err).Tracef("Failed to read file descriptors for process %v", pid)
			continue
		}

		// Iterate over each file in the file descriptor directory
		for _, file := range files {
			filePath, err := os.Readlink(filepath.Join(fdDir, file.Name()))
			if err != nil {
				logrus.WithError(err).Tracef("Failed to read link for file descriptor %v", file.Name())
				continue
			}

			// Check if the file path is within the specified directory
			if strings.HasPrefix(filePath, directory+"/") || filePath == directory {
				openedFiles = append(openedFiles, filePath)
			}
		}
	}

	return openedFiles, nil
}

// IsDirectoryEmpty returns true if the specified directory is empty.
func IsDirectoryEmpty(directory string) (bool, error) {
	f, err := os.Open(directory)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	return false, nil
}

// CheckIsFileSizeSame verifies if all files in the provided paths have the same
// apparent and actual size.
// It returns an error if any file is a directory, does not exist, or has a different size.
func CheckIsFileSizeSame(paths ...string) error {
	referenceInfo, err := os.Stat(paths[0])
	if err != nil {
		return err
	}

	if referenceInfo.IsDir() {
		return errors.Errorf("file %v is a directory", paths[0])
	}

	referenceApparentSize := referenceInfo.Size()

	referenceActualSize, err := getFileBlockSizeEstimate(paths[0])
	if err != nil {
		return err
	}

	for _, path := range paths {
		fileInfo, err := os.Stat(path)
		if err != nil {
			return err
		}

		if fileInfo.IsDir() {
			return errors.Errorf("file %v is a directory", path)

		}

		if fileInfo.Size() != referenceApparentSize {
			return errors.Errorf("file %v apparent size %v is not equal to %v", path, fileInfo.Size(), referenceApparentSize)
		}

		actualSize, err := getFileBlockSizeEstimate(path)
		if err != nil {
			return err
		}

		if actualSize != referenceActualSize {
			return errors.Errorf("file %v actual size %v is not equal to %v", path, actualSize, referenceActualSize)
		}
	}

	return nil
}

func getFileBlockSizeEstimate(path string) (uint64, error) {
	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		return 0, err
	}

	return uint64(stat.Blocks) * uint64(stat.Blksize), nil
}
