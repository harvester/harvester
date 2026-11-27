package restic

import "fmt"

// RestoreCommand retries dump's lock acquisition before it outputs any data.
// pipefail preserves both dump and writer errors. Quiet mode and stderr-only
// retry messages keep the stream unchanged; the leading / matches the backup.
func RestoreCommand(command, writer, filename string, tags []string) string {
	dump := fmt.Sprintf("-q dump --tag=%s latest %s",
		shellQuote(completedTags(tags)), shellQuote("/"+filename))
	return fmt.Sprintf("set -eo pipefail\n%s | %s", retryCommand(command, dump), writer)
}
