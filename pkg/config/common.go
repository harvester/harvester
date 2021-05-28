package config

type CommonOptions struct {
	Debug     bool
	Trace     bool
	LogFormat string

	ProfilerAddress string
	KubeConfig      string
}
