package util

import (
	"os/exec"
	"time"
)

var (
	sleepInterval = 5 * time.Second
)

// SleepAndReboot do sleep and exec reboot
func SleepAndReboot() error {
	time.Sleep(sleepInterval)
	return exec.Command("reboot").Run()
}
