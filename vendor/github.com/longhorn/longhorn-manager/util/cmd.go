package util

import (
	"bufio"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
)

func CmdOutLines(cmd *exec.Cmd, cancel <-chan interface{}) (<-chan string, <-chan error) {
	r := make(chan string)
	errCh := make(chan error, 1)
	out, err := cmd.StdoutPipe()
	if err != nil {
		defer close(errCh)
		defer close(r)
		errCh <- errors.Wrapf(err, "error obtaining stdout of `%s`", strings.Join(cmd.Args, " "))
		return r, errCh
	}
	if err := cmd.Start(); err != nil {
		defer close(errCh)
		defer close(r)
		errCh <- errors.Wrapf(err, "error starting cmd `%s`", strings.Join(cmd.Args, " "))
		return r, errCh
	}
	go func() {
		defer close(errCh)
		defer close(r)
		defer func() {
			if err := cmd.Wait(); err != nil {
				errCh <- err
			}
		}()
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			select {
			case <-cancel:
				break
			case r <- scanner.Text():
				// continue
			}
		}
	}()
	return r, errCh
}
