package node

// TODO: use rancher/system-agent as source after it bumps k8s to v1.30.x
type HTTPGetAction struct {
	URL        string `json:"url,omitempty"`
	Insecure   bool   `json:"insecure,omitempty"`
	ClientCert string `json:"clientCert,omitempty"`
	ClientKey  string `json:"clientKey,omitempty"`
	CACert     string `json:"caCert,omitempty"`
}

type Probe struct {
	Name                string        `json:"name,omitempty"`
	InitialDelaySeconds int           `json:"initialDelaySeconds,omitempty"` // default 0
	TimeoutSeconds      int           `json:"timeoutSeconds,omitempty"`      // default 1
	SuccessThreshold    int           `json:"successThreshold,omitempty"`    // default 1
	FailureThreshold    int           `json:"failureThreshold,omitempty"`    // default 3
	HTTPGetAction       HTTPGetAction `json:"httpGet,omitempty"`
}

type Plan struct {
	Files                []File                `json:"files,omitempty"`
	OneTimeInstructions  []OneTimeInstruction  `json:"instructions,omitempty"`
	Probes               map[string]Probe      `json:"probes,omitempty"`
	PeriodicInstructions []PeriodicInstruction `json:"periodicInstructions,omitempty"`
	// ResetFailureCountOnStartup denotes whether the system-agent should reset the failure count
	// and applied-checksum for plans that are force applied each time the system-agent starts.
	ResetFailureCountOnStartup bool `json:"resetFailureCountOnStartup,omitempty"`
}

type CommonInstruction struct {
	Name    string   `json:"name,omitempty"`
	Image   string   `json:"image,omitempty"`
	Env     []string `json:"env,omitempty"`
	Args    []string `json:"args,omitempty"`
	Command string   `json:"command,omitempty"`
}

type PeriodicInstruction struct {
	CommonInstruction
	PeriodSeconds    int  `json:"periodSeconds,omitempty"` // default 600, i.e. 10 minutes
	SaveStderrOutput bool `json:"saveStderrOutput,omitempty"`
}

type PeriodicInstructionOutput struct {
	Name                  string `json:"name"`
	Stdout                []byte `json:"stdout"`                // Stdout is a byte array of the gzip+base64 stdout output
	Stderr                []byte `json:"stderr"`                // Stderr is a byte array of the gzip+base64 stderr output
	ExitCode              int    `json:"exitCode"`              // ExitCode is an int representing the exit code of the last run instruction
	LastSuccessfulRunTime string `json:"lastSuccessfulRunTime"` // LastSuccessfulRunTime is a time.UnixDate formatted string of the last successful time (exit code 0) the instruction was run
	Failures              int    `json:"failures"`              // Failures is the number of time the periodic instruction has failed to run
	LastFailedRunTime     string `json:"lastFailedRunTime"`     // LastFailedRunTime is a time.UnixDate formatted string of the time that the periodic instruction started failing
}

type OneTimeInstruction struct {
	CommonInstruction
	SaveOutput bool `json:"saveOutput,omitempty"`
}

// Path would be `/etc/kubernetes/ssl/ca.pem`, Content is base64 encoded.
// If Directory is true, then we are creating a directory, not a file
type File struct {
	Content     string `json:"content,omitempty"`
	Directory   bool   `json:"directory,omitempty"`
	UID         int    `json:"uid,omitempty"`
	GID         int    `json:"gid,omitempty"`
	Path        string `json:"path,omitempty"`
	Permissions string `json:"permissions,omitempty"` // internally, the string will be converted to a uint32 to satisfy os.FileMode
}
