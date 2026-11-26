package restic

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBackupCompletion(t *testing.T) {
	for _, tc := range []struct {
		name      string
		snapshots string
		code      int
		locks     int
		wantErr   bool
	}{
		{name: "matching ID", snapshots: `[{"id":"abc"}]`},
		{name: "matching original", snapshots: `[{"id":"def","original":"abc"}]`},
		{name: "no tag written", snapshots: `[]`, wantErr: true},
		{name: "unrelated snapshot", snapshots: `[{"id":"def","original":"previous"}]`, wantErr: true},
		{name: "invalid JSON", snapshots: `invalid`, wantErr: true},
		{name: "empty output", wantErr: true},
		{name: "query failure", snapshots: `[{"id":"abc"}]`, code: 1, wantErr: true},
		{name: "wrong password", code: 12, wantErr: true},
		{name: "query lock", snapshots: `[{"id":"def","original":"abc"}]`, locks: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := filepath.Join(t.TempDir(), "calls")
			script := fmt.Sprintf(`
queries=0
fake() {
    printf '%%s\n' "$*" >> %s
    if [ "$1" = --retry-lock ]; then shift 2; fi
    case "$1" in
        unlock) echo 'removed stale lock';;
        backup) echo '{"message_type":"summary","snapshot_id":"abc"}';;
        tag) echo '{"message_type":"summary","changed_snapshots":0}';;
        snapshots)
            queries=$((queries + 1))
            if [ "$queries" -le %d ]; then return 11; fi
            printf '%%s\n' %s
            return %d;;
        *) return 99;;
    esac
}
`, shellQuote(calls), tc.locks, shellQuote(tc.snapshots), tc.code)
			script += BackupCommand("fake", "printf data", "volume", []string{"vmb=backup"})
			out, err := runShell(t, script)
			if tc.wantErr {
				require.Error(t, err, out)
			} else {
				require.NoError(t, err, out)
			}
			if tc.code != 0 {
				var exit *exec.ExitError
				require.ErrorAs(t, err, &exit, out)
				require.Equal(t, tc.code, exit.ExitCode())
			}
			log, err := os.ReadFile(calls)
			require.NoError(t, err)
			require.Equal(t, 1, strings.Count(string(log), "--retry-lock 5m backup "))
			require.Equal(t, 1, strings.Count(string(log), "--retry-lock 5m tag "))
			require.Equal(t, 1+tc.locks, strings.Count(string(log),
				"--retry-lock 5m snapshots --json --tag=vmb=backup,harvester-complete\n"))
		})
	}
}

func TestBackupRejectsUnwrittenTag(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires an unprivileged user to deny snapshot writes")
	}
	lr := newLocalRestic(t)
	snapshots := filepath.Join(lr.repository, "snapshots")
	t.Cleanup(func() { require.NoError(t, os.Chmod(snapshots, 0700)) })
	tagStatus := filepath.Join(t.TempDir(), "tag-status")
	// Deny only tag's snapshot write, after the backup has uploaded successfully.
	script := fmt.Sprintf(`
fault() {
    if [ "$3" != tag ]; then %s --no-cache "$@"; return $?; fi
    chmod 500 %s
    status=0
    %s --no-cache "$@" || status=$?
    chmod 700 %s
    printf '%%s\n' "$status" > %s
    return "$status"
}
`, shellQuote(lr.binary), shellQuote(snapshots), shellQuote(lr.binary), shellQuote(snapshots), shellQuote(tagStatus))
	tags := []string{"ns=default", "vmb=unwritten-tag", "sn=volume"}
	script += BackupCommand("fault", "printf complete-data", "volume", tags)
	out, err := runShell(t, script)
	require.Error(t, err, out)
	require.Contains(t, out, "unable to modify the tags")
	require.Contains(t, out, "could not be verified")
	status, err := os.ReadFile(tagStatus)
	require.NoError(t, err)
	require.Equal(t, "0\n", string(status), "Restic must exhibit the false success being guarded against")
	require.JSONEq(t, "[]", string(lr.run("", "snapshots", "--json", "--tag", completedTags(tags))))
	require.Equal(t, "complete-data", string(lr.run("", "dump", "--tag", strings.Join(tags, ","), "latest", "/volume")))
}

func TestBackupAllowsRetainedSnapshot(t *testing.T) {
	lr := newLocalRestic(t)
	saved := filepath.Join(t.TempDir(), "original")
	// Recreate the old snapshot to model a failed deletion after tag has saved
	// its replacement. Restic reports zero changes in that case, despite the tag.
	script := fmt.Sprintf(`
retain() {
    if [ "$3" != tag ]; then %s --no-cache "$@"; return $?; fi
    cp "$RESTIC_REPOSITORY/snapshots/$SNAPSHOT" %s || return $?
    %s --no-cache "$@" > /dev/null || return $?
    cp %s "$RESTIC_REPOSITORY/snapshots/$SNAPSHOT" || return $?
    echo '{"message_type":"summary","changed_snapshots":0}'
}
`, shellQuote(lr.binary), shellQuote(saved), shellQuote(lr.binary), shellQuote(saved))
	tags := []string{"ns=default", "vmb=retained-snapshot", "sn=volume"}
	script += BackupCommand("retain", "printf complete-data", "volume", tags)
	out, err := runShell(t, script)
	require.NoError(t, err, out)
	var snapshots []struct {
		ID       string   `json:"id"`
		Original string   `json:"original"`
		Tags     []string `json:"tags"`
	}
	data := lr.run("", "snapshots", "--json", "--tag", strings.Join(tags, ","))
	require.NoError(t, json.Unmarshal(data, &snapshots))
	require.Len(t, snapshots, 2, "both the original and completed snapshots must remain")
	var original, completed string
	for _, snapshot := range snapshots {
		if snapshot.Original == "" {
			original = snapshot.ID
			continue
		}
		completed = snapshot.Original
		require.Contains(t, snapshot.Tags, CompleteTag)
	}
	require.NotEmpty(t, original)
	require.Equal(t, original, completed)
	require.Equal(t, "complete-data", string(lr.run("", "dump", "--tag", completedTags(tags), "latest", "/volume")))
}
