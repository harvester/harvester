package exec

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/longhorn/go-common-libs/types"
)

// ExecuteInterface is the interface for executing commands.
type ExecuteInterface interface {
	Execute(envs []string, binary string, args []string, timeout time.Duration) (string, error)
	ExecuteWithStdin(binary string, args []string, stdinString string, timeout time.Duration) (string, error)
	ExecuteWithStdinPipe(binary string, args []string, stdinString string, timeout time.Duration) (string, error)
}

// NewExecutor returns a new Executor.
func NewExecutor() ExecuteInterface {
	return &Executor{}
}

// Executor is the implementation of ExecuteInterface.
type Executor struct{}

// Execute executes the given command with the specified environment variables, binary, and arguments.
// It returns the command's output and any occurred error.
func (e *Executor) Execute(envs []string, binary string, args []string, timeout time.Duration) (string, error) {
	// If the timeout is set to -1, execute the command without any timeout.
	// Otherwise, execute the command with the specified timeout.
	switch timeout {
	case types.ExecuteNoTimeout:
		return e.executeWithoutTimeout(envs, binary, args)
	default:
		return e.executeWithTimeout(envs, binary, args, timeout)
	}
}

// ExecuteWithTimeout executes the command with timeout.
func (e *Executor) executeWithTimeout(envs []string, binary string, args []string, timeout time.Duration) (string, error) {
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), envs...)
	return e.executeCmd(cmd, timeout)
}

// ExecuteWithoutTimeout executes the command without timeout.
func (e *Executor) executeWithoutTimeout(envs []string, binary string, args []string) (string, error) {
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), envs...)

	var output, stderr bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return output.String(), errors.Wrapf(err, "failed to execute: %v %v, output %s, stderr %s",
			binary, args, output.String(), stderr.String())
	}
	return output.String(), nil
}

// executeCmd executes the command with timeout. If timeout is 0, it will use default timeout.
func (e *Executor) executeCmd(cmd *exec.Cmd, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var output, stderr bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr

	errChan := make(chan error, 1)
	go func() {
		errChan <- cmd.Run()
	}()

	select {
	case err := <-errChan:
		if err != nil {
			return "", errors.Wrapf(err, "failed to execute: %v %v, output %s, stderr %s",
				cmd.Path, cmd.Args, output.String(), stderr.String())
		}
	case <-ctx.Done():
		return "", errors.Errorf("timeout executing: %v %v", cmd.Path, cmd.Args)
	}

	return output.String(), nil
}

// ExecuteWithStdin executes the command with stdin.
func (e *Executor) ExecuteWithStdin(binary string, args []string, stdinString string, timeout time.Duration) (string, error) {
	cmd := exec.Command(binary, args...)
	cmd.Env = os.Environ()

	if stdinString != "" {
		cmd.Stdin = strings.NewReader(stdinString)
	}

	return e.executeCmd(cmd, timeout)
}

// ExecuteWithStdinPipe executes the command with stdin pipe.
func (e *Executor) ExecuteWithStdinPipe(binary string, args []string, stdinString string, timeout time.Duration) (string, error) {
	cmd := exec.Command(binary, args...)
	cmd.Env = os.Environ()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}

	go func() {
		defer func() {
			_ = stdin.Close()
		}()
		_, _ = io.WriteString(stdin, stdinString)
	}()

	return e.executeCmd(cmd, timeout)
}
