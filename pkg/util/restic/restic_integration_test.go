package restic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type localRestic struct {
	t          *testing.T
	ctx        context.Context
	binary     string
	repository string
}

func newLocalRestic(t *testing.T) *localRestic {
	t.Helper()
	binary := os.Getenv("HARVESTER_TEST_RESTIC_BINARY")
	if binary == "" {
		t.Skip("HARVESTER_TEST_RESTIC_BINARY is not set")
	}
	// Restic's lock backoff dominates even for a tiny repository.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	repository := filepath.Join(t.TempDir(), "repository")
	t.Setenv("RESTIC_REPOSITORY", repository)
	t.Setenv("RESTIC_PASSWORD", "local-regression-test")
	t.Setenv("RESTIC_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))
	lr := &localRestic{t: t, ctx: ctx, binary: binary, repository: repository}
	lr.run("", "init")
	for i := 0; i < 3; i++ {
		lr.run(fmt.Sprintf("snapshot-%d", i), "backup", "--stdin", "--stdin-filename", "volume",
			"--tag", fmt.Sprintf("ns=default,vmb=backup-%d,sn=volume", i))
	}
	return lr
}

func (lr *localRestic) run(input string, args ...string) []byte {
	lr.t.Helper()
	// #nosec G204 -- the binary comes from trusted test configuration; arguments are test-controlled.
	cmd := exec.CommandContext(lr.ctx, lr.binary, args...)
	cmd.Stdin = strings.NewReader(input)
	output, err := cmd.CombinedOutput()
	require.NoError(lr.t, err, "%s", output)
	return output
}

func (lr *localRestic) holdLock() (*os.File, *exec.Cmd, *bytes.Buffer) {
	lr.t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(lr.t, err)
	lr.t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	// #nosec G204 -- the binary comes from trusted test configuration; arguments are fixed.
	holder := exec.CommandContext(lr.ctx, lr.binary, "backup", "--stdin", "--stdin-filename", "lock-holder",
		"--tag", "ns=default,vmb=unrelated,sn=lock-holder")
	holder.Stdin = reader
	logs := &bytes.Buffer{}
	holder.Stdout, holder.Stderr = logs, logs
	require.NoError(lr.t, holder.Start())
	lr.t.Cleanup(func() { _ = holder.Process.Kill(); _ = holder.Wait() })
	lr.waitForLock()
	return writer, holder, logs
}

func (lr *localRestic) waitForLock() {
	lr.t.Helper()
	require.Eventually(lr.t, func() bool {
		locks, err := os.ReadDir(filepath.Join(lr.repository, "locks"))
		if err != nil {
			return false
		}
		for _, lock := range locks {
			// Restic publishes a 64-character ID after renaming its temporary file.
			if len(lock.Name()) == 64 {
				return true
			}
		}
		return false
	}, 5*time.Second, 20*time.Millisecond, "lock holder did not acquire repository lock")
}

func (lr *localRestic) holdRestore(payload string) (*os.File, *exec.Cmd, *bytes.Buffer) {
	lr.t.Helper()
	lr.run(payload, "backup", "--stdin", "--stdin-filename", "volume", "--tag", "vmb=restore-source")
	reader, writer, err := os.Pipe()
	require.NoError(lr.t, err)
	lr.t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	// Leave the output unread so dump retains its shared lock while restoring.
	// #nosec G204 -- the binary comes from trusted test configuration; arguments are fixed.
	holder := exec.CommandContext(lr.ctx, lr.binary, "dump", "--tag", "vmb=restore-source", "latest", "/volume")
	logs := &bytes.Buffer{}
	holder.Stdout, holder.Stderr = writer, logs
	require.NoError(lr.t, holder.Start())
	lr.t.Cleanup(func() { _ = holder.Process.Kill(); _ = holder.Wait() })
	require.NoError(lr.t, writer.Close())
	lr.waitForLock()
	return reader, holder, logs
}

type forgetResult struct {
	index  int
	output []byte
	err    error
}

func (lr *localRestic) startForgets() <-chan forgetResult {
	lr.t.Helper()
	results := make(chan forgetResult, 3)
	for i := 0; i < 3; i++ {
		go func() {
			command := ForgetCommand(fmt.Sprintf("%q --no-cache", lr.binary), []string{
				"ns=default", fmt.Sprintf("vmb=backup-%d", i), "sn=volume",
			})
			output, err := exec.CommandContext(lr.ctx, "/bin/sh", "-c", command).CombinedOutput()
			results <- forgetResult{index: i, output: output, err: err}
		}()
	}
	return results
}

func (lr *localRestic) waitForgets(results <-chan forgetResult) {
	lr.t.Helper()
	for i := 0; i < 3; i++ {
		select {
		case r := <-results:
			require.NoError(lr.t, r.err, "forget %d: %s", r.index, r.output)
		case <-lr.ctx.Done():
			lr.t.Fatal("concurrent forget commands did not finish after lock release")
		}
	}
}

func (lr *localRestic) assertClean() {
	lr.t.Helper()
	var snapshots []struct {
		Tags []string `json:"tags"`
	}
	require.NoError(lr.t, json.Unmarshal(lr.run("", "snapshots", "--json"), &snapshots))
	require.Len(lr.t, snapshots, 1, "only the unrelated backup must remain")
	require.Contains(lr.t, snapshots[0].Tags, "vmb=unrelated")
	locks, err := os.ReadDir(filepath.Join(lr.repository, "locks"))
	require.NoError(lr.t, err)
	require.Empty(lr.t, locks, "successful cleanup must leave no repository locks")
}

// Set HARVESTER_TEST_RESTIC_BINARY to exercise real lock contention locally.
func TestConcurrentForget(t *testing.T) {
	lr := newLocalRestic(t)
	// Keep a backup's shared lock alive; all forget commands must wait.
	writer, holder, logs := lr.holdLock()
	locks, err := os.ReadDir(filepath.Join(lr.repository, "locks"))
	require.NoError(t, err)
	lr.run("", "unlock")
	remaining, err := os.ReadDir(filepath.Join(lr.repository, "locks"))
	require.NoError(t, err)
	require.Equal(t, locks, remaining, "unlock must preserve the active backup's lock")
	results := lr.startForgets()
	select {
	case r := <-results:
		t.Fatalf("forget %d returned before lock release: %v\n%s", r.index, r.err, r.output)
	case <-time.After(500 * time.Millisecond):
	}
	_, err = writer.WriteString("unrelated backup payload")
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, holder.Wait(), "%s", logs.String())
	lr.waitForgets(results)
	lr.assertClean()
}

func TestForgetAfterKilledBackup(t *testing.T) {
	lr := newLocalRestic(t)
	_, holder, _ := lr.holdLock()
	require.NoError(t, holder.Process.Kill())
	require.Error(t, holder.Wait())
	locks, err := os.ReadDir(filepath.Join(lr.repository, "locks"))
	require.NoError(t, err)
	require.NotEmpty(t, locks, "killing the backup must leave its repository lock")

	// Both processes share a hostname, so Restic detects the dead PID immediately.
	// Across Pods, unlock removes the lock only after the 30-minute timeout.
	command := ForgetCommand(shellQuote(lr.binary)+" --no-cache", []string{
		"ns=default", "vmb=backup-0", "sn=volume",
	})
	output, err := runShell(t, command)
	require.NoError(t, err, output)
	remaining, err := os.ReadDir(filepath.Join(lr.repository, "locks"))
	require.NoError(t, err)
	require.Empty(t, remaining, "forget must recover the orphaned lock")

	var snapshots []struct {
		Tags []string `json:"tags"`
	}
	require.NoError(t, json.Unmarshal(lr.run("", "snapshots", "--json"), &snapshots))
	require.Len(t, snapshots, 2, "forget must remove only the selected backup")
	for _, snapshot := range snapshots {
		require.NotContains(t, snapshot.Tags, "vmb=backup-0")
	}
}

func TestBackupWaitsForRestore(t *testing.T) {
	lr := newLocalRestic(t)
	payload := strings.Repeat("restore-data", 128*1024)
	reader, holder, logs := lr.holdRestore(payload)
	locks, err := os.ReadDir(filepath.Join(lr.repository, "locks"))
	require.NoError(t, err)

	command := BackupCommand(shellQuote(lr.binary)+" --no-cache", "printf complete-data", "volume",
		[]string{"ns=default", "vmb=concurrent", "sn=volume"})
	// Exercise multiple tag attempts quickly; unit tests check the production 5m wait.
	command = strings.ReplaceAll(command, "--retry-lock 5m", "--retry-lock 100ms")
	output, err := os.CreateTemp(t.TempDir(), "backup-log")
	require.NoError(t, err)
	t.Cleanup(func() { _ = output.Close() })
	// #nosec G702 -- the script is generated from trusted test inputs.
	job := exec.CommandContext(lr.ctx, "/bin/bash", "-c", command)
	job.Stdout, job.Stderr = output, output
	require.NoError(t, job.Start())
	t.Cleanup(func() { _ = job.Process.Kill(); _ = job.Wait() })
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(output.Name())
		return err == nil && strings.Count(string(data), "repo already locked") >= 2
	}, 30*time.Second, 50*time.Millisecond, "tag did not retry after its first lock wait")

	lr.assertBackupTag(false)
	for _, lock := range locks {
		_, err := os.Stat(filepath.Join(lr.repository, "locks", lock.Name()))
		require.NoError(t, err, "backup must preserve the active restore's lock")
	}
	n, err := io.Copy(io.Discard, reader)
	require.NoError(t, err)
	require.EqualValues(t, len(payload), n)
	require.NoError(t, holder.Wait(), "%s", logs.String())
	err = job.Wait()
	data, readErr := os.ReadFile(output.Name())
	require.NoError(t, readErr)
	require.NoError(t, err, "%s", data)
	lr.assertBackupTag(true)
	require.Equal(t, "complete-data", string(lr.run("", "dump", "--tag", "vmb=concurrent", "latest", "/volume")))
}

func (lr *localRestic) assertBackupTag(complete bool) {
	lr.t.Helper()
	var snapshots []struct {
		Tags []string `json:"tags"`
	}
	data := lr.run("", "snapshots", "--json", "--tag", "vmb=concurrent")
	require.NoError(lr.t, json.Unmarshal(data, &snapshots))
	require.Len(lr.t, snapshots, 1, "waiting for the tag lock must not repeat the upload")
	if complete {
		require.Contains(lr.t, snapshots[0].Tags, CompleteTag)
		return
	}
	require.NotContains(lr.t, snapshots[0].Tags, CompleteTag)
}
