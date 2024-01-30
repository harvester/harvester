/*
 * errors.go - Custom errors and error functions used by fscrypt
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

package util

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/pkg/errors"
)

// ErrReader wraps an io.Reader, passing along calls to Read() until a read
// fails. Then, the error is stored, and all subsequent calls to Read() do
// nothing. This allows you to write code which has many subsequent reads and
// do all of the error checking at the end. For example:
//
//	r := NewErrReader(reader)
//	r.Read(foo)
//	r.Read(bar)
//	r.Read(baz)
//	if r.Err() != nil {
//		// Handle error
//	}
//
// Taken from https://blog.golang.org/errors-are-values by Rob Pike.
type ErrReader struct {
	r   io.Reader
	err error
}

// NewErrReader creates an ErrReader which wraps the provided reader.
func NewErrReader(reader io.Reader) *ErrReader {
	return &ErrReader{r: reader, err: nil}
}

// Read runs ReadFull on the wrapped reader if no errors have occurred.
// Otherwise, the previous error is just returned and no reads are attempted.
func (e *ErrReader) Read(p []byte) (n int, err error) {
	if e.err == nil {
		n, e.err = io.ReadFull(e.r, p)
	}
	return n, e.err
}

// Err returns the first encountered err (or nil if no errors occurred).
func (e *ErrReader) Err() error {
	return e.err
}

// ErrWriter works exactly like ErrReader, except with io.Writer.
type ErrWriter struct {
	w   io.Writer
	err error
}

// NewErrWriter creates an ErrWriter which wraps the provided writer.
func NewErrWriter(writer io.Writer) *ErrWriter {
	return &ErrWriter{w: writer, err: nil}
}

// Write runs the wrapped writer's Write if no errors have occurred. Otherwise,
// the previous error is just returned and no writes are attempted.
func (e *ErrWriter) Write(p []byte) (n int, err error) {
	if e.err == nil {
		n, e.err = e.w.Write(p)
	}
	return n, e.err
}

// Err returns the first encountered err (or nil if no errors occurred).
func (e *ErrWriter) Err() error {
	return e.err
}

// CheckValidLength returns an invalid length error if expected != actual
func CheckValidLength(expected, actual int) error {
	if expected == actual {
		return nil
	}
	return fmt.Errorf("expected length of %d, got %d", expected, actual)
}

// SystemError is an error that should indicate something has gone wrong in the
// underlying system (syscall failure, bad ioctl, etc...).
type SystemError string

func (s SystemError) Error() string {
	return "system error: " + string(s)
}

// NeverError panics if a non-nil error is passed in. It should be used to check
// for logic errors, not to handle recoverable errors.
func NeverError(err error) {
	if err != nil {
		log.Panicf("NeverError() check failed: %v", err)
	}
}

var (
	// testEnvVarName is the name of an environment variable that should be
	// set to an empty mountpoint. This is only used for integration tests.
	// If not set, integration tests are skipped.
	testEnvVarName = "TEST_FILESYSTEM_ROOT"
	// ErrSkipIntegration indicates integration tests shouldn't be run.
	ErrSkipIntegration = errors.New("skipping integration test")
)

// TestRoot returns a the root of a filesystem specified by testEnvVarName. This
// function is only used for integration tests.
func TestRoot() (string, error) {
	path := os.Getenv(testEnvVarName)
	if path == "" {
		return "", ErrSkipIntegration
	}
	return path, nil
}
