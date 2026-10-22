//go:build !linux

package acceleratedio

import (
	"errors"
	"io"
)

func Run(_ []string, _ io.Reader, _, _ io.Writer) error {
	return errors.New("io-mode is supported on linux only")
}
