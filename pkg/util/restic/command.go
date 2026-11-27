package restic

import "fmt"

// retryCommand removes stale locks before each lock-acquisition attempt.
// Restic backs off within each 5m window; only exit code 11 starts a new one.
// Wrap the Restic process before piping its output so a consumer's exit code
// cannot hide a lock error. Keep unlock and retry output out of JSON and volume data.
func retryCommand(command, args string) string {
	return fmt.Sprintf(
		"(while true; do\n"+
			"    %s unlock >&2 || exit $?\n"+
			"    status=0\n"+
			"    %s --retry-lock 5m %s || status=$?\n"+
			"    if [ \"$status\" -ne 11 ]; then exit \"$status\"; fi\n"+
			"    printf 'Repository is locked; retrying lock acquisition\\n' >&2\n"+
			"done)",
		command, command, args,
	)
}
