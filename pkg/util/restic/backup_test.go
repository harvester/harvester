package restic

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func runShell(t *testing.T, command string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// #nosec G702 -- tests intentionally execute generated scripts with trusted test inputs.
	out, err := exec.CommandContext(ctx, "/bin/bash", "-c", command).CombinedOutput()
	return string(out), err
}

func TestBackupCommitRequiresEngineSuccess(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "committed")
	command := "fake() { if [ \"$1\" = --retry-lock ]; then shift 2; fi; " +
		"case \"$1\" in unlock) echo 'removed stale lock';; backup) " +
		"echo '{\"message_type\":\"summary\",\"snapshot_id\":\"abc\"}'; return 1;; " +
		"tag) touch " + shellQuote(marker) + ";; esac; };\n"
	command += BackupCommand("fake", "printf data", "volume", nil)
	out, err := runShell(t, command)
	require.Error(t, err, out)
	_, err = os.Stat(marker)
	require.True(t, os.IsNotExist(err), "failed engine committed a snapshot")
}

func TestBackupTagRetriesLocks(t *testing.T) {
	for _, tc := range []struct {
		name  string
		locks int
		code  int
	}{
		{name: "success"},
		{name: "lock waits exhausted", locks: 2},
		{name: "backend failure", code: 1},
		{name: "missing repository", code: 10},
		{name: "wrong password", code: 12},
		{name: "interrupted", code: 130},
		{name: "unknown failure", code: 42},
		{name: "failure after lock wait", locks: 1, code: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := filepath.Join(t.TempDir(), "calls")
			script := fmt.Sprintf(`
tag_calls=0
fake() {
    printf '%%s\n' "$*" >> %s
    if [ "$1" = --retry-lock ]; then shift 2; fi
    case "$1" in
        unlock) echo 'removed stale lock';;
        backup) echo '{"message_type":"summary","snapshot_id":"abc"}';;
        tag)
            tag_calls=$((tag_calls + 1))
            if [ "$tag_calls" -le %d ]; then return 11; fi
            return %d;;
        snapshots) echo '[{"id":"def","original":"abc"}]';;
        *) return 99;;
    esac
}
`, shellQuote(calls), tc.locks, tc.code) + BackupCommand("fake", "printf data", "volume", nil)
			out, err := runShell(t, script)
			if tc.code == 0 {
				require.NoError(t, err, out)
			} else {
				var exit *exec.ExitError
				require.ErrorAs(t, err, &exit, out)
				require.Equal(t, tc.code, exit.ExitCode())
			}
			log, err := os.ReadFile(calls)
			require.NoError(t, err)
			lines := strings.Split(strings.TrimSpace(string(log)), "\n")
			expected := 2 * (2 + tc.locks)
			if tc.code == 0 {
				expected += 2 // Unlock and verify the completed snapshot.
				require.Equal(t, "--retry-lock 5m snapshots --json --tag=harvester-complete", lines[len(lines)-1])
			}
			require.Len(t, lines, expected)
			for i := 0; i < len(lines); i += 2 {
				require.Equal(t, "unlock", lines[i])
			}
			require.Contains(t, lines[1], "--retry-lock 5m backup ")
			for i := 3; i < 2*(2+tc.locks); i += 2 {
				require.Equal(t, "--retry-lock 5m tag --add harvester-complete abc", lines[i],
					"retry only the tag for the uploaded snapshot")
			}
		})
	}
}

func TestBackupRetriesUploadLock(t *testing.T) {
	calls := filepath.Join(t.TempDir(), "calls")
	script := fmt.Sprintf(`
attempts=0
fake() {
    printf '%%s\n' "$*" >> %s
    if [ "$1" = --retry-lock ]; then shift 2; fi
    case "$1" in
        unlock) echo 'removed stale lock';;
        backup)
            attempts=$((attempts + 1))
            if [ "$attempts" -le 2 ]; then return 11; fi
            echo '{"message_type":"summary","snapshot_id":"abc"}';;
        tag) return 0;;
        snapshots) echo '[{"id":"def","original":"abc"}]';;
        *) return 99;;
    esac
}
`, shellQuote(calls)) + BackupCommand("fake", "printf data", "volume", nil)
	out, err := runShell(t, script)
	require.NoError(t, err, out)
	log, err := os.ReadFile(calls)
	require.NoError(t, err)
	require.Equal(t, 3, strings.Count(string(log), "--retry-lock 5m backup "))
	require.Equal(t, 1, strings.Count(string(log), "--retry-lock 5m tag --add harvester-complete abc"))
	require.Equal(t, 5, strings.Count(string(log), "unlock\n"))
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
	// Simulate an interrupted backup that never added its completion tag.
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
