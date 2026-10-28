package restic

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func runShell(t *testing.T, command string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/bin/bash", "-c", command).CombinedOutput()
	return string(out), err
}

func TestBackupCommitRequiresEngineSuccess(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "committed")
	command := "fake() { case \"$1\" in snapshots) echo '[]';; backup) " +
		"echo '{\"message_type\":\"summary\",\"snapshot_id\":\"abc\"}'; return 1;; " +
		"tag) touch " + shellQuote(marker) + ";; esac; };\n"
	command += BackupCommand("fake", "printf data", "volume", nil)
	out, err := runShell(t, command)
	require.Error(t, err, out)
	_, err = os.Stat(marker)
	require.True(t, os.IsNotExist(err), "failed engine committed a snapshot")
}

func TestLocalBackupCompletion(t *testing.T) {
	binary := os.Getenv("HARVESTER_TEST_RESTIC_BINARY")
	if binary == "" {
		t.Skip("HARVESTER_TEST_RESTIC_BINARY is not set")
	}
	dir := t.TempDir()
	t.Setenv("RESTIC_REPOSITORY", filepath.Join(dir, "repo"))
	t.Setenv("RESTIC_PASSWORD", "local-regression-test")
	t.Setenv("TMPDIR", dir)
	command := shellQuote(binary) + " --no-cache"
	out, err := runShell(t, command+" init")
	require.NoError(t, err, out)
	tags := []string{"ns=default", "vmb=backup", "sn=volume"}
	// Simulate a snapshot left by the old stdin pipeline before this change.
	out, err = runShell(t, "printf partial | "+command+
		" backup --stdin --stdin-filename volume --tag=ns=default,vmb=backup,sn=volume")
	require.NoError(t, err, out)
	for _, reader := range []string{
		"sh -c 'printf partial; exit 1'",
		"sh -c 'printf partial; kill -KILL $$'",
	} {
		out, err = runShell(t, BackupCommand(command, reader, "volume", tags))
		require.Error(t, err, out)
		out, err = runShell(t, SnapshotCheckCommand(command, tags))
		var exit *exec.ExitError
		require.ErrorAs(t, err, &exit, out)
		require.Equal(t, SnapshotCheckMissingExitCode, exit.ExitCode())
	}
	out, err = runShell(t, BackupCommand(command, "printf complete-data", "volume", tags))
	require.NoError(t, err, out)
	out, err = runShell(t, SnapshotCheckCommand(command, tags))
	require.NoError(t, err, out)
	out, err = runShell(t, command+" dump --tag="+completedTags(tags)+" latest /volume")
	require.NoError(t, err, out)
	require.Equal(t, "complete-data", out)
	out, err = runShell(t, ForgetCommand(command, tags))
	require.NoError(t, err, out)
	out, err = runShell(t, command+" snapshots --json")
	require.NoError(t, err, out)
	require.JSONEq(t, "[]", out)
}
