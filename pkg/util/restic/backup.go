package restic

import (
	"fmt"
	"strings"
)

const CompleteTag = "harvester-complete"

func completedTags(tags []string) string {
	return strings.Join(append(append([]string{}, tags...), CompleteTag), ",")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// BackupCommand commits only the exact snapshot produced by a successful
// reader and engine. Never promote snapshots by tag: a previous attempt may
// have left a partial snapshot with those same tags.
func BackupCommand(command, reader, filename string, tags []string) string {
	return fmt.Sprintf(
		"set -eo pipefail\n"+
			"(%[1]s snapshots > /dev/null 2>&1 || { %[1]s init || %[1]s snapshots > /dev/null 2>&1; })\n"+
			"SNAPSHOT=$(%[1]s backup --json --stdin-from-command --stdin-filename %[2]s --tag=%[3]s -- %[4]s"+
			" | jq -er 'select(.message_type == \"summary\") | .snapshot_id // empty')\n"+
			"test -n \"$SNAPSHOT\"\n"+
			"%[1]s tag --add %[5]s \"$SNAPSHOT\"",
		command, shellQuote(filename), shellQuote(strings.Join(tags, ",")), reader, CompleteTag,
	)
}
