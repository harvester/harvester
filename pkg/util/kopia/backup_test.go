package kopia

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/harvester/harvester/pkg/settings"
)

func runShell(t *testing.T, command string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/bin/bash", "-c", command).CombinedOutput()
	return string(out), err
}

func TestBackupCommitRequiresPipelineSuccess(t *testing.T) {
	for _, tc := range []struct {
		name, reader string
		engineExit   string
	}{
		{"reader", "sh -c 'printf partial; exit 1'", "0"},
		{"killed reader", "sh -c 'printf partial; kill -KILL $$'", "0"},
		{"engine", "printf data", "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "committed")
			command := "fake() { if [ \"$2\" = create ]; then cat >/dev/null; " +
				"echo '{\"id\":\"abc\"}'; return " + tc.engineExit + "; fi; touch " + shellQuote(marker) + "; };\n"
			command += BackupCommand("fake", "true", tc.reader, "volume", nil)
			out, err := runShell(t, command)
			require.Error(t, err, out)
			_, err = os.Stat(marker)
			require.True(t, os.IsNotExist(err), "failed pipeline committed a snapshot")
		})
	}
}

// This uses a local repository; it does not need a cluster or object store.
func TestLocalBackupCompletion(t *testing.T) {
	binary := os.Getenv("HARVESTER_TEST_KOPIA_BINARY")
	if binary == "" {
		t.Skip("HARVESTER_TEST_KOPIA_BINARY is not set")
	}
	resetKopiaSettings(t)
	require.NoError(t, settings.ThirdPartyBackupConfigSet.Set(`{"kopia":{"cacheSize":"8Mi"}}`))
	env, err := CacheConfigEnv()
	require.NoError(t, err)
	for _, entry := range env {
		t.Setenv(entry.Name, entry.Value)
	}
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	t.Setenv(LogDirEnvVar, logDir)
	t.Setenv("KOPIA_PASSWORD", "local-regression-test")
	// Seed both families above their 256KiB retention target to exercise sweeps.
	for _, family := range []string{"cli-logs", "content-logs"} {
		path := filepath.Join(logDir, family, "kopia-old.log")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
		require.NoError(t, os.WriteFile(path, make([]byte, 512*1024), 0600))
		old := time.Now().Add(-time.Hour)
		require.NoError(t, os.Chtimes(path, old, old))
	}
	command := strings.Replace(Command(), "kopia ", shellQuote(binary)+" --no-auto-maintenance ", 1)
	command = strings.Replace(command, ConfigPath, shellQuote(filepath.Join(dir, "config")), 1)
	out, err := runShell(t, command+" repository create filesystem --path="+
		shellQuote(filepath.Join(dir, "repo"))+" --cache-directory="+shellQuote(filepath.Join(dir, "cache")))
	require.NoError(t, err, out)
	// Exercise the production check/restore helpers with the local config.
	rewrite := func(script string) string { return strings.ReplaceAll(script, Command(), command) }
	tags := []string{"ns:default", "vmb:backup", "sn:volume"}
	out, err = runShell(t, BackupCommand(command, "true", "sh -c 'printf partial; exit 1'", "volume", tags))
	require.Error(t, err, out)
	out, err = runShell(t, rewrite(SnapshotCheckCommand("true", tags)))
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit, out)
	require.Equal(t, SnapshotCheckMissingExitCode, exit.ExitCode())
	out, err = runShell(t, rewrite("set -eo pipefail\n"+SnapshotObjectCommand("true", tags)))
	require.Error(t, err, out)

	out, err = runShell(t, BackupCommand(command, "true", "printf complete-data", "volume", tags))
	require.NoError(t, err, out)
	out, err = runShell(t, rewrite(SnapshotCheckCommand("true", tags)))
	require.NoError(t, err, out)
	out, err = runShell(t, rewrite("set -eo pipefail\n"+SnapshotObjectCommand("true", tags))+"\n"+
		command+" show \"$OBJ/volume\"")
	require.NoError(t, err, out)
	require.Equal(t, "complete-data", out)

	// Deletion must include both uncommitted attempts and pinned snapshots.
	out, err = runShell(t, rewrite(SnapshotDeleteCommand("true", tags)))
	require.NoError(t, err, out)
	out, err = runShell(t, rewrite(SnapshotListCommand(tags)))
	require.NoError(t, err, out)
	require.JSONEq(t, "[]", out)

	for _, family := range []string{"cli-logs", "content-logs"} {
		path := filepath.Join(logDir, family)
		_, err := os.Stat(filepath.Join(path, "kopia-old.log"))
		require.True(t, os.IsNotExist(err), "old %s log was not swept", family)
		files, err := filepath.Glob(filepath.Join(path, "kopia-*.log"))
		require.NoError(t, err)
		require.NotEmpty(t, files, "%s logging is disabled", family)
		var total int64
		for _, name := range files {
			info, err := os.Stat(name)
			require.NoError(t, err)
			total += info.Size()
		}
		require.Positive(t, total, "%s logs are empty", family)
		require.LessOrEqual(t, total, int64(256*1024), "%s retention target exceeded after exit", family)
	}
}
