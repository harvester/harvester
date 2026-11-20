package restic

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRestoreRetriesLocks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		locks  int
		code   int
		writer string
		want   int
	}{
		{name: "success", writer: "cat"},
		{name: "locked", locks: 2, writer: "cat"},
		{name: "missing repository", code: 10, writer: "cat", want: 10},
		{name: "wrong password", code: 12, writer: "cat", want: 12},
		{name: "transfer failure", code: 1, writer: "cat", want: 1},
		{name: "failure after lock", locks: 1, code: 1, writer: "cat", want: 1},
		{name: "interrupted", code: 130, writer: "cat", want: 130},
		{name: "writer failure", writer: "(cat; exit 7)", want: 7},
		{name: "writer exit 11", writer: "(cat; exit 11)", want: 11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			calls, data := filepath.Join(dir, "calls"), filepath.Join(dir, "data")
			script := fakeCommand(calls, tc.locks, tc.code, `printf 'volume\000data\n'`)
			writer := tc.writer + " > " + shellQuote(data)
			script += RestoreCommand("fake", writer, "volume", []string{"vmb=backup"})
			requireExit(t, script, tc.want)
			log, err := os.ReadFile(calls)
			require.NoError(t, err)
			expected := "--retry-lock 5m -q dump --tag=vmb=backup,harvester-complete latest /volume\n"
			require.Equal(t, strings.Repeat(expected, tc.locks+1), string(log))
			payload, err := os.ReadFile(data)
			require.NoError(t, err)
			require.Equal(t, []byte("volume\x00data\n"), payload, "retry messages must not reach the writer")
		})
	}
}
