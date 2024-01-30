package meta

const (
	// CLIAPIVersion used to communicate with user e.g. longhorn-manager
	CLIAPIVersion    = 9
	CLIAPIMinVersion = 3

	// ControllerAPIVersion used to communicate with instance-manager
	ControllerAPIVersion    = 5
	ControllerAPIMinVersion = 3

	// DataFormatVersion used by the Replica to store data
	DataFormatVersion    = 1
	DataFormatMinVersion = 1
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

	CLIAPIVersion           int `json:"cliAPIVersion"`
	CLIAPIMinVersion        int `json:"cliAPIMinVersion"`
	ControllerAPIVersion    int `json:"controllerAPIVersion"`
	ControllerAPIMinVersion int `json:"controllerAPIMinVersion"`
	DataFormatVersion       int `json:"dataFormatVersion"`
	DataFormatMinVersion    int `json:"dataFormatMinVersion"`
}

func GetVersion() VersionOutput {
	return VersionOutput{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,

		CLIAPIVersion:           CLIAPIVersion,
		CLIAPIMinVersion:        CLIAPIMinVersion,
		ControllerAPIVersion:    ControllerAPIVersion,
		ControllerAPIMinVersion: ControllerAPIMinVersion,
		DataFormatVersion:       DataFormatVersion,
		DataFormatMinVersion:    DataFormatMinVersion,
	}
}
