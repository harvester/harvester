package core

import (
	"errors"
)

// ReadCloseFail 内部测试使用
type ReadCloseFail struct{}

// Read 供测试使用
func (r *ReadCloseFail) Read(p []byte) (n int, err error) {
	return 0, errors.New("must fail")
}

// Close 内部测试使用
func (r *ReadCloseFail) Close() error {
	return nil
}
