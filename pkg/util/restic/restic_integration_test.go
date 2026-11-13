package restic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	require.Eventually(lr.t, func() bool {
		locks, err := os.ReadDir(filepath.Join(lr.repository, "locks"))
		return err == nil && len(locks) > 0
	}, 5*time.Second, 20*time.Millisecond, "lock holder did not acquire repository lock")
	return writer, holder, logs
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
	results := lr.startForgets()
	select {
	case r := <-results:
		t.Fatalf("forget %d returned before lock release: %v\n%s", r.index, r.err, r.output)
	case <-time.After(500 * time.Millisecond):
	}
	_, err := writer.WriteString("unrelated backup payload")
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, holder.Wait(), "%s", logs.String())
	lr.waitForgets(results)
	lr.assertClean()
}
