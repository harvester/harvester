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
	backup := fmt.Sprintf(
		"backup --json --stdin-from-command --stdin-filename %s --tag=%s -- %s",
		shellQuote(filename), shellQuote(strings.Join(tags, ",")), reader,
	)
	return fmt.Sprintf(
		"set -eo pipefail\n"+
			"SNAPSHOT=$(%s"+
			" | jq -er 'select(.message_type == \"summary\") | .snapshot_id // empty')\n"+
			"test -n \"$SNAPSHOT\"\n"+
			"%s",
		retryCommand(command, backup),
		retryCommand(command, "tag --add "+CompleteTag+" \"$SNAPSHOT\""),
	)
}
