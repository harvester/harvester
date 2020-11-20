package system

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/docker/docker/pkg/mount"
	"github.com/otiai10/copy"
	"github.com/sirupsen/logrus"
)

type VersionName string

const (
	VersionCurrent  VersionName = "current"
	VersionPrevious VersionName = "previous"
)

// StatComponentVersion will de-reference the version symlink for the component.
func StatComponentVersion(root, key string, alias VersionName) (os.FileInfo, error) {
	aliasPath := filepath.Join(root, key, string(alias))
	aliasInfo, err := os.Stat(aliasPath)
	if err != nil {
		return nil, err
	}
	if !aliasInfo.IsDir() {
		return nil, fmt.Errorf("stat %s: not a directory", aliasPath)
	}
	version, err := os.Readlink(aliasPath)
	if err != nil {
		return nil, err
	}
	versionPath := filepath.Join(root, key, version)
	versionInfo, err := os.Stat(versionPath)
	if err != nil {
		return nil, err
	}
	if !versionInfo.IsDir() {
		return versionInfo, fmt.Errorf("stat %s: not a directory", versionPath)
	}
	return versionInfo, nil
}

// CopyComponent will copy the component identified by `key` from `src` to `dst`, moving the `current` symlink to the
// version from `src` (after renaming `current` to `previous`).
func CopyComponent(src, dst string, remount bool, key string) (bool, error) {
	srcInfo, err := StatComponentVersion(src, key, VersionCurrent)
	if err != nil {
		return false, err
	}
	dstInfo, _ := StatComponentVersion(dst, key, VersionCurrent)
	if dstInfo != nil && dstInfo.Name() == srcInfo.Name() {
		logrus.Infof("skipping %q because destination version matches source: %s", key, dstInfo.Name())
		return false, nil
	}
	if remount {
		if err := mount.Mount("", dst, "none", "remount,rw"); err != nil {
			return false, err
		}
	}

	srcPath := filepath.Join(src, key, srcInfo.Name())
	dstPath := filepath.Join(dst, key, srcInfo.Name())
	dstPrevPath := filepath.Join(dst, key, string(VersionPrevious))
	dstCurrPath := filepath.Join(dst, key, string(VersionCurrent))

	dstCurrTemp := dstCurrPath + `.tmp`
	if err := os.Symlink(filepath.Base(dstPath), dstCurrTemp); err != nil {
		return false, err
	}
	logrus.Debugf("created symlink: %v", dstCurrTemp)
	defer os.Remove(dstCurrTemp) // if this fails, that means it's gone which is correct

	dstTemp, err := ioutil.TempDir(filepath.Split(dstPath))
	if err != nil {
		return false, err
	}
	logrus.Debugf("created temporary dir: %v", dstTemp)
	defer os.RemoveAll(dstTemp) // if this fails, that means it's gone which is correct

	logrus.Debugf("copying: %v -> %v", srcPath, dstTemp)
	if err := copy.Copy(srcPath, dstTemp); err != nil {
		return false, err
	}

	// if the destination already exists, attempt to move it out of the way
	if dstInfo, err := os.Stat(dstPath); err == nil {
		if dstInfo.IsDir() {
			dstExist := dstPath + `.old`
			if err = os.Rename(dstPath, dstExist); err != nil {
				return false, err
			}
			defer os.Rename(dstExist, dstPath) // if this fails then the destination was replaced, all good
		} else {
			os.Remove(dstPath) // if this fails then the rename below will fail which is the desired behavior
		}
	}

	logrus.Debugf("renaming: %v -> %v", dstTemp, dstPath)
	if err := os.Rename(dstTemp, dstPath); err != nil {
		return false, err
	}

	logrus.Debugf("chmod %s %s", srcInfo.Mode(), dstPath)
	if err := os.Chmod(dstPath, srcInfo.Mode()); err != nil {
		logrus.Error(err)
	}

	logrus.Debugf("removing: %v", dstPrevPath)
	if err := os.Remove(dstPrevPath); err != nil {
		logrus.Warn(err)
	}

	logrus.Debugf("copying: %v -> %v", dstCurrPath, dstPrevPath)
	if err := copy.Copy(dstCurrPath, dstPrevPath); err != nil {
		logrus.Error(err)
	}

	logrus.Debugf("renaming: %v -> %v", dstCurrTemp, dstCurrPath)
	if err := os.Rename(dstCurrTemp, dstCurrPath); err != nil {
		return false, err
	}

	return true, nil
}
