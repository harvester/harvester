package ns

import (
	"syscall"

	"github.com/pkg/errors"

	"github.com/longhorn/go-common-libs/io"
	"github.com/longhorn/go-common-libs/sys"
	"github.com/longhorn/go-common-libs/types"
)

// GetKernelRelease switches to the host namespace and retrieves the kernel release.
func GetKernelRelease() (string, error) {
	var err error
	defer func() {
		err = errors.Wrap(err, "failed to get kernel release")
	}()

	fn := func() (interface{}, error) {
		return sys.GetKernelRelease()
	}

	rawResult, err := RunFunc(fn, 0)
	if err != nil {
		return "", err
	}

	var result string
	var ableToCast bool
	result, ableToCast = rawResult.(string)
	if !ableToCast {
		return "", errors.Errorf(types.ErrNamespaceCastResultFmt, result, rawResult)
	}
	return result, nil
}

// GetOSDistro switches to the host namespace and retrieves the OS distro.
func GetOSDistro() (result string, err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to get host OS distro")
	}()

	fn := func() (interface{}, error) {
		return io.ReadFileContent(types.OsReleaseFilePath)
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

	return sys.GetOSDistro(result)
}

// Sync switches to the host namespace and calls sync.
func Sync() (err error) {
	defer func() {
		err = errors.Wrap(err, "failed to get kernel release")
	}()

	fn := func() (interface{}, error) {
		syscall.Sync()
		return nil, nil
	}

	_, err = RunFunc(fn, 0)
	return err
}

// GetSystemBlockDevices switches to the host namespace and retrieves the
// system block devices.
func GetSystemBlockDevices() (result map[string]types.BlockDeviceInfo, err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to get system block devices")
	}()

	fn := func() (interface{}, error) {
		return sys.GetSystemBlockDeviceInfo()
	}

	rawResult, err := RunFunc(fn, 0)
	if err != nil {
		return nil, err
	}

	var ableToCast bool
	result, ableToCast = rawResult.(map[string]types.BlockDeviceInfo)
	if !ableToCast {
		return nil, errors.Errorf(types.ErrNamespaceCastResultFmt, result, rawResult)
	}
	return result, nil
}
