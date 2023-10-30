package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	DefaulCmdTimeout = time.Minute // one minute by default

	GRPCHealthProbe = "/usr/local/bin/grpc_health_probe"
)

func Execute(binary string, args ...string) (string, error) {
	return ExecuteWithTimeout(DefaulCmdTimeout, binary, args...)
}

func ExecuteWithTimeout(timeout time.Duration, binary string, args ...string) (string, error) {
	var err error
	cmd := exec.Command(binary, args...)
	done := make(chan struct{})

	var output, stderr bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr

	go func() {
		err = cmd.Run()
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		if cmd.Process != nil {
			if err := cmd.Process.Kill(); err != nil {
				logrus.WithError(err).Warnf("Problem killing process pid=%v", cmd.Process.Pid)
			}

		}
		return "", errors.Wrapf(err, "timeout executing: %v %v, output %s, stderr %s",
			binary, args, output.String(), stderr.String())
	}

	if err != nil {
		return "", errors.Wrapf(err, "failed to execute: %v %v, output %s, stderr %s",
			binary, args, output.String(), stderr.String())
	}
	return output.String(), nil
}

func PrintJSON(obj interface{}) error {
	output, err := json.MarshalIndent(obj, "", "\t")
	if err != nil {
		return err
	}

	fmt.Println(string(output))
	return nil
}

func GetURL(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func RemoveFile(file string) error {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		// file doesn't exist
		return nil
	}

	if _, err := Execute("rm", file); err != nil {
		return errors.Wrapf(err, "failed to remove file %v", file)
	}

	return nil
}

func GRPCServiceReadinessProbe(address string) bool {
	if _, err := Execute(GRPCHealthProbe, "-addr", address); err != nil {
		return false
	}
	return true
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func UUID() string {
	return uuid.New().String()
}
