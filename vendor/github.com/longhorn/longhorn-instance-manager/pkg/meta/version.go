package meta

const (
	// InstanceManagerAPIVersion used to communicate with the user e.g. longhorn-manager
	InstanceManagerAPIVersion    = 1
	InstanceManagerAPIMinVersion = 1

	// InstanceManagerProxyAPIVersion is used for compatibility check for longhorn-manager
	InstanceManagerProxyAPIVersion    = 1
	InstanceManagerProxyAPIMinVersion = 1
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

	InstanceManagerAPIVersion    int `json:"instanceManagerAPIVersion"`
	InstanceManagerAPIMinVersion int `json:"instanceManagerAPIMinVersion"`

	InstanceManagerProxyAPIVersion    int `json:"instanceManagerProxyAPIVersion"`
	InstanceManagerProxyAPIMinVersion int `json:"instanceManagerProxyAPIMinVersion"`
}

func GetVersion() VersionOutput {
	return VersionOutput{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,

		InstanceManagerAPIVersion:    InstanceManagerAPIVersion,
		InstanceManagerAPIMinVersion: InstanceManagerAPIMinVersion,

		InstanceManagerProxyAPIVersion:    InstanceManagerProxyAPIVersion,
		InstanceManagerProxyAPIMinVersion: InstanceManagerProxyAPIMinVersion,
	}
}
