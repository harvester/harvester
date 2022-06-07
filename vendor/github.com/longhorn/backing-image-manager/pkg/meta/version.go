package meta

const (
	// BackingImageManagerAPIVersion used to communicate with the user
	// \e.g. longhorn-manager
	BackingImageManagerAPIVersion    = 3
	BackingImageManagerAPIMinVersion = 3
)

// Following variables are filled in by main.go
var (
	Version   string
	GitCommit string
	BuildDate string
)

type VersionOutput struct {
	Version   string `json:"version"`
	GitCommit string `json:"gitCommit"`
	BuildDate string `json:"buildDate"`

	BackingImageManagerAPIVersion    int `json:"backingImageManagerAPIVersion"`
	BackingImageManagerAPIMinVersion int `json:"backingImageManagerAPIMinVersion"`
}

func GetVersion() VersionOutput {
	return VersionOutput{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,

		BackingImageManagerAPIVersion:    BackingImageManagerAPIVersion,
		BackingImageManagerAPIMinVersion: BackingImageManagerAPIMinVersion,
	}
}
