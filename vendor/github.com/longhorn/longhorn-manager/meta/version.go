package meta

// Following variables will be filled by command `-ldflags "-X ..."`
// in scripts/build
var (
	Version   string
	GitCommit string
	BuildDate string
)
