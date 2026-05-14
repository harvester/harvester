package version

import "fmt"

var (
	Version          = "dev"
	GitCommit        = "HEAD"
	HarvesterVersion = "dev" // Will be replaced by ldflags
)

func FriendlyVersion() string {
	return fmt.Sprintf("%s (%s)", Version, GitCommit)
}
