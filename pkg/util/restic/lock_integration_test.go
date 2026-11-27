//go:build linux

package restic

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Pause a real tag process while its exclusive lock is published. Restic waits
// 200ms after publishing the lock before checking for concurrent acquisitions.
func (lr *localRestic) holdExclusiveLock() (*exec.Cmd, *bytes.Buffer) {
	lr.t.Helper()
	// #nosec G204 -- the binary comes from trusted test configuration; arguments are fixed.
	holder := exec.CommandContext(lr.ctx, lr.binary,
		"tag", "--add", "lock-holder", "--tag", "vmb=backup-1")
	logs := &bytes.Buffer{}
	holder.Stdout, holder.Stderr = logs, logs
	require.NoError(lr.t, holder.Start())
	lr.t.Cleanup(func() { _ = holder.Process.Kill(); _ = holder.Wait() })
	lr.waitForLock()
	require.NoError(lr.t, holder.Process.Signal(syscall.SIGSTOP))
	return holder, logs
}

type waitingCommand struct {
	log  string
	done <-chan error
}

func (lr *localRestic) startCommand(command string) waitingCommand {
	lr.t.Helper()
	output, err := os.CreateTemp(lr.t.TempDir(), "command-log")
	require.NoError(lr.t, err)
	lr.t.Cleanup(func() { _ = output.Close() })
	// Short windows exercise the outer retry; production retains the 5m wait.
	command = strings.ReplaceAll(command, "--retry-lock 5m", "--retry-lock 100ms")
	// #nosec G702 -- the script is generated from trusted test inputs.
	cmd := exec.CommandContext(lr.ctx, "/bin/bash", "-c", command)
	cmd.Stdout, cmd.Stderr = output, output
	require.NoError(lr.t, cmd.Start())
	lr.t.Cleanup(func() { _ = cmd.Process.Kill() })
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	return waitingCommand{log: output.Name(), done: done}
}

func TestCommandsWaitForExclusiveLock(t *testing.T) {
	lr := newLocalRestic(t)
	lr.run("", "tag", "--add", CompleteTag, "--tag", "vmb=backup-0")
	holder, logs := lr.holdExclusiveLock()
	command := shellQuote(lr.binary) + " --no-cache"
	tags := []string{"ns=default", "vmb=backup-0", "sn=volume"}
	data := filepath.Join(t.TempDir(), "restored")
	readerCalls := filepath.Join(t.TempDir(), "reader-calls")
	reader := "sh -c " + shellQuote("echo read >> "+shellQuote(readerCalls)+"; printf complete-data")
	waiting := []waitingCommand{
		lr.startCommand(BackupCommand(command, reader, "volume",
			[]string{"ns=default", "vmb=concurrent", "sn=volume"})),
		lr.startCommand(SnapshotCheckCommand(command, tags)),
		lr.startCommand(RestoreCommand(command, "cat > "+shellQuote(data), "volume", tags)),
	}
	for _, job := range waiting {
		require.Eventually(t, func() bool {
			out, err := os.ReadFile(job.log)
			return err == nil && strings.Count(string(out), "retrying lock acquisition") >= 2
		}, 30*time.Second, 50*time.Millisecond, "command did not retry its lock wait: %s", job.log)
	}
	_, err := os.Stat(readerCalls)
	require.True(t, os.IsNotExist(err), "backup must wait before reading the source")
	payload, err := os.ReadFile(data)
	require.NoError(t, err)
	require.Empty(t, payload, "lock messages must not enter the restore stream")

	require.NoError(t, holder.Process.Signal(syscall.SIGCONT))
	require.NoError(t, holder.Wait(), "%s", logs.String())
	for _, job := range waiting {
		select {
		case err := <-job.done:
			out, readErr := os.ReadFile(job.log)
			require.NoError(t, readErr)
			require.NoError(t, err, "%s", out)
		case <-lr.ctx.Done():
			t.Fatal("command did not finish after the exclusive lock was released")
		}
	}
	payload, err = os.ReadFile(data)
	require.NoError(t, err)
	require.Equal(t, "snapshot-0", string(payload))
	calls, err := os.ReadFile(readerCalls)
	require.NoError(t, err)
	require.Equal(t, "read\n", string(calls))
	lr.assertBackupTag(true)
	require.Equal(t, "complete-data", string(lr.run("", "dump", "--tag", "vmb=concurrent", "latest", "/volume")))
}

func TestBackupRequiresRepository(t *testing.T) {
	binary := os.Getenv("HARVESTER_TEST_RESTIC_BINARY")
	if binary == "" {
		t.Skip("HARVESTER_TEST_RESTIC_BINARY is not set")
	}
	repo := filepath.Join(t.TempDir(), "repository")
	t.Setenv("RESTIC_REPOSITORY", repo)
	t.Setenv("RESTIC_PASSWORD", "local-regression-test")
	command := shellQuote(binary) + " --no-cache"
	tags := []string{"ns=default", "vmb=backup", "sn=volume"}
	out, err := runShell(t, BackupCommand(command, "printf complete-data", "volume", tags))
	require.Error(t, err, out)
	_, err = os.Stat(filepath.Join(repo, "config"))
	require.True(t, os.IsNotExist(err), "backup must not initialize a missing repository")
}

func TestBackupRecoversKilledLock(t *testing.T) {
	for _, when := range []string{"before first attempt", "during retry"} {
		t.Run(when, func(t *testing.T) {
			lr := newLocalRestic(t)
			_, holder, _ := lr.holdLock()
			if when == "before first attempt" {
				require.NoError(t, holder.Process.Kill())
				require.Error(t, holder.Wait())
			}

			command := BackupCommand(shellQuote(lr.binary)+" --no-cache", "printf complete-data", "volume",
				[]string{"ns=default", "vmb=concurrent", "sn=volume"})
			job := lr.startCommand(command)
			if when == "during retry" {
				require.Eventually(t, func() bool {
					out, err := os.ReadFile(job.log)
					return err == nil && strings.Contains(string(out), "retrying lock acquisition")
				}, 30*time.Second, 50*time.Millisecond, "tag did not wait for the active backup")
				lr.assertBackupTag(false)
				require.NoError(t, holder.Process.Kill())
				require.Error(t, holder.Wait())
			}

			// Local processes share a hostname, so unlock detects the dead PID.
			// Across Pods, it waits until 30 minutes after the lock's last refresh.
			select {
			case err := <-job.done:
				out, readErr := os.ReadFile(job.log)
				require.NoError(t, readErr)
				require.NoError(t, err, "%s", out)
				require.Contains(t, string(out), "successfully removed 1 locks")
			case <-lr.ctx.Done():
				t.Fatal("backup did not recover the killed process's lock")
			}
			lr.assertBackupTag(true)
			payload := lr.run("", "dump", "--tag", "vmb=concurrent", "latest", "/volume")
			require.Equal(t, "complete-data", string(payload))
		})
	}
}
