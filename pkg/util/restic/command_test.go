package restic

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireExit(t *testing.T, command string, code int) string {
	t.Helper()
	out, err := runShell(t, command)
	if code == 0 {
		require.NoError(t, err, out)
		return out
	}
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit, out)
	require.Equal(t, code, exit.ExitCode(), out)
	return out
}

func TestInitCommand(t *testing.T) {
	for _, tc := range []struct {
		name     string
		probes   string
		initCode int
		code     int
		calls    string
	}{
		{name: "existing repository", probes: "0", calls: "snapshots"},
		{name: "locked repository", probes: "11\n11\n0", calls: "snapshots\nsnapshots\nsnapshots"},
		{name: "missing repository", probes: "10", calls: "snapshots\ninit"},
		{name: "missing after lock", probes: "11\n10", calls: "snapshots\nsnapshots\ninit"},
		{name: "wrong password", probes: "12", code: 12, calls: "snapshots"},
		{name: "backend failure", probes: "1", code: 1, calls: "snapshots"},
		{name: "interrupted", probes: "130", code: 130, calls: "snapshots"},
		{name: "concurrent initialization", probes: "10\n0", initCode: 1, calls: "snapshots\ninit\nsnapshots"},
		{name: "locked after init", probes: "10\n11\n0", initCode: 1, calls: "snapshots\ninit\nsnapshots\nsnapshots"},
		{name: "init failed", probes: "10\n10", initCode: 1, code: 10, calls: "snapshots\ninit\nsnapshots"},
		{name: "init with wrong password", probes: "10\n12", initCode: 1, code: 12, calls: "snapshots\ninit\nsnapshots"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			probes, calls := filepath.Join(dir, "probes"), filepath.Join(dir, "calls")
			require.NoError(t, os.WriteFile(probes, []byte(tc.probes+"\n"), 0600))
			script := fmt.Sprintf(`
fake() {
    if [ "$1" = --retry-lock ]; then shift 2; fi
    printf '%%s\n' "$1" >> %s
    case "$1" in
        snapshots)
            read -r code < %s || return 99
            sed -i '1d' %s
            return "$code";;
        init) return %d;;
        *) return 99;;
    esac
}
`, shellQuote(calls), shellQuote(probes), shellQuote(probes), tc.initCode)
			requireExit(t, script+"set -eo pipefail\n"+initCommand("fake"), tc.code)
			log, err := os.ReadFile(calls)
			require.NoError(t, err)
			require.Equal(t, tc.calls+"\n", string(log))
		})
	}
}

func TestSnapshotCheckRetriesLocks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		locks  int
		result string
		code   int
		want   int
	}{
		{name: "found after lock", locks: 2, result: `[{"id":"abc"}]`},
		{name: "missing snapshot", result: "[]", want: SnapshotCheckMissingExitCode},
		{name: "missing after lock", locks: 2, result: "[]", want: SnapshotCheckMissingExitCode},
		{name: "missing repository", code: 10, want: 10},
		{name: "wrong password", code: 12, want: 12},
		{name: "backend failure", locks: 1, code: 1, want: 1},
		{name: "interrupted", code: 130, want: 130},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := filepath.Join(t.TempDir(), "calls")
			script := fakeCommand(calls, tc.locks, tc.code, "printf '%s\\n' "+shellQuote(tc.result))
			requireExit(t, script+SnapshotCheckCommand("fake", []string{"vmb=backup"}), tc.want)
			log, err := os.ReadFile(calls)
			require.NoError(t, err)
			expected := "--retry-lock 5m snapshots --json --tag=vmb=backup,harvester-complete\n"
			require.Equal(t, strings.Repeat(expected, tc.locks+1), string(log))
		})
	}
}

// fakeCommand fails lock acquisition before producing output. Its call log
// catches unexpected init calls and retries of non-lock errors.
func fakeCommand(calls string, locks, code int, output string) string {
	return fmt.Sprintf(`
attempts=0
fake() {
    printf '%%s\n' "$*" >> %s
    attempts=$((attempts + 1))
    if [ "$attempts" -le %d ]; then return 11; fi
    %s
    return %d
}
`, shellQuote(calls), locks, output, code)
}
