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
			expected := "unlock\n--retry-lock 5m snapshots --json --tag=vmb=backup,harvester-complete\n"
			require.Equal(t, strings.Repeat(expected, tc.locks+1), string(log))
		})
	}
}

func TestRetryStopsOnUnlockError(t *testing.T) {
	for _, code := range []int{1, 10, 11, 12, 130} {
		for _, attempt := range []int{1, 2} {
			t.Run(fmt.Sprintf("exit_%d_attempt_%d", code, attempt), func(t *testing.T) {
				calls := filepath.Join(t.TempDir(), "calls")
				script := fmt.Sprintf(`
unlocks=0
fake() {
    printf '%%s\n' "$*" >> %s
    if [ "$1" != unlock ]; then return 11; fi
    unlocks=$((unlocks + 1))
    if [ "$unlocks" -eq %d ]; then return %d; fi
    return 0
}
set -e
`, shellQuote(calls), attempt, code)
				requireExit(t, script+retryCommand("fake", "snapshots"), code)
				log, err := os.ReadFile(calls)
				require.NoError(t, err)
				expected := strings.Repeat("unlock\n--retry-lock 5m snapshots\n", attempt-1) + "unlock\n"
				require.Equal(t, expected, string(log), "unlock failure must stop before the next command")
			})
		}
	}
}

// fakeCommand fails lock acquisition before producing output. Its call log
// catches unexpected init calls, missing unlock calls and retries of non-lock errors.
func fakeCommand(calls string, locks, code int, output string) string {
	return fmt.Sprintf(`
attempts=0
fake() {
    printf '%%s\n' "$*" >> %s
    if [ "$1" = unlock ]; then echo 'removed stale lock'; return 0; fi
    attempts=$((attempts + 1))
    if [ "$attempts" -le %d ]; then return 11; fi
    %s
    return %d
}
`, shellQuote(calls), locks, output, code)
}
