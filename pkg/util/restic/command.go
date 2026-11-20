package restic

import "fmt"

// retryCommand waits for lock acquisition without retrying other failures.
// Restic backs off within each 5m window; only exit code 11 starts a new one.
// Wrap the Restic process before piping its output so a consumer's exit code
// cannot hide a lock error. Keep retry messages out of JSON and volume data.
func retryCommand(command, args string) string {
	return fmt.Sprintf(
		"(until %s --retry-lock 5m %s; do\n"+
			"    status=$?\n"+
			"    if [ \"$status\" -ne 11 ]; then exit \"$status\"; fi\n"+
			"    printf 'Repository is locked; retrying lock acquisition\\n' >&2\n"+
			"done)",
		command, args,
	)
}

// initCommand initializes only a missing repository. If another client creates
// it first, a successful probe after init fails allows the backup to continue.
func initCommand(command string) string {
	probe := retryCommand(command, "snapshots")
	return fmt.Sprintf(
		"(if %s > /dev/null; then exit 0; else status=$?; fi\n"+
			"if [ \"$status\" -ne 10 ]; then exit \"$status\"; fi\n"+
			"%s init || %s > /dev/null)",
		probe, command, probe,
	)
}
