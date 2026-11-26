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
			"%s\n%s",
		retryCommand(command, backup),
		retryCommand(command, "tag --add "+CompleteTag+" \"$SNAPSHOT\""),
		verifyCompletion(command, tags),
	)
}

// Tag can return success after failing to save the modified snapshot. Verify
// its completion tag using the uploaded ID, which tag preserves as original.
func verifyCompletion(command string, tags []string) string {
	query := "snapshots --json --tag=" + shellQuote(completedTags(tags))
	return fmt.Sprintf(
		"SNAPSHOTS=$( %s )\n"+
			"if ! printf '%%s\\n' \"$SNAPSHOTS\" | jq -en --arg id \"$SNAPSHOT\" "+
			"'input | any(.[]; .id == $id or .original == $id)' > /dev/null; then\n"+
			"    printf 'Completed snapshot %%s could not be verified\\n' \"$SNAPSHOT\" >&2\n"+
			"    exit 1\n"+
			"fi",
		retryCommand(command, query),
	)
}
