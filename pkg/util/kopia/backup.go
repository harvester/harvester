package kopia

import "fmt"

const CompletePin = "harvester-complete"

// BackupCommand records a completion pin only after the entire pipeline
// succeeds. A killed/failed reader may leave an unpinned partial snapshot;
// snapshot discovery and restore must never accept it.
func BackupCommand(command, connect, reader, filename string, tags []string) string {
	return fmt.Sprintf(
		"set -eo pipefail\n%s\n"+
			"SNAPSHOT=$(%s | %s snapshot create %s --stdin-file %s --json %s"+
			" | jq -er 'select((.incomplete // \"\") == \"\") | .id // empty')\n"+
			"test -n \"$SNAPSHOT\"\n"+
			"%s snapshot pin --add %s \"$SNAPSHOT\"",
		connect, reader, command, shellQuote("/"+filename), shellQuote(filename),
		SnapshotTagsFlags(tags), command, CompletePin,
	)
}

func completedSnapshots(tags []string) string {
	return fmt.Sprintf(
		"%s | jq '[.[] | select((.incomplete // \"\") == \"\")"+
			" | select((.pins // []) | index(\"%s\"))]'",
		SnapshotListCommand(tags), CompletePin,
	)
}
