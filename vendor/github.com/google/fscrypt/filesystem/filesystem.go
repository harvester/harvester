/*
 * filesystem.go - Contains the functionality for a specific filesystem. This
 * includes the commands to setup the filesystem, apply policies, and locate
 * metadata.
 *
 * Copyright 2017 Google Inc.
 * Author: Joe Richey (joerichey@google.com)
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not
 * use this file except in compliance with the License. You may obtain a copy of
 * the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 * License for the specific language governing permissions and limitations under
 * the License.
 */

// Package filesystem deals with the structure of the files on disk used to
// store the metadata for fscrypt. Specifically, this package includes:
//	- mountpoint management (mountpoint.go)
//		- querying existing mounted filesystems
//		- getting filesystems from a UUID
//		- finding the filesystem for a specific path
//	- metadata organization (filesystem.go)
//		- setting up a mounted filesystem for use with fscrypt
//		- adding/querying/deleting metadata
//		- making links to other filesystems' metadata
//		- following links to get data from other filesystems
package filesystem

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/google/fscrypt/metadata"
	"github.com/google/fscrypt/util"
)

// ErrAlreadySetup indicates that a filesystem is already setup for fscrypt.
type ErrAlreadySetup struct {
	Mount *Mount
}

func (err *ErrAlreadySetup) Error() string {
	return fmt.Sprintf("filesystem %s is already setup for use with fscrypt",
		err.Mount.Path)
}

// ErrCorruptMetadata indicates that an fscrypt metadata file is corrupt.
type ErrCorruptMetadata struct {
	Path            string
	UnderlyingError error
}

func (err *ErrCorruptMetadata) Error() string {
	return fmt.Sprintf("fscrypt metadata file at %q is corrupt: %s",
		err.Path, err.UnderlyingError)
}

// ErrFollowLink indicates that a protector link can't be followed.
type ErrFollowLink struct {
	Link            string
	UnderlyingError error
}

func (err *ErrFollowLink) Error() string {
	return fmt.Sprintf("cannot follow filesystem link %q: %s",
		err.Link, err.UnderlyingError)
}

// ErrInsecurePermissions indicates that a filesystem is not considered to be
// setup for fscrypt because a metadata directory has insecure permissions.
type ErrInsecurePermissions struct {
	Path string
}

func (err *ErrInsecurePermissions) Error() string {
	return fmt.Sprintf("%q has insecure permissions (world-writable without sticky bit)",
		err.Path)
}

// ErrMakeLink indicates that a protector link can't be created.
type ErrMakeLink struct {
	Target          *Mount
	UnderlyingError error
}

func (err *ErrMakeLink) Error() string {
	return fmt.Sprintf("cannot create filesystem link to %q: %s",
		err.Target.Path, err.UnderlyingError)
}

// ErrMountOwnedByAnotherUser indicates that the mountpoint root directory is
// owned by a user that isn't trusted in the current context, so we don't
// consider fscrypt to be properly setup on the filesystem.
type ErrMountOwnedByAnotherUser struct {
	Mount *Mount
}

func (err *ErrMountOwnedByAnotherUser) Error() string {
	return fmt.Sprintf("another non-root user owns the root directory of %s", err.Mount.Path)
}

// ErrNoCreatePermission indicates that the current user lacks permission to
// create fscrypt metadata on the given filesystem.
type ErrNoCreatePermission struct {
	Mount *Mount
}

func (err *ErrNoCreatePermission) Error() string {
	return fmt.Sprintf("user lacks permission to create fscrypt metadata on %s", err.Mount.Path)
}

// ErrNotAMountpoint indicates that a path is not a mountpoint.
type ErrNotAMountpoint struct {
	Path string
}

func (err *ErrNotAMountpoint) Error() string {
	return fmt.Sprintf("%q is not a mountpoint", err.Path)
}

// ErrNotSetup indicates that a filesystem is not setup for fscrypt.
type ErrNotSetup struct {
	Mount *Mount
}

func (err *ErrNotSetup) Error() string {
	return fmt.Sprintf("filesystem %s is not setup for use with fscrypt", err.Mount.Path)
}

// ErrSetupByAnotherUser indicates that one or more of the fscrypt metadata
// directories is owned by a user that isn't trusted in the current context, so
// we don't consider fscrypt to be properly setup on the filesystem.
type ErrSetupByAnotherUser struct {
	Mount *Mount
}

func (err *ErrSetupByAnotherUser) Error() string {
	return fmt.Sprintf("another non-root user owns fscrypt metadata directories on %s", err.Mount.Path)
}

// ErrSetupNotSupported indicates that the given filesystem type is not
// supported for fscrypt setup.
type ErrSetupNotSupported struct {
	Mount *Mount
}

func (err *ErrSetupNotSupported) Error() string {
	return fmt.Sprintf("filesystem type %s is not supported for fscrypt setup",
		err.Mount.FilesystemType)
}

// ErrPolicyNotFound indicates that the policy metadata was not found.
type ErrPolicyNotFound struct {
	Descriptor string
	Mount      *Mount
}

func (err *ErrPolicyNotFound) Error() string {
	return fmt.Sprintf("policy metadata for %s not found on filesystem %s",
		err.Descriptor, err.Mount.Path)
}

// ErrProtectorNotFound indicates that the protector metadata was not found.
type ErrProtectorNotFound struct {
	Descriptor string
	Mount      *Mount
}

func (err *ErrProtectorNotFound) Error() string {
	return fmt.Sprintf("protector metadata for %s not found on filesystem %s",
		err.Descriptor, err.Mount.Path)
}

// SortDescriptorsByLastMtime indicates whether descriptors are sorted by last
// modification time when being listed.  This can be set to true to get
// consistent output for testing.
var SortDescriptorsByLastMtime = false

// Mount contains information for a specific mounted filesystem.
//	Path           - Absolute path where the directory is mounted
//	FilesystemType - Type of the mounted filesystem, e.g. "ext4"
//	Device         - Device for filesystem (empty string if we cannot find one)
//	DeviceNumber   - Device number of the filesystem.  This is set even if
//			 Device isn't, since all filesystems have a device
//			 number assigned by the kernel, even pseudo-filesystems.
//	Subtree        - The mounted subtree of the filesystem.  This is usually
//			 "/", meaning that the entire filesystem is mounted, but
//			 it can differ for bind mounts.
//	ReadOnly       - True if this is a read-only mount
//
// In order to use a Mount to store fscrypt metadata, some directories must be
// setup first. Specifically, the directories created look like:
// <mountpoint>
// └── .fscrypt
//     ├── policies
//     └── protectors
//
// These "policies" and "protectors" directories will contain files that are
// the corresponding metadata structures for policies and protectors. The public
// interface includes functions for setting up these directories and Adding,
// Getting, and Removing these files.
//
// There is also the ability to reference another filesystem's metadata. This is
// used when a Policy on filesystem A is protected with Protector on filesystem
// B. In this scenario, we store a "link file" in the protectors directory.
//
// We also allow ".fscrypt" to be a symlink which was previously created. This
// allows login protectors to be created when the root filesystem is read-only,
// provided that "/.fscrypt" is a symlink pointing to a writable location.
type Mount struct {
	Path           string
	FilesystemType string
	Device         string
	DeviceNumber   DeviceNumber
	Subtree        string
	ReadOnly       bool
}

// PathSorter allows mounts to be sorted by Path.
type PathSorter []*Mount

func (p PathSorter) Len() int           { return len(p) }
func (p PathSorter) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p PathSorter) Less(i, j int) bool { return p[i].Path < p[j].Path }

const (
	// Names of the various directories used in fscrypt
	baseDirName       = ".fscrypt"
	policyDirName     = "policies"
	protectorDirName  = "protectors"
	tempPrefix        = ".tmp"
	linkFileExtension = ".link"

	// The base directory should be read-only (except for the creator)
	basePermissions = 0755

	// The metadata files shouldn't be readable or writable by other users.
	// Having them be world-readable wouldn't necessarily be a huge issue,
	// but given that some of these files contain (strong) password hashes,
	// we error on the side of caution -- similar to /etc/shadow.
	// Note: existing files on-disk might have mode 0644, as that was the
	// mode used by fscrypt v0.3.2 and earlier.
	filePermissions = os.FileMode(0600)

	// Maximum size of a metadata file.  This value is arbitrary, and it can
	// be changed.  We just set a reasonable limit that shouldn't be reached
	// in practice, except by users trying to cause havoc by creating
	// extremely large files in the metadata directories.
	maxMetadataFileSize = 16384
)

// SetupMode is a mode for creating the fscrypt metadata directories.
type SetupMode int

const (
	// SingleUserWritable specifies to make the fscrypt metadata directories
	// writable by a single user (usually root) only.
	SingleUserWritable SetupMode = iota
	// WorldWritable specifies to make the fscrypt metadata directories
	// world-writable (with the sticky bit set).
	WorldWritable
)

func (m *Mount) String() string {
	return fmt.Sprintf(`%s
	FilesystemType: %s
	Device:         %s`, m.Path, m.FilesystemType, m.Device)
}

// BaseDir returns the path to the base fscrypt directory for this filesystem.
func (m *Mount) BaseDir() string {
	rawBaseDir := filepath.Join(m.Path, baseDirName)
	// We allow the base directory to be a symlink, but some callers need
	// the real path, so dereference the symlink here if needed. Since the
	// directory the symlink points to may not exist yet, we have to read
	// the symlink manually rather than use filepath.EvalSymlinks.
	target, err := os.Readlink(rawBaseDir)
	if err != nil {
		return rawBaseDir // not a symlink
	}
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(m.Path, target)
}

// ProtectorDir returns the directory containing the protector metadata.
func (m *Mount) ProtectorDir() string {
	return filepath.Join(m.BaseDir(), protectorDirName)
}

// protectorPath returns the full path to a regular protector file with the
// specified descriptor.
func (m *Mount) protectorPath(descriptor string) string {
	return filepath.Join(m.ProtectorDir(), descriptor)
}

// linkedProtectorPath returns the full path to a linked protector file with the
// specified descriptor.
func (m *Mount) linkedProtectorPath(descriptor string) string {
	return m.protectorPath(descriptor) + linkFileExtension
}

// PolicyDir returns the directory containing the policy metadata.
func (m *Mount) PolicyDir() string {
	return filepath.Join(m.BaseDir(), policyDirName)
}

// PolicyPath returns the full path to a regular policy file with the
// specified descriptor.
func (m *Mount) PolicyPath(descriptor string) string {
	return filepath.Join(m.PolicyDir(), descriptor)
}

// tempMount creates a temporary directory alongside this Mount's base fscrypt
// directory and returns a temporary Mount which represents this temporary
// directory. The caller is responsible for removing this temporary directory.
func (m *Mount) tempMount() (*Mount, error) {
	tempDir, err := ioutil.TempDir(filepath.Dir(m.BaseDir()), tempPrefix)
	return &Mount{Path: tempDir}, err
}

// ErrEncryptionNotEnabled indicates that encryption is not enabled on the given
// filesystem.
type ErrEncryptionNotEnabled struct {
	Mount *Mount
}

func (err *ErrEncryptionNotEnabled) Error() string {
	return fmt.Sprintf("encryption not enabled on filesystem %s (%s).",
		err.Mount.Path, err.Mount.Device)
}

// ErrEncryptionNotSupported indicates that encryption is not supported on the
// given filesystem.
type ErrEncryptionNotSupported struct {
	Mount *Mount
}

func (err *ErrEncryptionNotSupported) Error() string {
	return fmt.Sprintf("This kernel doesn't support encryption on %s filesystems.",
		err.Mount.FilesystemType)
}

// EncryptionSupportError adds filesystem-specific context to the
// ErrEncryptionNotEnabled and ErrEncryptionNotSupported errors from the
// metadata package.
func (m *Mount) EncryptionSupportError(err error) error {
	switch err {
	case metadata.ErrEncryptionNotEnabled:
		return &ErrEncryptionNotEnabled{m}
	case metadata.ErrEncryptionNotSupported:
		return &ErrEncryptionNotSupported{m}
	}
	return err
}

// isFscryptSetupAllowed decides whether the given filesystem is allowed to be
// set up for fscrypt, without actually accessing it.  This basically checks
// whether the filesystem type is one of the types that supports encryption, or
// at least is in some stage of planning for encrption support in the future.
//
// We need this list so that we can skip filesystems that are irrelevant for
// fscrypt without having to look for the fscrypt metadata directories on them,
// which can trigger errors, long delays, or side effects on some filesystems.
//
// Unfortunately, this means that if a completely new filesystem adds encryption
// support, then it will need to be manually added to this list.  But it seems
// to be a worthwhile tradeoff to avoid the above issues.
func (m *Mount) isFscryptSetupAllowed() bool {
	if m.Path == "/" {
		// The root filesystem is always allowed, since it's where login
		// protectors are stored.
		return true
	}
	switch m.FilesystemType {
	case "ext4", "f2fs", "ubifs", "btrfs", "ceph", "xfs":
		return true
	default:
		return false
	}
}

// CheckSupport returns an error if this filesystem does not support encryption.
func (m *Mount) CheckSupport() error {
	if !m.isFscryptSetupAllowed() {
		return &ErrEncryptionNotSupported{m}
	}
	return m.EncryptionSupportError(metadata.CheckSupport(m.Path))
}

func checkOwnership(path string, info os.FileInfo, trustedUser *user.User) bool {
	if trustedUser == nil {
		return true
	}
	trustedUID := uint32(util.AtoiOrPanic(trustedUser.Uid))
	actualUID := info.Sys().(*syscall.Stat_t).Uid
	if actualUID != 0 && actualUID != trustedUID {
		log.Printf("WARNING: %q is owned by uid %d, but expected %d or 0",
			path, actualUID, trustedUID)
		return false
	}
	return true
}

// CheckSetup returns an error if any of the fscrypt metadata directories do not
// exist. Will log any unexpected errors or incorrect permissions.
func (m *Mount) CheckSetup(trustedUser *user.User) error {
	if !m.isFscryptSetupAllowed() {
		return &ErrNotSetup{m}
	}
	// Check that the mountpoint directory itself is not a symlink and has
	// proper ownership, as otherwise we can't trust anything beneath it.
	info, err := loggedLstat(m.Path)
	if err != nil {
		return &ErrNotSetup{m}
	}
	if (info.Mode() & os.ModeSymlink) != 0 {
		log.Printf("mountpoint directory %q cannot be a symlink", m.Path)
		return &ErrNotSetup{m}
	}
	if !info.IsDir() {
		log.Printf("mountpoint %q is not a directory", m.Path)
		return &ErrNotSetup{m}
	}
	if !checkOwnership(m.Path, info, trustedUser) {
		return &ErrMountOwnedByAnotherUser{m}
	}

	// Check BaseDir similarly.  However, unlike the other directories, we
	// allow BaseDir to be a symlink, to support the use case of metadata
	// for a read-only filesystem being redirected to a writable location.
	info, err = loggedStat(m.BaseDir())
	if err != nil {
		return &ErrNotSetup{m}
	}
	if !info.IsDir() {
		log.Printf("%q is not a directory", m.BaseDir())
		return &ErrNotSetup{m}
	}
	if !checkOwnership(m.Path, info, trustedUser) {
		return &ErrMountOwnedByAnotherUser{m}
	}

	// Check that the policies and protectors directories aren't symlinks and
	// have proper ownership.
	subdirs := []string{m.PolicyDir(), m.ProtectorDir()}
	for _, path := range subdirs {
		info, err := loggedLstat(path)
		if err != nil {
			return &ErrNotSetup{m}
		}
		if (info.Mode() & os.ModeSymlink) != 0 {
			log.Printf("directory %q cannot be a symlink", path)
			return &ErrNotSetup{m}
		}
		if !info.IsDir() {
			log.Printf("%q is not a directory", path)
			return &ErrNotSetup{m}
		}
		// We are no longer too picky about the mode, given that
		// 'fscrypt setup' now offers a choice of two different modes,
		// and system administrators could customize it further.
		// However, we can at least verify that if the directory is
		// world-writable, then the sticky bit is also set.
		if info.Mode()&(os.ModeSticky|0002) == 0002 {
			log.Printf("%q is world-writable but doesn't have sticky bit set", path)
			return &ErrInsecurePermissions{path}
		}
		if !checkOwnership(path, info, trustedUser) {
			return &ErrSetupByAnotherUser{m}
		}
	}
	return nil
}

// makeDirectories creates the three metadata directories with the correct
// permissions. Note that this function overrides the umask.
func (m *Mount) makeDirectories(setupMode SetupMode) error {
	// Zero the umask so we get the permissions we want
	oldMask := unix.Umask(0)
	defer func() {
		unix.Umask(oldMask)
	}()

	if err := os.Mkdir(m.BaseDir(), basePermissions); err != nil {
		return err
	}

	var dirMode os.FileMode
	switch setupMode {
	case SingleUserWritable:
		dirMode = 0755
	case WorldWritable:
		dirMode = os.ModeSticky | 0777
	}
	if err := os.Mkdir(m.PolicyDir(), dirMode); err != nil {
		return err
	}
	return os.Mkdir(m.ProtectorDir(), dirMode)
}

// GetSetupMode returns the current mode for fscrypt metadata creation on this
// filesystem.
func (m *Mount) GetSetupMode() (SetupMode, *user.User, error) {
	info1, err1 := os.Stat(m.PolicyDir())
	info2, err2 := os.Stat(m.ProtectorDir())

	if err1 == nil && err2 == nil {
		mask := os.ModeSticky | 0777
		mode1 := info1.Mode() & mask
		mode2 := info2.Mode() & mask
		uid1 := info1.Sys().(*syscall.Stat_t).Uid
		uid2 := info2.Sys().(*syscall.Stat_t).Uid
		user, err := util.UserFromUID(int64(uid1))
		if err == nil && mode1 == mode2 && uid1 == uid2 {
			switch mode1 {
			case mask:
				return WorldWritable, nil, nil
			case 0755:
				return SingleUserWritable, user, nil
			}
		}
		log.Printf("filesystem %s uses custom permissions on metadata directories", m.Path)
	}
	return -1, nil, errors.New("unable to determine setup mode")
}

// Setup sets up the filesystem for use with fscrypt. Note that this merely
// creates the appropriate files on the filesystem. It does not actually modify
// the filesystem's feature flags. This operation is atomic; it either succeeds
// or no files in the baseDir are created.
func (m *Mount) Setup(mode SetupMode) error {
	if m.CheckSetup(nil) == nil {
		return &ErrAlreadySetup{m}
	}
	if !m.isFscryptSetupAllowed() {
		return &ErrSetupNotSupported{m}
	}
	// We build the directories under a temp Mount and then move into place.
	temp, err := m.tempMount()
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp.Path)

	if err = temp.makeDirectories(mode); err != nil {
		return err
	}

	// Atomically move directory into place.
	return os.Rename(temp.BaseDir(), m.BaseDir())
}

// RemoveAllMetadata removes all the policy and protector metadata from the
// filesystem. This operation is atomic; it either succeeds or no files in the
// baseDir are removed.
// WARNING: Will cause data loss if the metadata is used to encrypt
// directories (this could include directories on other filesystems).
func (m *Mount) RemoveAllMetadata() error {
	if err := m.CheckSetup(nil); err != nil {
		return err
	}
	// temp will hold the old metadata temporarily
	temp, err := m.tempMount()
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp.Path)

	// Move directory into temp (to be destroyed on defer)
	return os.Rename(m.BaseDir(), temp.BaseDir())
}

func syncDirectory(dirPath string) error {
	dirFile, err := os.Open(dirPath)
	if err != nil {
		return err
	}
	if err = dirFile.Sync(); err != nil {
		dirFile.Close()
		return err
	}
	return dirFile.Close()
}

func (m *Mount) overwriteDataNonAtomic(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err != nil {
		log.Printf("WARNING: overwrite of %q failed; file will be corrupted!", path)
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	log.Printf("successfully overwrote %q non-atomically", path)
	return nil
}

// writeData writes the given data to the given path such that, if possible, the
// data is either written to stable storage or an error is returned.  If a file
// already exists at the path, it will be replaced.
//
// However, if the process doesn't have write permission to the directory but
// does have write permission to the file itself, then as a fallback the file is
// overwritten in-place rather than replaced.  Note that this may be non-atomic.
func (m *Mount) writeData(path string, data []byte, owner *user.User, mode os.FileMode) error {
	// Write the data to a temporary file, sync it, then rename into place
	// so that the operation will be atomic.
	dirPath := filepath.Dir(path)
	tempFile, err := ioutil.TempFile(dirPath, tempPrefix)
	if err != nil {
		log.Print(err)
		if os.IsPermission(err) {
			if _, err = os.Lstat(path); err == nil {
				log.Printf("trying non-atomic overwrite of %q", path)
				return m.overwriteDataNonAtomic(path, data)
			}
			return &ErrNoCreatePermission{m}
		}
		return err
	}
	defer os.Remove(tempFile.Name())

	// Ensure the new file has the right permissions mask.
	if err = tempFile.Chmod(mode); err != nil {
		tempFile.Close()
		return err
	}
	// Override the file owner if one was specified.  This happens when root
	// needs to create files owned by a particular user.
	if owner != nil {
		if err = util.Chown(tempFile, owner); err != nil {
			log.Printf("could not set owner of %q to %v: %v",
				path, owner.Username, err)
			tempFile.Close()
			return err
		}
	}
	if _, err = tempFile.Write(data); err != nil {
		tempFile.Close()
		return err
	}
	if err = tempFile.Sync(); err != nil {
		tempFile.Close()
		return err
	}
	if err = tempFile.Close(); err != nil {
		return err
	}

	if err = os.Rename(tempFile.Name(), path); err != nil {
		return err
	}
	// Ensure the rename has been persisted before returning success.
	return syncDirectory(dirPath)
}

// addMetadata writes the metadata structure to the file with the specified
// path. This will overwrite any existing data. The operation is atomic.
func (m *Mount) addMetadata(path string, md metadata.Metadata, owner *user.User) error {
	if err := md.CheckValidity(); err != nil {
		return errors.Wrap(err, "provided metadata is invalid")
	}

	data, err := proto.Marshal(md)
	if err != nil {
		return err
	}

	mode := filePermissions
	// If the file already exists, then preserve its owner and mode if
	// possible.  This is necessary because by default, for atomicity
	// reasons we'll replace the file rather than overwrite it.
	info, err := os.Lstat(path)
	if err == nil {
		if owner == nil && util.IsUserRoot() {
			uid := info.Sys().(*syscall.Stat_t).Uid
			if owner, err = util.UserFromUID(int64(uid)); err != nil {
				log.Print(err)
			}
		}
		mode = info.Mode() & 0777
	} else if !os.IsNotExist(err) {
		log.Print(err)
	}

	if owner != nil {
		log.Printf("writing metadata to %q and setting owner to %s", path, owner.Username)
	} else {
		log.Printf("writing metadata to %q", path)
	}
	return m.writeData(path, data, owner, mode)
}

// readMetadataFileSafe gets the contents of a metadata file extra-carefully,
// considering that it could be a malicious file created to cause a
// denial-of-service.  Specifically, the following checks are done:
//
// - It must be a regular file, not another type of file like a symlink or FIFO.
//   (Symlinks aren't bad by themselves, but given that a malicious user could
//   point one to absolutely anywhere, and there is no known use case for the
//   metadata files themselves being symlinks, it seems best to disallow them.)
// - It must have a reasonable size (<= maxMetadataFileSize).
// - If trustedUser is non-nil, then the file must be owned by the given user
//   or by root.
//
// Take care to avoid TOCTOU (time-of-check-time-of-use) bugs when doing these
// tests.  Notably, we must open the file before checking the file type, as the
// file type could change between any previous checks and the open.  When doing
// this, O_NOFOLLOW is needed to avoid following a symlink (this applies to the
// last path component only), and O_NONBLOCK is needed to avoid blocking if the
// file is a FIFO.
//
// This function returns the data read as well as the UID of the user who owns
// the file.  The returned UID is needed for login protectors, where the UID
// needs to be cross-checked with the UID stored in the file itself.
func readMetadataFileSafe(path string, trustedUser *user.User) ([]byte, int64, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, -1, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, -1, err
	}
	if !info.Mode().IsRegular() {
		return nil, -1, &ErrCorruptMetadata{path, errors.New("not a regular file")}
	}
	if !checkOwnership(path, info, trustedUser) {
		return nil, -1, &ErrCorruptMetadata{path, errors.New("metadata file belongs to another user")}
	}
	// Clear O_NONBLOCK, since it has served its purpose when opening the
	// file, and the behavior of reading from a regular file with O_NONBLOCK
	// is technically unspecified.
	if _, err = unix.FcntlInt(file.Fd(), unix.F_SETFL, 0); err != nil {
		return nil, -1, &os.PathError{Op: "clearing O_NONBLOCK", Path: path, Err: err}
	}
	// Read the file contents, allowing at most maxMetadataFileSize bytes.
	reader := &io.LimitedReader{R: file, N: maxMetadataFileSize + 1}
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, -1, err
	}
	if reader.N == 0 {
		return nil, -1, &ErrCorruptMetadata{path, errors.New("metadata file size limit exceeded")}
	}
	return data, int64(info.Sys().(*syscall.Stat_t).Uid), nil
}

// getMetadata reads the metadata structure from the file with the specified
// path. Only reads normal metadata files, not linked metadata.
func (m *Mount) getMetadata(path string, trustedUser *user.User, md metadata.Metadata) (int64, error) {
	data, owner, err := readMetadataFileSafe(path, trustedUser)
	if err != nil {
		log.Printf("could not read metadata from %q: %v", path, err)
		return -1, err
	}

	if err := proto.Unmarshal(data, md); err != nil {
		return -1, &ErrCorruptMetadata{path, err}
	}

	if err := md.CheckValidity(); err != nil {
		return -1, &ErrCorruptMetadata{path, err}
	}

	log.Printf("successfully read metadata from %q", path)
	return owner, nil
}

// removeMetadata deletes the metadata struct from the file with the specified
// path. Works with regular or linked metadata.
func (m *Mount) removeMetadata(path string) error {
	if err := os.Remove(path); err != nil {
		log.Printf("could not remove metadata file at %q: %v", path, err)
		return err
	}

	log.Printf("successfully removed metadata file at %q", path)
	return nil
}

// AddProtector adds the protector metadata to this filesystem's storage. This
// will overwrite the value of an existing protector with this descriptor. This
// will fail with ErrLinkedProtector if a linked protector with this descriptor
// already exists on the filesystem.
func (m *Mount) AddProtector(data *metadata.ProtectorData, owner *user.User) error {
	var err error
	if err = m.CheckSetup(nil); err != nil {
		return err
	}
	if isRegularFile(m.linkedProtectorPath(data.ProtectorDescriptor)) {
		return errors.Errorf("cannot modify linked protector %s on filesystem %s",
			data.ProtectorDescriptor, m.Path)
	}
	path := m.protectorPath(data.ProtectorDescriptor)
	return m.addMetadata(path, data, owner)
}

// AddLinkedProtector adds a link in this filesystem to the protector metadata
// in the dest filesystem, if one doesn't already exist.  On success, the return
// value is a nil error and a bool that is true iff the link is newly created.
func (m *Mount) AddLinkedProtector(descriptor string, dest *Mount, trustedUser *user.User,
	ownerIfCreating *user.User) (bool, error) {
	if err := m.CheckSetup(trustedUser); err != nil {
		return false, err
	}
	// Check that the link is good (descriptor exists, filesystem has UUID).
	if _, err := dest.GetRegularProtector(descriptor, trustedUser); err != nil {
		return false, err
	}

	linkPath := m.linkedProtectorPath(descriptor)

	// Check whether the link already exists.
	existingLink, _, err := readMetadataFileSafe(linkPath, trustedUser)
	if err == nil {
		existingLinkedMnt, err := getMountFromLink(string(existingLink))
		if err != nil {
			return false, errors.Wrap(err, linkPath)
		}
		if existingLinkedMnt != dest {
			return false, errors.Errorf("link %q points to %q, but expected %q",
				linkPath, existingLinkedMnt.Path, dest.Path)
		}
		return false, nil
	}
	if !os.IsNotExist(err) {
		return false, err
	}

	var newLink string
	newLink, err = makeLink(dest)
	if err != nil {
		return false, err
	}
	return true, m.writeData(linkPath, []byte(newLink), ownerIfCreating, filePermissions)
}

// GetRegularProtector looks up the protector metadata by descriptor. This will
// fail with ErrProtectorNotFound if the descriptor is a linked protector.
func (m *Mount) GetRegularProtector(descriptor string, trustedUser *user.User) (*metadata.ProtectorData, error) {
	if err := m.CheckSetup(trustedUser); err != nil {
		return nil, err
	}
	data := new(metadata.ProtectorData)
	path := m.protectorPath(descriptor)
	owner, err := m.getMetadata(path, trustedUser, data)
	if os.IsNotExist(err) {
		err = &ErrProtectorNotFound{descriptor, m}
	}
	if err != nil {
		return nil, err
	}
	// Login protectors have their UID stored in the file.  Since normally
	// any user can create files in the fscrypt metadata directories, for a
	// login protector to be considered valid it *must* be owned by the
	// claimed user or by root.  Note: fscrypt v0.3.2 and later always makes
	// login protectors owned by the user, but previous versions could
	// create them owned by root -- that is the main reason we allow root.
	if data.Source == metadata.SourceType_pam_passphrase && owner != 0 && owner != data.Uid {
		log.Printf("WARNING: %q claims to be the login protector for uid %d, but it is owned by uid %d.  Needs to be %d or 0.",
			path, data.Uid, owner, data.Uid)
		return nil, &ErrCorruptMetadata{path, errors.New("login protector belongs to wrong user")}
	}
	return data, nil
}

// GetProtector returns the Mount of the filesystem containing the information
// and that protector's data. If the descriptor is a regular (not linked)
// protector, the mount will return itself.
func (m *Mount) GetProtector(descriptor string, trustedUser *user.User) (*Mount, *metadata.ProtectorData, error) {
	if err := m.CheckSetup(trustedUser); err != nil {
		return nil, nil, err
	}
	// Get the link data from the link file
	path := m.linkedProtectorPath(descriptor)
	link, _, err := readMetadataFileSafe(path, trustedUser)
	if err != nil {
		// If the link doesn't exist, try for a regular protector.
		if os.IsNotExist(err) {
			data, err := m.GetRegularProtector(descriptor, trustedUser)
			return m, data, err
		}
		return nil, nil, err
	}
	log.Printf("following protector link %s", path)
	linkedMnt, err := getMountFromLink(string(link))
	if err != nil {
		return nil, nil, errors.Wrap(err, path)
	}
	data, err := linkedMnt.GetRegularProtector(descriptor, trustedUser)
	if err != nil {
		return nil, nil, &ErrFollowLink{string(link), err}
	}
	return linkedMnt, data, nil
}

// RemoveProtector deletes the protector metadata (or a link to another
// filesystem's metadata) from the filesystem storage.
func (m *Mount) RemoveProtector(descriptor string) error {
	if err := m.CheckSetup(nil); err != nil {
		return err
	}
	// We first try to remove the linkedProtector. If that metadata does not
	// exist, we try to remove the normal protector.
	err := m.removeMetadata(m.linkedProtectorPath(descriptor))
	if os.IsNotExist(err) {
		err = m.removeMetadata(m.protectorPath(descriptor))
		if os.IsNotExist(err) {
			err = &ErrProtectorNotFound{descriptor, m}
		}
	}
	return err
}

// ListProtectors lists the descriptors of all protectors on this filesystem.
// This does not include linked protectors.  If trustedUser is non-nil, then
// the protectors are restricted to those owned by the given user or by root.
func (m *Mount) ListProtectors(trustedUser *user.User) ([]string, error) {
	return m.listMetadata(m.ProtectorDir(), "protectors", trustedUser)
}

// AddPolicy adds the policy metadata to the filesystem storage.
func (m *Mount) AddPolicy(data *metadata.PolicyData, owner *user.User) error {
	if err := m.CheckSetup(nil); err != nil {
		return err
	}

	return m.addMetadata(m.PolicyPath(data.KeyDescriptor), data, owner)
}

// GetPolicy looks up the policy metadata by descriptor.
func (m *Mount) GetPolicy(descriptor string, trustedUser *user.User) (*metadata.PolicyData, error) {
	if err := m.CheckSetup(trustedUser); err != nil {
		return nil, err
	}
	data := new(metadata.PolicyData)
	_, err := m.getMetadata(m.PolicyPath(descriptor), trustedUser, data)
	if os.IsNotExist(err) {
		err = &ErrPolicyNotFound{descriptor, m}
	}
	return data, err
}

// RemovePolicy deletes the policy metadata from the filesystem storage.
func (m *Mount) RemovePolicy(descriptor string) error {
	if err := m.CheckSetup(nil); err != nil {
		return err
	}
	err := m.removeMetadata(m.PolicyPath(descriptor))
	if os.IsNotExist(err) {
		err = &ErrPolicyNotFound{descriptor, m}
	}
	return err
}

// ListPolicies lists the descriptors of all policies on this filesystem.  If
// trustedUser is non-nil, then the policies are restricted to those owned by
// the given user or by root.
func (m *Mount) ListPolicies(trustedUser *user.User) ([]string, error) {
	return m.listMetadata(m.PolicyDir(), "policies", trustedUser)
}

type namesAndTimes struct {
	names []string
	times []time.Time
}

func (c namesAndTimes) Len() int {
	return len(c.names)
}

func (c namesAndTimes) Less(i, j int) bool {
	return c.times[i].Before(c.times[j])
}

func (c namesAndTimes) Swap(i, j int) {
	c.names[i], c.names[j] = c.names[j], c.names[i]
	c.times[i], c.times[j] = c.times[j], c.times[i]
}

func sortFileListByLastMtime(directoryPath string, names []string) error {
	c := namesAndTimes{names: names, times: make([]time.Time, len(names))}
	for i, name := range names {
		fi, err := os.Lstat(filepath.Join(directoryPath, name))
		if err != nil {
			return err
		}
		c.times[i] = fi.ModTime()
	}
	sort.Sort(c)
	return nil
}

// listDirectory returns a list of descriptors for a metadata directory,
// including files which are links to other filesystem's metadata.
func (m *Mount) listDirectory(directoryPath string) ([]string, error) {
	dir, err := os.Open(directoryPath)
	if err != nil {
		return nil, err
	}
	defer dir.Close()

	names, err := dir.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	if SortDescriptorsByLastMtime {
		if err := sortFileListByLastMtime(directoryPath, names); err != nil {
			return nil, err
		}
	}

	descriptors := make([]string, 0, len(names))
	for _, name := range names {
		// Be sure to include links as well
		descriptors = append(descriptors, strings.TrimSuffix(name, linkFileExtension))
	}
	return descriptors, nil
}

func (m *Mount) listMetadata(dirPath string, metadataType string, owner *user.User) ([]string, error) {
	log.Printf("listing %s in %q", metadataType, dirPath)
	if err := m.CheckSetup(owner); err != nil {
		return nil, err
	}
	names, err := m.listDirectory(dirPath)
	if err != nil {
		return nil, err
	}
	filesIgnoredDescription := ""
	if owner != nil {
		filteredNames := make([]string, 0, len(names))
		uid := uint32(util.AtoiOrPanic(owner.Uid))
		for _, name := range names {
			info, err := os.Lstat(filepath.Join(dirPath, name))
			if err != nil {
				continue
			}
			fileUID := info.Sys().(*syscall.Stat_t).Uid
			if fileUID != uid && fileUID != 0 {
				continue
			}
			filteredNames = append(filteredNames, name)
		}
		numIgnored := len(names) - len(filteredNames)
		if numIgnored != 0 {
			filesIgnoredDescription =
				fmt.Sprintf(" (ignored %d %s not owned by %s or root)",
					numIgnored, metadataType, owner.Username)
		}
		names = filteredNames
	}
	log.Printf("found %d %s%s", len(names), metadataType, filesIgnoredDescription)
	return names, nil
}
