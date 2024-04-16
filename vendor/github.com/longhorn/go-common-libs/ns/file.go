package ns

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/longhorn/go-common-libs/io"
	"github.com/longhorn/go-common-libs/types"
)

// CopyDirectory switches to the host namespace and copies the content from
// source to destination. It will overwrite the destination if overWrite is true.
// Top level directory is prohibited.
func CopyDirectory(source, destination string, overWrite bool) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to copy host content from %v to %v", source, destination)
	}()

	srcDir, err := filepath.Abs(filepath.Clean(source))
	if err != nil {
		return err
	}

	dstDir, err := filepath.Abs(filepath.Clean(destination))
	if err != nil {
		return err
	}

	if strings.Count(srcDir, "/") < 2 || strings.Count(dstDir, "/") < 2 {
		return errors.Errorf("prohibit copying the content for the top level of directory %v or %v", srcDir, dstDir)
	}

	fn := func() (interface{}, error) {
		return "", io.CopyFiles(source, destination, overWrite)
	}

	_, err = RunFunc(fn, 0)
	return err
}

// CreateDirectory switches to the host namespace and creates a directory at
// the specified path.
func CreateDirectory(path string, modTime time.Time) (result string, err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to create directory %s", path)
	}()

	fn := func() (interface{}, error) {
		return io.CreateDirectory(path, modTime)
	}

	rawResult, err := RunFunc(fn, 0)
	if err != nil {
		return "", err
	}

	var ableToCast bool
	result, ableToCast = rawResult.(string)
	if !ableToCast {
		return "", errors.Errorf(types.ErrNamespaceCastResultFmt, result, rawResult)
	}
	return result, nil
}

// DeleteDirectory switches to the host namespace and removes the directory
// at the specified path.
func DeleteDirectory(directory string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to remove host directory %v", directory)
	}()

	dir, err := filepath.Abs(filepath.Clean(directory))
	if err != nil {
		return err
	}

	if strings.Count(dir, "/") < 2 {
		return errors.Errorf("prohibit removing the top level of directory %v", dir)
	}

	fn := func() (interface{}, error) {
		if _, err := os.Stat(dir); err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}

		return nil, os.RemoveAll(dir)
	}

	_, err = RunFunc(fn, 0)
	return err
}

// ReadDirectory switches to the host namespace and reads the content of the
// directory at the specified path.
func ReadDirectory(directory string) (result []fs.DirEntry, err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to read directory %s", directory)
	}()

	fn := func() (interface{}, error) {
		return os.ReadDir(directory)
	}

	rawResult, err := RunFunc(fn, 0)
	if err != nil {
		return nil, err
	}

	var ableToCast bool
	result, ableToCast = rawResult.([]fs.DirEntry)
	if !ableToCast {
		return nil, errors.Errorf(types.ErrNamespaceCastResultFmt, result, rawResult)
	}
	return result, nil
}

// CopyFiles switches to the host namespace and copies the all files from
// source to destination. It will overwrite the destination if overWrite is true.
func CopyFiles(sourcePath, destinationPath string, doOverWrite bool) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to copy files from %s to %s", sourcePath, destinationPath)
	}()

	fn := func() (interface{}, error) {
		return "", io.CopyFiles(sourcePath, destinationPath, doOverWrite)
	}

	_, err = RunFunc(fn, 0)
	return err
}

// GetEmptyFiles switches to the host namespace and retrieves a list
// of paths for all empty files within the specified directory.
func GetEmptyFiles(directory string) (result []string, err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to get empty files in %s", directory)
	}()

	fn := func() (interface{}, error) {
		return io.GetEmptyFiles(directory)
	}

	rawResult, err := RunFunc(fn, 0)
	if err != nil {
		return nil, err
	}

	var ableToCast bool
	result, ableToCast = rawResult.([]string)
	if !ableToCast {
		return nil, errors.Errorf(types.ErrNamespaceCastResultFmt, result, rawResult)
	}
	return result, nil
}

// GetFileInfo switches to the host namespace and returns the file info of
// the file at the specified path.
func GetFileInfo(path string) (result fs.FileInfo, err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to get file info of %s", path)
	}()

	fn := func() (interface{}, error) {
		return os.Stat(path)
	}

	rawResult, err := RunFunc(fn, 0)
	if err != nil {
		return nil, err
	}

	var ableToCast bool
	result, ableToCast = rawResult.(fs.FileInfo)
	if !ableToCast {
		return nil, errors.Errorf(types.ErrNamespaceCastResultFmt, result, rawResult)
	}
	return result, nil
}

// ReadFileContent switches to the host namespace and returns the content of
// the file at the specified path.
func ReadFileContent(filePath string) (result string, err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to read file content of %s", filePath)
	}()

	fn := func() (interface{}, error) {
		return io.ReadFileContent(filePath)
	}

	rawResult, err := RunFunc(fn, 0)
	if err != nil {
		return "", err
	}

	var ableToCast bool
	result, ableToCast = rawResult.(string)
	if !ableToCast {
		return "", errors.Errorf(types.ErrNamespaceCastResultFmt, result, rawResult)
	}
	return result, nil
}

// SyncFile switches to the host namespace and syncs the file at the
// specified path.
func SyncFile(filePath string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to sync file %s", filePath)
	}()

	fn := func() (interface{}, error) {
		return nil, io.SyncFile(filePath)
	}

	_, err = RunFunc(fn, 0)
	return err
}

// WriteFile switches to the host namespace and writes the data to the file
// at the specified path.
func WriteFile(filePath, data string) error {
	var err error
	defer func() {
		err = errors.Wrapf(err, "failed to write file %s", filePath)
	}()

	fn := func() (interface{}, error) {
		return "", os.WriteFile(filePath, []byte(data), 0644)
	}

	_, err = RunFunc(fn, 0)
	return err
}

// DeletePath switches to the host namespace and removes the file or
// directory at the specified path.
func DeletePath(path string) error {
	var err error
	defer func() {
		err = errors.Wrapf(err, "failed to delete path %s", path)
	}()

	fn := func() (interface{}, error) {
		return "", os.RemoveAll(path)
	}

	_, err = RunFunc(fn, 0)
	return err
}

// GetDiskStat switches to the host namespace and returns the disk stat
// of the disk at the specified path.
func GetDiskStat(path string) (*types.DiskStat, error) {
	var err error
	defer func() {
		err = errors.Wrapf(err, "failed to get disk stat %s", path)
	}()

	fn := func() (interface{}, error) {
		return io.GetDiskStat(path)
	}

	rawResult, err := RunFunc(fn, 0)
	if err != nil {
		return nil, err
	}

	var ableToCast bool
	result, ableToCast := rawResult.(types.DiskStat)
	if !ableToCast {
		return nil, errors.Errorf(types.ErrNamespaceCastResultFmt, result, rawResult)
	}
	return &result, nil
}
