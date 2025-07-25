package utils

const (
	DefaultMTU       = 1500
	MaxMTU           = 9000
	MinMTU           = 576 // IPv4 does not define this explicitly; IPv6 defines 1280; Some protocol requires 576; hence 576 is used
	defaultNamespace = "default"
)
